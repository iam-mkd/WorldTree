package world

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iam-mkd/WorldTree/internal/ports"
	"github.com/iam-mkd/WorldTree/internal/registry"
)

// colliding is the normalised form of a typical compose file: fixed name, fixed host port.
const colliding = `{"name":"app","services":{"db":{"image":"postgres:17","container_name":"app-db",
"ports":[{"mode":"ingress","target":5432,"published":"5432","protocol":"tcp"}]}}}`

const fixedSubnet = `{"name":"app","services":{"db":{"image":"postgres:17"}},
"networks":{"n":{"name":"app_n","ipam":{"config":[{"subnet":"172.31.250.0/24"}]}}}}`

type fakeDocker struct {
	calls  [][]string
	fail   error
	loaded []string // files the loader was given
}

func (f *fakeDocker) run(_ context.Context, _ string, _ io.Writer, args ...string) error {
	f.calls = append(f.calls, args)
	return f.fail
}

func newTestUpper(t *testing.T, cfg string) (*Upper, *fakeDocker) {
	t.Helper()
	reg, err := registry.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fd := &fakeDocker{}
	return &Upper{
		Reg:   reg,
		Alloc: ports.Allocator{Min: 41000, Max: 41999},
		Load: func(_ context.Context, _ string, files []string) ([]byte, error) {
			fd.loaded = files
			return []byte(cfg), nil
		},
		Run:   fd.run,
		Query: func(context.Context, ...string) ([]byte, error) { return nil, nil },
		Log:   io.Discard,
		Now:   func() time.Time { return time.Unix(0, 0) },

		LockTimeout: time.Second,
	}, fd
}

func projectDir(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("services: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func load(t *testing.T, reg *registry.Registry) *registry.State {
	t.Helper()
	st, err := reg.Load()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestUpRegistersLeasesWritesOverrideAndRunsCompose(t *testing.T) {
	u, fd := newTestUpper(t, colliding)
	dir := projectDir(t, "docker-compose.yml")

	res, err := u.Up(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	w := load(t, u.Reg).Worlds[res.ID]
	if w == nil || w.Phase != registry.Ready || w.Project != res.Project || w.Dir != dir {
		t.Fatalf("registry world = %+v", w)
	}
	if len(res.Leases) != 1 {
		t.Fatalf("leases = %v", res.Leases)
	}
	var port int
	for _, p := range res.Leases {
		port = p
	}
	if port < 41000 || port > 41999 {
		t.Errorf("leased port %d outside allocator range", port)
	}

	ov, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(OverridePath)))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"!reset", strconv.Itoa(port), res.Project} {
		if !strings.Contains(string(ov), want) {
			t.Errorf("override lacks %q:\n%s", want, ov)
		}
	}

	if gi, err := os.ReadFile(filepath.Join(dir, ".worldtree", ".gitignore")); err != nil || string(gi) != "*\n" {
		t.Errorf(".worldtree/.gitignore = %q, %v; want \"*\\n\"", gi, err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, ".worldtree"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != ".gitignore,compose.override.yml,env" {
		t.Errorf(".worldtree holds %v; want only .gitignore, compose.override.yml and env (no temp files)", names)
	}
	envData, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(EnvPath)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envData), "WORLDTREE_PORT_DB_5432='"+strconv.Itoa(port)+"'") ||
		!strings.Contains(string(envData), "DATABASE_URL='postgres://postgres@127.0.0.1:"+strconv.Itoa(port)+"/postgres'") {
		t.Errorf("env file lacks port/database vars:\n%s", envData)
	}
	if st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(EnvPath))); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("env file mode = %v, want 0600 (it holds the database password)", st.Mode().Perm())
	}

	if len(fd.calls) != 1 {
		t.Fatalf("docker calls = %v", fd.calls)
	}
	got := strings.Join(fd.calls[0], " ")
	if want := []string{filepath.Join(dir, "docker-compose.yml")}; strings.Join(fd.loaded, ",") != strings.Join(want, ",") {
		t.Errorf("analysis loaded %v, want %v", fd.loaded, want)
	}
	for _, want := range []string{
		"compose -p " + res.Project,
		"-f " + filepath.Join(dir, "docker-compose.yml"),
		"-f " + filepath.Join(dir, filepath.FromSlash(OverridePath)),
		"up -d --wait",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("docker args %q lack %q", got, want)
		}
	}
}

func TestUpIsIdempotentAndKeepsPorts(t *testing.T) {
	u, _ := newTestUpper(t, colliding)
	dir := projectDir(t, "docker-compose.yml")
	first, err := u.Up(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := u.Up(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Project != second.Project {
		t.Errorf("identity changed: %v/%v vs %v/%v", first.ID, first.Project, second.ID, second.Project)
	}
	for k, p := range first.Leases {
		if second.Leases[k] != p {
			t.Errorf("lease %s moved %d -> %d", k, p, second.Leases[k])
		}
	}
	if n := len(load(t, u.Reg).Leases); n != 1 {
		t.Errorf("registry has %d leases, want 1", n)
	}
}

func TestUpFailureLeavesCreatingWorldThatRetrySucceedsOn(t *testing.T) {
	u, fd := newTestUpper(t, colliding)
	dir := projectDir(t, "docker-compose.yml")
	fd.fail = errors.New("port is already allocated")

	if _, err := u.Up(context.Background(), dir); err == nil {
		t.Fatal("expected error")
	}
	st := load(t, u.Reg)
	id := ID(dir)
	if w := st.Worlds[id]; w == nil || w.Phase != registry.Creating {
		t.Fatalf("after failed up, world = %+v, want phase creating", w)
	}
	before := st.LeasesOf(id)
	if len(before) != 1 {
		t.Fatalf("leases after failure = %v", before)
	}

	fd.fail = nil
	res, err := u.Up(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if load(t, u.Reg).Worlds[id].Phase != registry.Ready {
		t.Error("retry did not reach ready")
	}
	for k, p := range before {
		if res.Leases[k] != p {
			t.Errorf("retry moved lease %s %d -> %d", k, p, res.Leases[k])
		}
	}
}

func TestUpRefusesUnisolatableProjectWithoutSideEffects(t *testing.T) {
	u, fd := newTestUpper(t, fixedSubnet)
	dir := projectDir(t, "docker-compose.yml")
	if _, err := u.Up(context.Background(), dir); err == nil {
		t.Fatal("expected refusal for fixed subnet")
	}
	if len(fd.calls) != 0 {
		t.Errorf("docker was called: %v", fd.calls)
	}
	if n := len(load(t, u.Reg).Worlds); n != 0 {
		t.Errorf("registry has %d worlds, want 0", n)
	}
	if _, err := os.Stat(filepath.Join(dir, ".worldtree")); !os.IsNotExist(err) {
		t.Errorf(".worldtree created for refused project: %v", err)
	}
}

func TestUpWaitsForBusyWorldAndGivesUp(t *testing.T) {
	u, fd := newTestUpper(t, colliding)
	u.LockTimeout = 50 * time.Millisecond
	dir := projectDir(t, "docker-compose.yml")

	unlock, err := u.Reg.LockWorld(ID(dir), time.Second) // another up/down in flight
	if err != nil {
		t.Fatal(err)
	}
	_, err = u.Up(context.Background(), dir)
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("err = %v, want busy world", err)
	}
	if len(fd.calls) != 0 || len(load(t, u.Reg).Worlds) != 0 {
		t.Errorf("busy up had side effects: calls=%v", fd.calls)
	}

	unlock()
	if _, err := u.Up(context.Background(), dir); err != nil {
		t.Fatalf("up after unlock: %v", err)
	}
}

func TestTwoDirsGetDistinctProjectsAndPorts(t *testing.T) {
	u, _ := newTestUpper(t, colliding)
	a, err := u.Up(context.Background(), projectDir(t, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := u.Up(context.Background(), projectDir(t, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if a.Project == b.Project {
		t.Errorf("same project %s", a.Project)
	}
	for _, pa := range a.Leases {
		for _, pb := range b.Leases {
			if pa == pb {
				t.Errorf("both worlds leased port %d", pa)
			}
		}
	}
}
