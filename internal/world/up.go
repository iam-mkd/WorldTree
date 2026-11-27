package world

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/iam-mkd/WorldTree/internal/compose"
	"github.com/iam-mkd/WorldTree/internal/ports"
	"github.com/iam-mkd/WorldTree/internal/registry"
)

// OverridePath is where `up` writes the world's compose override, relative to the world dir.
const OverridePath = ".worldtree/compose.override.yml"

// Analysis is a compose project analysed for one world.
type Analysis struct {
	Dir     string   // absolute world dir
	Files   []string // compose files, resolved once; analysis and `up` both use exactly these
	ID      string
	Project string
	Plan    *compose.Plan
}

// Loader runs `docker compose config` for dir with exactly files (compose.LoadConfig).
type Loader func(ctx context.Context, dir string, files []string) ([]byte, error)

// Analyse resolves the compose files of absDir once, then loads and analyses them. load
// needs the compose CLI but creates nothing in Docker.
func Analyse(ctx context.Context, absDir string, load Loader) (*Analysis, error) {
	files, err := compose.DefaultFiles(absDir)
	if err != nil {
		return nil, err
	}
	raw, err := load(ctx, absDir, files)
	if err != nil {
		return nil, err
	}
	cfg, err := compose.ParseConfig(raw)
	if err != nil {
		return nil, err
	}
	id := ID(absDir)
	project := ProjectName(filepath.Base(absDir), id)
	plan, err := compose.NewPlan(cfg, compose.Options{
		WorldID: id, Project: project, ProjectDir: absDir, IsFile: isFile,
	})
	if err != nil {
		return nil, err
	}
	return &Analysis{Dir: absDir, Files: files, ID: id, Project: project, Plan: plan}, nil
}

// Upper brings worlds up. Load and Run are the only ways it reaches Docker,
// so tests can replace them.
type Upper struct {
	Reg   *registry.Registry
	Alloc ports.Allocator
	Load  Loader
	// Run runs `docker <args>` in dir, streaming output to log.
	Run func(ctx context.Context, dir string, log io.Writer, args ...string) error
	Log io.Writer
	Now func() time.Time
	// LockTimeout bounds the wait for another `up`/`down` of the same world.
	LockTimeout time.Duration
}

// NewUpper returns an Upper wired to the real registry, allocator and docker CLI.
func NewUpper(reg *registry.Registry, log io.Writer) *Upper {
	return &Upper{Reg: reg, Alloc: ports.Default(), Load: compose.LoadConfig, Run: runDocker, Log: log,
		Now: time.Now, LockTimeout: 15 * time.Minute} // a first `up` can spend minutes building images
}

// UpResult describes a world after a successful `up`.
type UpResult struct {
	*Analysis
	Leases map[string]int // PortRequest.Key -> host port
}

// Up creates the world for absDir, or reconciles it if it exists. Every step is
// idempotent, so re-running after a failure or crash continues from what exists:
//
//  1. analyse the compose project; refuse it if it cannot be isolated (nothing is recorded)
//  2. register the world as `creating` and lease its ports, in one locked registry update,
//     before anything is created in Docker
//  3. write <dir>/.worldtree/compose.override.yml
//  4. docker compose -p <project> -f <project files...> -f <override> up -d --wait
//  5. mark the world `ready`
//
// If step 4 fails the world stays `creating` with its leases; re-run `up` or `gc` it.
//
// The whole sequence holds the per-world lock, so two `up`s of one dir cannot interleave
// (one could otherwise release a lease the other's containers are about to publish).
func (u *Upper) Up(ctx context.Context, absDir string) (*UpResult, error) {
	unlock, err := u.Reg.LockWorld(ID(absDir), u.LockTimeout)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := stopStaleCleanup(ctx, u.Reg, ID(absDir), absDir, u.Run); err != nil {
		return nil, err
	}

	a, err := Analyse(ctx, absDir, u.Load)
	if err != nil {
		return nil, err
	}
	if a.Plan.HasErrors() {
		for _, f := range a.Plan.Findings {
			if f.Severity == compose.Error {
				fmt.Fprintf(u.Log, "  %s\n", f)
			}
		}
		return nil, fmt.Errorf("%s cannot be isolated as is (see errors above); nothing was created", absDir)
	}

	keys := make([]string, len(a.Plan.Ports))
	for i, r := range a.Plan.Ports {
		keys[i] = r.Key
	}
	var leases map[string]int
	err = u.Reg.Update(func(s *registry.State) error {
		s.Put(registry.World{ID: a.ID, Dir: absDir, Project: a.Project, Phase: registry.Creating}, u.Now())
		var err error
		leases, err = s.Lease(a.ID, keys, u.Alloc)
		return err
	})
	if err != nil {
		return nil, err
	}

	override, err := a.Plan.Render(leases)
	if err != nil {
		return nil, err
	}
	ovPath := filepath.Join(absDir, filepath.FromSlash(OverridePath))
	if err := writeIfChanged(ovPath, override); err != nil {
		return nil, err
	}
	// Self-ignoring dir: the world's files never show up in the user's `git status`.
	if err := writeIfChanged(filepath.Join(filepath.Dir(ovPath), ".gitignore"), []byte("*\n")); err != nil {
		return nil, err
	}

	args := []string{"compose", "-p", a.Project, "--project-directory", absDir}
	for _, f := range a.Files {
		args = append(args, "-f", f)
	}
	args = append(args, "-f", ovPath, "up", "-d", "--wait")
	if err := u.Run(ctx, absDir, u.Log, args...); err != nil {
		return nil, fmt.Errorf("docker compose up failed; world %s left in phase %q (re-run `worldtree up` or `worldtree gc`): %w", a.ID, registry.Creating, err)
	}

	err = u.Reg.Update(func(s *registry.State) error {
		w, ok := s.Worlds[a.ID]
		if !ok {
			return fmt.Errorf("world %s vanished from the registry during up (concurrent down?)", a.ID)
		}
		cp := *w
		cp.Phase = registry.Ready
		s.Put(cp, u.Now())
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &UpResult{Analysis: a, Leases: leases}, nil
}

func writeIfChanged(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && string(old) == string(data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil { // CreateTemp makes 0600
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func runDocker(ctx context.Context, dir string, log io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = log, log
	return cmd.Run()
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}
