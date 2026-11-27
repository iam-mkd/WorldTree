// Package registry is WorldTree's on-disk state: the known worlds and their port
// leases, in one JSON file.
//
// Writers take an exclusive lock on a separate lock file (flock / LockFileEx), read
// the current state, change it, and replace the file atomically (temp file + fsync +
// rename). Readers need no lock: rename means they see the old or the new file, never
// half of one. A process killed mid-update leaves the previous state intact, and the
// OS drops its lock.
package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

// SchemaVersion of registry.json. A newer file is refused rather than silently rewritten.
const SchemaVersion = 1

// Phase of a world's lifecycle.
type Phase string

// Phases. A world is registered as Creating *before* anything is created in Docker,
// so a crash always leaves a record `gc` can follow.
const (
	Creating Phase = "creating"
	Ready    Phase = "ready"
	Removing Phase = "removing"
)

// World is one registered world.
type World struct {
	ID        string    `json:"id"`
	Dir       string    `json:"dir"`
	Project   string    `json:"project"`
	Phase     Phase     `json:"phase"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Lease reserves one host port for one world. Key identifies what it is for
// (compose.PortRequest.Key), so re-running `up` gets the same port back.
type Lease struct {
	Port  int    `json:"port"`
	World string `json:"world"`
	Key   string `json:"key"`
}

// State is the whole registry.
type State struct {
	Version int               `json:"version"`
	Worlds  map[string]*World `json:"worlds"`
	Leases  []Lease           `json:"leases"`
}

func newState() *State { return &State{Version: SchemaVersion, Worlds: map[string]*World{}} }

// Registry is a handle on a registry directory. It is safe to use from many
// goroutines and processes at once.
type Registry struct {
	dir         string
	LockTimeout time.Duration
}

// Open returns the registry in dir, creating dir if needed.
func Open(dir string) (*Registry, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("registry: %w", err)
	}
	return &Registry{dir: dir, LockTimeout: 30 * time.Second}, nil
}

// DefaultDir is $WORLDTREE_HOME, else the per-user state dir:
// Linux $XDG_STATE_HOME/worldtree (~/.local/state/worldtree), macOS
// ~/Library/Application Support/worldtree, Windows %LocalAppData%\worldtree.
func DefaultDir() (string, error) {
	if d := os.Getenv("WORLDTREE_HOME"); d != "" {
		return d, nil
	}
	switch runtime.GOOS {
	case "darwin", "windows":
		d, err := os.UserConfigDir() // macOS: ~/Library/Application Support; Windows: %AppData%
		if runtime.GOOS == "windows" {
			if l := os.Getenv("LocalAppData"); l != "" {
				d, err = l, nil
			}
		}
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "worldtree"), nil
	default:
		if d := os.Getenv("XDG_STATE_HOME"); d != "" {
			return filepath.Join(d, "worldtree"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "state", "worldtree"), nil
	}
}

// Path of the JSON state file.
func (r *Registry) Path() string { return filepath.Join(r.dir, "registry.json") }

func (r *Registry) lockPath() string { return filepath.Join(r.dir, "registry.lock") }

// Load returns a snapshot of the state without locking.
func (r *Registry) Load() (*State, error) {
	data, err := os.ReadFile(r.Path())
	if errors.Is(err, os.ErrNotExist) {
		return newState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("registry: %w", err)
	}
	return decode(r.Path(), data)
}

// decode parses and validates the registry. Anything it cannot trust is an error, and
// Update never writes after a failed decode, so a bad file is left as it is.
func decode(path string, data []byte) (*State, error) {
	// Never overwrite a file we cannot read: it may be hand-edited or from a newer worldtree.
	corrupt := func(why any) error {
		return fmt.Errorf("registry %s is corrupt (%v); fix or move it aside", path, why)
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, corrupt("document is null") // Unmarshal would silently keep the empty default
	}
	s := newState()
	if err := json.Unmarshal(data, s); err != nil {
		return nil, corrupt(err)
	}
	for id, w := range s.Worlds {
		if w == nil {
			return nil, corrupt(fmt.Sprintf("world %q is null", id))
		}
		if w.ID != id {
			return nil, corrupt(fmt.Sprintf("world %q has id %q", id, w.ID))
		}
	}
	ports := map[int]string{}
	for _, l := range s.Leases {
		if other, dup := ports[l.Port]; dup {
			return nil, corrupt(fmt.Sprintf("port %d leased to both %q and %q", l.Port, other, l.World))
		}
		ports[l.Port] = l.World
	}
	if s.Version > SchemaVersion {
		return nil, fmt.Errorf("registry %s has schema %d; this worldtree understands %d (upgrade worldtree)", path, s.Version, SchemaVersion)
	}
	if s.Worlds == nil {
		s.Worlds = map[string]*World{}
	}
	s.Version = SchemaVersion
	return s, nil
}

// Update runs fn on the current state under the exclusive lock and saves the
// result atomically. If fn returns an error nothing is written.
func (r *Registry) Update(fn func(*State) error) error {
	unlock, err := lock(r.lockPath(), r.LockTimeout)
	if err != nil {
		return err
	}
	defer unlock()

	before, err := os.ReadFile(r.Path())
	var s *State
	switch {
	case errors.Is(err, os.ErrNotExist):
		s, before = newState(), nil
	case err != nil:
		return fmt.Errorf("registry: %w", err)
	default:
		if s, err = decode(r.Path(), before); err != nil {
			return err
		}
	}
	if err := fn(s); err != nil {
		return err
	}
	sort.Slice(s.Leases, func(i, j int) bool { return s.Leases[i].Port < s.Leases[j].Port })
	after, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("registry: %w", err)
	}
	after = append(after, '\n')
	if bytes.Equal(before, after) {
		return nil
	}
	return writeAtomic(r.Path(), after)
}

// writeAtomic writes data to a temp file in the same dir, fsyncs it, and renames it over
// path. Readers see the old or the new file, never a mix. The directory fsync after the
// rename is best effort: a nil return does not guarantee the new file survives a power
// loss, and the previous registry state may come back instead (still a whole file).
func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".registry-*.tmp")
	if err != nil {
		return fmt.Errorf("registry: %w", err)
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("registry: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("registry: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("registry: %w", err)
	}
	if err := replaceFile(tmp, path); err != nil {
		return fmt.Errorf("registry: %w", err)
	}
	ok = true
	syncDir(filepath.Dir(path))
	return nil
}
