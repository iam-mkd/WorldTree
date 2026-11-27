package registry

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iam-mkd/WorldTree/internal/ports"
)

// The test binary doubles as a helper process (see helperMain) for the
// multi-process and kill -9 tests.
func TestMain(m *testing.M) {
	if mode := os.Getenv("WT_REGISTRY_HELPER"); mode != "" {
		os.Exit(helperMain(mode))
	}
	os.Exit(m.Run())
}

var t0 = time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)

// always-free allocator over a small range, so concurrent worlds fight over the same ports.
func tinyAlloc() ports.Allocator {
	return ports.Allocator{Min: 30000, Max: 30099, Probe: func(int) bool { return true }}
}

func helperMain(mode string) int {
	r, err := Open(os.Getenv("WT_REGISTRY_DIR"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	id := os.Getenv("WT_REGISTRY_WORLD")
	switch mode {
	case "lease":
		var got map[string]int
		err = r.Update(func(s *State) error {
			s.Put(World{ID: id, Dir: "/w/" + id, Phase: Creating}, time.Now())
			got, err = s.Lease(id, []string{"db", "web"}, tinyAlloc())
			return err
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(got["db"], got["web"])
	case "hold":
		// Take the lock, say so, and block until killed.
		_ = r.Update(func(*State) error {
			fmt.Println("locked")
			time.Sleep(time.Hour)
			return nil
		})
	}
	return 0
}

func helper(t *testing.T, dir, mode, world string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), "WT_REGISTRY_HELPER="+mode, "WT_REGISTRY_DIR="+dir, "WT_REGISTRY_WORLD="+world)
	cmd.Stderr = os.Stderr
	return cmd
}

func open(t *testing.T) *Registry {
	t.Helper()
	r, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestEmptyRegistryLoads(t *testing.T) {
	s, err := open(t).Load()
	if err != nil || s.Version != SchemaVersion || len(s.Worlds) != 0 {
		t.Fatalf("got %+v, %v", s, err)
	}
}

func TestPutKeepsCreatedAtAndRemoveReleasesLeases(t *testing.T) {
	r := open(t)
	must(t, r.Update(func(s *State) error {
		s.Put(World{ID: "a", Phase: Creating}, t0)
		s.Put(World{ID: "b", Phase: Creating}, t0)
		_, err := s.Lease("a", []string{"x", "y"}, tinyAlloc())
		if err != nil {
			return err
		}
		_, err = s.Lease("b", []string{"x"}, tinyAlloc())
		return err
	}))
	must(t, r.Update(func(s *State) error {
		s.Put(World{ID: "a", Phase: Ready}, t0.Add(time.Minute))
		s.Remove("b")
		s.Remove("never-existed")
		return nil
	}))
	s, err := r.Load()
	must(t, err)
	a := s.Worlds["a"]
	if a.Phase != Ready || !a.CreatedAt.Equal(t0) || !a.UpdatedAt.Equal(t0.Add(time.Minute)) {
		t.Errorf("world a = %+v", a)
	}
	if _, ok := s.Worlds["b"]; ok || len(s.Leases) != 2 {
		t.Errorf("b not removed cleanly: %+v %+v", s.Worlds, s.Leases)
	}
}

func TestLeaseIsStableAndReconciles(t *testing.T) {
	r := open(t)
	var first, second map[string]int
	must(t, r.Update(func(s *State) error {
		s.Put(World{ID: "a"}, t0)
		var err error
		first, err = s.Lease("a", []string{"db", "web", "old"}, tinyAlloc())
		return err
	}))
	must(t, r.Update(func(s *State) error {
		var err error
		second, err = s.Lease("a", []string{"web", "db", "new"}, tinyAlloc())
		return err
	}))
	if first["db"] != second["db"] || first["web"] != second["web"] {
		t.Errorf("kept keys moved: %v -> %v", first, second)
	}
	s, _ := r.Load()
	got := s.LeasesOf("a")
	if _, ok := got["old"]; ok || len(got) != 3 {
		t.Errorf("leases after reconcile = %v, want db, web, new", got)
	}
	for k, p := range got {
		if k != "db" && k != "web" && (p == first["db"] || p == first["web"]) {
			t.Errorf("new key %s got a port still leased: %d", k, p)
		}
	}
}

func TestLeaseRejectsBadInput(t *testing.T) {
	s := newState()
	if _, err := s.Lease("ghost", []string{"x"}, tinyAlloc()); err == nil {
		t.Error("want error for unregistered world")
	}
	s.Put(World{ID: "a"}, t0)
	if _, err := s.Lease("a", []string{"x", "x"}, tinyAlloc()); err == nil {
		t.Error("want error for duplicate key")
	}
}

func TestLeaseSkipsHeldPort(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	defer l.Close()
	held := l.Addr().(*net.TCPAddr).Port
	s := newState()
	s.Put(World{ID: "a"}, t0)
	// The held port is the whole range, so the only correct answer is an error.
	got, err := s.Lease("a", []string{"db"}, ports.Allocator{Min: held, Max: held})
	if err == nil {
		t.Fatalf("leased %v although the only port in range (%d) is held", got, held)
	}
	if len(s.LeasesOf("a")) != 0 {
		t.Errorf("failed lease left leases behind: %v", s.LeasesOf("a"))
	}
}

// A reader holding registry.json open (a concurrent Load) must not make a committed
// Update fail. On Unix rename never conflicts; on Windows replaceFile retries.
func TestUpdateWhileReaderHoldsFileOpen(t *testing.T) {
	r := open(t)
	must(t, r.Update(func(s *State) error { s.Put(World{ID: "a"}, t0); return nil }))
	f, err := os.Open(r.Path())
	must(t, err)
	released := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = f.Close()
		close(released)
	}()
	must(t, r.Update(func(s *State) error { s.Put(World{ID: "b"}, t0); return nil }))
	<-released
	s, err := r.Load()
	must(t, err)
	if _, ok := s.Worlds["b"]; !ok {
		t.Error("update lost")
	}
}

func TestFailedUpdateWritesNothing(t *testing.T) {
	r := open(t)
	must(t, r.Update(func(s *State) error { s.Put(World{ID: "a"}, t0); return nil }))
	before, _ := os.ReadFile(r.Path())
	boom := errors.New("boom")
	if err := r.Update(func(s *State) error { s.Remove("a"); return boom }); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	after, _ := os.ReadFile(r.Path())
	if string(before) != string(after) {
		t.Error("registry changed although fn failed")
	}
	if tmp, _ := filepath.Glob(filepath.Join(filepath.Dir(r.Path()), ".registry-*")); len(tmp) != 0 {
		t.Errorf("temp files left: %v", tmp)
	}
}

func TestCorruptOrNewerFileIsRefusedNotOverwritten(t *testing.T) {
	for name, content := range map[string]string{
		"corrupt":        `{"version": 1, "worlds": {`,
		"newer":          `{"version": 99, "worlds": {}}`,
		"null document":  `null`,
		"null world":     `{"version": 1, "worlds": {"a": null}}`,
		"world id moved": `{"version": 1, "worlds": {"a": {"id": "b"}}}`,
		"duplicate port": `{"version": 1, "worlds": {}, "leases": [{"port": 20000, "world": "a", "key": "x"}, {"port": 20000, "world": "b", "key": "y"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			r := open(t)
			must(t, os.WriteFile(r.Path(), []byte(content), 0o600))
			if _, err := r.Load(); err == nil {
				t.Error("Load: want error")
			}
			if err := r.Update(func(*State) error { return nil }); err == nil {
				t.Error("Update: want error")
			}
			if b, _ := os.ReadFile(r.Path()); string(b) != content {
				t.Error("file was overwritten")
			}
		})
	}
}

func checkUnique(t *testing.T, r *Registry, worlds int) {
	t.Helper()
	s, err := r.Load()
	must(t, err)
	if len(s.Worlds) != worlds || len(s.Leases) != 2*worlds {
		t.Fatalf("want %d worlds / %d leases, got %d / %d", worlds, 2*worlds, len(s.Worlds), len(s.Leases))
	}
	seen := map[int]string{}
	for _, l := range s.Leases {
		if w, dup := seen[l.Port]; dup {
			t.Errorf("port %d leased to %s and %s", l.Port, w, l.World)
		}
		seen[l.Port] = l.World
	}
}

func TestConcurrentGoroutines(t *testing.T) {
	r := open(t)
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			errs <- r.Update(func(s *State) error {
				s.Put(World{ID: id}, t0)
				_, err := s.Lease(id, []string{"db", "web"}, tinyAlloc())
				return err
			})
		}(fmt.Sprintf("g%02d", i))
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		must(t, err)
	}
	checkUnique(t, r, n)
}

func TestConcurrentProcesses(t *testing.T) {
	r := open(t)
	dir := filepath.Dir(r.Path())
	const n = 20
	cmds := make([]*exec.Cmd, n)
	outs := make([]*strings.Builder, n)
	for i := range cmds {
		cmds[i] = helper(t, dir, "lease", fmt.Sprintf("p%02d", i))
		outs[i] = &strings.Builder{}
		cmds[i].Stdout = outs[i]
	}
	for _, c := range cmds { // start all before waiting on any
		must(t, c.Start())
	}
	reported := map[int]bool{}
	for i, c := range cmds {
		if err := c.Wait(); err != nil {
			t.Fatalf("helper %d: %v", i, err)
		}
		for _, f := range strings.Fields(outs[i].String()) {
			p, _ := strconv.Atoi(f)
			if reported[p] {
				t.Errorf("port %d reported by two processes", p)
			}
			reported[p] = true
		}
	}
	checkUnique(t, r, n)
}

func TestKilledLockHolderReleasesLock(t *testing.T) {
	r := open(t)
	cmd := helper(t, filepath.Dir(r.Path()), "hold", "")
	stdout, err := cmd.StdoutPipe()
	must(t, err)
	must(t, cmd.Start())
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("helper did not take the lock: %q %v", line, err)
	}

	r.LockTimeout = 200 * time.Millisecond
	if err := r.Update(func(*State) error { return nil }); err == nil || !strings.Contains(err.Error(), "locked by another") {
		t.Fatalf("want lock timeout while helper holds it, got %v", err)
	}

	must(t, cmd.Process.Kill()) // SIGKILL on Unix, TerminateProcess on Windows
	_ = cmd.Wait()
	r.LockTimeout = 5 * time.Second
	start := time.Now()
	must(t, r.Update(func(s *State) error { s.Put(World{ID: "after"}, t0); return nil }))
	if d := time.Since(start); d > time.Second {
		t.Errorf("lock took %s to free after kill", d)
	}
}

func TestDefaultDirHonoursEnv(t *testing.T) {
	t.Setenv("WORLDTREE_HOME", "/custom/wt")
	if d, err := DefaultDir(); err != nil || d != "/custom/wt" {
		t.Errorf("got %q %v", d, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
