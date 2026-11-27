package world

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iam-mkd/WorldTree/internal/compose"
	"github.com/iam-mkd/WorldTree/internal/registry"
)

// Downer removes worlds. Run and Query are its only ways to reach Docker.
type Downer struct {
	Reg *registry.Registry
	// Run runs `docker <args>` in dir, streaming output to log.
	Run func(ctx context.Context, dir string, log io.Writer, args ...string) error
	// Query runs `docker <args>` and returns stdout.
	Query       func(ctx context.Context, args ...string) ([]byte, error)
	Log         io.Writer
	Now         func() time.Time
	LockTimeout time.Duration
	// RemoveAll deletes a directory tree; nil means os.RemoveAll. Injected for tests.
	RemoveAll func(path string) error
}

// CleanupImage runs the fallback delete of container-owned files in a world's .worldtree dir.
const CleanupImage = "alpine:3"

// NewDowner returns a Downer wired to the real registry and docker CLI.
func NewDowner(reg *registry.Registry, log io.Writer) *Downer {
	return &Downer{Reg: reg, Run: runDocker, Query: queryDocker, Log: log, Now: time.Now, LockTimeout: 15 * time.Minute}
}

// DownResult says what `down` found.
type DownResult struct {
	ID         string
	Project    string
	Registered bool // the world had a registry record
	Swept      int  // labelled resources compose down left behind, removed by the sweep
}

// Down removes the world of absDir: containers, networks, named volumes (including its
// database), port leases, the registry record and <dir>/.worldtree. absDir need not exist
// any more (a deleted worktree); the world is found by its id, which is derived from the path.
//
// Every step is idempotent, so `down` of an unknown, half-created or half-removed world
// succeeds and finishes the job:
//
//  1. mark the world `removing` (if registered), before touching Docker
//  2. docker compose -p <project> down -v --remove-orphans, run where no compose file is
//     found, so compose works from its labels alone and never reads the user's files
//  3. remove anything still labelled dev.worldtree.world=<id> (containers, volumes, networks)
//  4. delete <dir>/.worldtree
//  5. remove the registry record, releasing its leases
func (d *Downer) Down(ctx context.Context, absDir string) (*DownResult, error) {
	id := ID(absDir)
	unlock, err := d.Reg.LockWorld(id, d.LockTimeout)
	if err != nil {
		return nil, err
	}
	defer unlock()

	res := &DownResult{ID: id, Project: ProjectName(filepath.Base(absDir), id)}
	err = d.Reg.Update(func(s *registry.State) error {
		w, ok := s.Worlds[id]
		if !ok {
			return nil
		}
		res.Registered = true
		if w.Project != "" {
			res.Project = w.Project
		}
		cp := *w
		cp.Phase = registry.Removing
		s.Put(cp, d.Now())
		return nil
	})
	if err != nil {
		return nil, err
	}

	// The registry dir holds no compose file, so compose cannot pick up the user's.
	neutral := filepath.Dir(d.Reg.Path())
	if err := d.Run(ctx, neutral, d.Log, "compose", "-p", res.Project, "down", "-v", "--remove-orphans"); err != nil {
		return nil, fmt.Errorf("docker compose down for %s failed; world %s left in phase %q (re-run `worldtree down`): %w",
			res.Project, id, registry.Removing, err)
	}
	n, err := d.sweep(ctx, id)
	res.Swept = n
	if err != nil {
		return nil, err
	}

	gen := filepath.Join(absDir, filepath.Dir(filepath.FromSlash(OverridePath)))
	if err := d.removeGenerated(ctx, id, gen, neutral); err != nil {
		return nil, fmt.Errorf("removing %s failed; world %s left in phase %q with its leases (re-run `worldtree down`): %w",
			gen, id, registry.Removing, err)
	}
	if err := d.Reg.Update(func(s *registry.State) error { s.Remove(id); return nil }); err != nil {
		return nil, err
	}
	return res, nil
}

// removeGenerated deletes the world's .worldtree dir. Remapped binds live under it, and a
// container may have made their contents unremovable for this user (on Linux, postgres
// chowns its data dir to its own uid with mode 0700). On a permission error only, the
// contents are deleted as root from a throwaway container that mounts just this dir.
func (d *Downer) removeGenerated(ctx context.Context, id, gen, neutral string) error {
	removeAll := d.RemoveAll
	if removeAll == nil {
		removeAll = os.RemoveAll
	}
	err := removeAll(gen)
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		return err
	}
	fmt.Fprintf(d.Log, "%s holds files owned by a container user; deleting them from a %s container\n", gen, CleanupImage)
	name := CleanupContainer(id)
	if rerr := d.Run(ctx, neutral, d.Log, "run", "--rm", "--name", name, "--label", compose.LabelWorld+"="+id,
		"-v", gen+":/w", CleanupImage, "find", "/w", "-mindepth", "1", "-delete"); rerr != nil {
		// The docker client may have been killed (cancelled ctx) while the container keeps
		// running; stop it before the world lock is released, even if ctx is cancelled.
		_ = d.Run(context.WithoutCancel(ctx), neutral, io.Discard, "rm", "-f", name)
		return fmt.Errorf("%w (container cleanup also failed: %v)", err, rerr)
	}
	return removeAll(gen)
}

// CleanupContainer is the name of the world's fallback cleanup container. If worldtree itself
// dies mid-cleanup the container can outlive the world lock; it carries the world label, so
// the next `down` sweeps it, and the next `up` removes it by name (stopStaleCleanup).
func CleanupContainer(id string) string { return "worldtree-cleanup-" + id }

// stopStaleCleanup force-removes a cleanup container left by a `down` that died, if the world
// is still `removing`, so it cannot delete files the caller is about to write. Called with the
// world lock held. A missing container is not an error.
func stopStaleCleanup(ctx context.Context, reg *registry.Registry, id, dir string,
	run func(ctx context.Context, dir string, log io.Writer, args ...string) error) error {
	st, err := reg.Load()
	if err != nil {
		return err
	}
	if w, ok := st.Worlds[id]; ok && w.Phase == registry.Removing {
		_ = run(ctx, dir, io.Discard, "rm", "-f", CleanupContainer(id))
	}
	return nil
}

// sweep removes resources labelled with the world id that compose down did not, such as
// containers started outside compose with the label, or resources of a renamed project.
func (d *Downer) sweep(ctx context.Context, id string) (int, error) {
	filter := "label=" + compose.LabelWorld + "=" + id
	kinds := []struct {
		list []string
		rm   []string
	}{
		{[]string{"ps", "-aq", "--filter", filter}, []string{"rm", "-f"}},
		{[]string{"volume", "ls", "-q", "--filter", filter}, []string{"volume", "rm", "-f"}},
		{[]string{"network", "ls", "-q", "--filter", filter}, []string{"network", "rm"}},
	}
	total := 0
	for _, k := range kinds { // containers first: volumes and networks in use cannot be removed
		out, err := d.Query(ctx, k.list...)
		if err != nil {
			return total, err
		}
		ids := strings.Fields(string(out))
		if len(ids) == 0 {
			continue
		}
		if _, err := d.Query(ctx, append(k.rm, ids...)...); err != nil {
			return total, err
		}
		total += len(ids)
	}
	return total, nil
}

func queryDocker(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}
