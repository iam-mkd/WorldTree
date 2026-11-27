package world

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iam-mkd/WorldTree/internal/registry"
)

// fakeQuery answers `docker <args>` queries from a table keyed by the first two args.
type fakeQuery struct {
	answers map[string]string
	calls   []string
	fail    error
}

func (f *fakeQuery) query(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	if f.fail != nil {
		return nil, f.fail
	}
	return []byte(f.answers[strings.Join(args[:2], " ")]), nil
}

type runCall struct {
	dir  string
	args string
}

func newTestDowner(reg *registry.Registry) (*Downer, *[]runCall, *fakeQuery) {
	var calls []runCall
	fq := &fakeQuery{answers: map[string]string{}}
	return &Downer{
		Reg: reg,
		Run: func(_ context.Context, dir string, _ io.Writer, args ...string) error {
			calls = append(calls, runCall{dir, strings.Join(args, " ")})
			return nil
		},
		Query:       fq.query,
		Log:         io.Discard,
		Now:         func() time.Time { return time.Unix(0, 0) },
		LockTimeout: time.Second,
	}, &calls, fq
}

func TestDownAfterUpRemovesEverything(t *testing.T) {
	u, _ := newTestUpper(t, colliding)
	dir := projectDir(t, "docker-compose.yml")
	up, err := u.Up(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}

	d, calls, _ := newTestDowner(u.Reg)
	res, err := d.Down(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Registered || res.Project != up.Project || res.ID != up.ID {
		t.Errorf("result = %+v, want registered world %s/%s", res, up.ID, up.Project)
	}
	st := load(t, u.Reg)
	if len(st.Worlds) != 0 || len(st.Leases) != 0 {
		t.Errorf("registry not empty after down: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(dir, ".worldtree")); !os.IsNotExist(err) {
		t.Errorf(".worldtree still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "docker-compose.yml")); err != nil {
		t.Errorf("user's compose file touched: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("compose calls = %v", *calls)
	}
	c := (*calls)[0]
	if c.args != "compose -p "+up.Project+" down -v --remove-orphans" {
		t.Errorf("compose args = %q", c.args)
	}
	if c.dir == dir {
		t.Error("compose down ran in the world dir; it must run where no compose file is found")
	}
}

func TestDownUnknownWorldIsIdempotent(t *testing.T) {
	u, _ := newTestUpper(t, colliding)
	d, calls, _ := newTestDowner(u.Reg)
	dir := filepath.Join(t.TempDir(), "gone") // never existed
	res, err := d.Down(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Registered {
		t.Error("unknown world reported as registered")
	}
	if want := ProjectName("gone", ID(dir)); res.Project != want || !strings.Contains((*calls)[0].args, want) {
		t.Errorf("project = %q, calls = %v; want %q (same name up would use)", res.Project, *calls, want)
	}
	if _, err := d.Down(context.Background(), dir); err != nil {
		t.Errorf("second down: %v", err)
	}
}

func TestDownAfterWorktreeDeleted(t *testing.T) {
	u, _ := newTestUpper(t, colliding)
	dir := projectDir(t, "docker-compose.yml")
	if _, err := u.Up(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	d, _, _ := newTestDowner(u.Reg)
	res, err := d.Down(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Registered || len(load(t, u.Reg).Worlds) != 0 {
		t.Errorf("deleted worktree's world not removed: %+v", res)
	}
}

func TestDownFailureLeavesRemovingThenRetrySucceeds(t *testing.T) {
	u, _ := newTestUpper(t, colliding)
	dir := projectDir(t, "docker-compose.yml")
	up, err := u.Up(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	d, _, _ := newTestDowner(u.Reg)
	d.Run = func(context.Context, string, io.Writer, ...string) error { return errors.New("daemon down") }
	if _, err := d.Down(context.Background(), dir); err == nil {
		t.Fatal("expected error")
	}
	st := load(t, u.Reg)
	if w := st.Worlds[up.ID]; w == nil || w.Phase != registry.Removing {
		t.Fatalf("world = %+v, want phase removing", w)
	}
	if len(st.LeasesOf(up.ID)) == 0 {
		t.Error("leases released although compose down failed")
	}

	d2, _, _ := newTestDowner(u.Reg)
	if _, err := d2.Down(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if len(load(t, u.Reg).Worlds) != 0 {
		t.Error("retry did not remove the world")
	}
}

func TestDownSweepsLabelledLeftovers(t *testing.T) {
	u, _ := newTestUpper(t, colliding)
	d, _, fq := newTestDowner(u.Reg)
	fq.answers["ps -aq"] = "c1\nc2\n"
	fq.answers["volume ls"] = "v1\n"
	res, err := d.Down(context.Background(), filepath.Join(t.TempDir(), "w"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Swept != 3 {
		t.Errorf("swept %d, want 3", res.Swept)
	}
	got := strings.Join(fq.calls, "\n")
	for _, want := range []string{"rm -f c1 c2", "volume rm -f v1"} {
		if !strings.Contains(got, want) {
			t.Errorf("docker calls lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "network rm") {
		t.Error("network rm called with nothing to remove")
	}
}

// permDenied fails the first n RemoveAll calls with a permission error, like a postgres
// data dir chowned to the container's uid with mode 0700.
func permDenied(n int) func(string) error {
	return func(p string) error {
		if n > 0 {
			n--
			return &fs.PathError{Op: "unlinkat", Path: p + "/binds/db/pgdata", Err: fs.ErrPermission}
		}
		return os.RemoveAll(p)
	}
}

func TestDownDeletesContainerOwnedFilesFromAContainer(t *testing.T) {
	u, _ := newTestUpper(t, colliding)
	dir := projectDir(t, "docker-compose.yml")
	up, err := u.Up(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	d, calls, _ := newTestDowner(u.Reg)
	d.RemoveAll = permDenied(1)
	if _, err := d.Down(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	gen := filepath.Join(dir, ".worldtree")
	want := "run --rm --name " + CleanupContainer(up.ID) + " --label dev.worldtree.world=" + up.ID +
		" -v " + gen + ":/w " + CleanupImage + " find /w -mindepth 1 -delete"
	if len(*calls) != 2 || (*calls)[1].args != want {
		t.Fatalf("docker calls = %v\nwant second = %q", *calls, want)
	}
	if len(load(t, u.Reg).Worlds) != 0 {
		t.Error("world not removed after container cleanup")
	}
}

func TestDownKeepsRecordWhenCleanupFails(t *testing.T) {
	u, _ := newTestUpper(t, colliding)
	dir := projectDir(t, "docker-compose.yml")
	up, err := u.Up(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	d, _, _ := newTestDowner(u.Reg)
	d.RemoveAll = permDenied(10)
	var removed []string
	d.Run = func(ctx context.Context, _ string, _ io.Writer, args ...string) error {
		switch args[0] {
		case "run":
			return errors.New("cannot pull alpine")
		case "rm":
			if ctx.Err() != nil {
				t.Error("cleanup container removed with a cancelled context")
			}
			removed = append(removed, strings.Join(args, " "))
		}
		return nil
	}
	if _, err := d.Down(context.Background(), dir); err == nil || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("err = %v, want permission error", err)
	}
	st := load(t, u.Reg)
	if w := st.Worlds[up.ID]; w == nil || w.Phase != registry.Removing || len(st.LeasesOf(up.ID)) == 0 {
		t.Errorf("world = %+v leases = %v; want removing with leases kept", w, st.LeasesOf(up.ID))
	}
	if want := "rm -f " + CleanupContainer(up.ID); len(removed) != 1 || removed[0] != want {
		t.Errorf("after failed cleanup removed %v, want [%q]", removed, want)
	}
}

func TestUpRemovesStaleCleanupContainerOfRemovingWorld(t *testing.T) {
	u, fd := newTestUpper(t, colliding)
	dir := projectDir(t, "docker-compose.yml")
	if _, err := u.Up(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	id := ID(dir)
	// A down died mid-cleanup: the world is still removing.
	if err := u.Reg.Update(func(s *registry.State) error {
		w := *s.Worlds[id]
		w.Phase = registry.Removing
		s.Put(w, time.Unix(0, 0))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fd.calls = nil
	if _, err := u.Up(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if len(fd.calls) != 2 || strings.Join(fd.calls[0], " ") != "rm -f "+CleanupContainer(id) {
		t.Errorf("docker calls = %v; want stale cleanup container removed before compose up", fd.calls)
	}
}

func TestDownOtherRemoveErrorsDoNotStartContainer(t *testing.T) {
	u, _ := newTestUpper(t, colliding)
	d, calls, _ := newTestDowner(u.Reg)
	d.RemoveAll = func(string) error { return errors.New("disk on fire") }
	if _, err := d.Down(context.Background(), projectDir(t, "docker-compose.yml")); err == nil {
		t.Fatal("expected error")
	}
	for _, c := range *calls {
		if strings.HasPrefix(c.args, "run ") {
			t.Errorf("cleanup container started for a non-permission error: %v", c.args)
		}
	}
}

func TestDownWaitsForBusyWorld(t *testing.T) {
	u, _ := newTestUpper(t, colliding)
	dir := projectDir(t, "docker-compose.yml")
	unlock, err := u.Reg.LockWorld(ID(dir), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	d, calls, _ := newTestDowner(u.Reg)
	d.LockTimeout = 50 * time.Millisecond
	if _, err := d.Down(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("err = %v, want busy", err)
	}
	if len(*calls) != 0 {
		t.Errorf("docker called while world busy: %v", *calls)
	}
}

func TestListShowsWorldsPortsContainersAndOrphans(t *testing.T) {
	u, _ := newTestUpper(t, colliding)
	a, err := u.Up(context.Background(), projectDir(t, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := u.Up(context.Background(), projectDir(t, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(b.Dir); err != nil {
		t.Fatal(err)
	}
	fq := &fakeQuery{answers: map[string]string{
		"ps -a": a.ID + " running\n" + a.ID + " exited\n" + b.ID + " running\n",
	}}
	ws, err := List(context.Background(), u.Reg, fq.query)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 {
		t.Fatalf("listed %d worlds", len(ws))
	}
	byID := map[string]Status{}
	for _, w := range ws {
		byID[w.ID] = w
	}
	sa, sb := byID[a.ID], byID[b.ID]
	if !sa.DockerOK || sa.Running != 1 || sa.Containers != 2 || sa.DirMissing {
		t.Errorf("world a status = %+v", sa)
	}
	if !sb.DirMissing || sb.Running != 1 {
		t.Errorf("world b status = %+v, want dir missing", sb)
	}
	if got := FormatPorts(sa.Ports); !strings.HasPrefix(got, "db:") {
		t.Errorf("ports = %q, want db:<port>", got)
	}

	fq.fail = errors.New("no docker")
	ws, err = List(context.Background(), u.Reg, fq.query)
	if err != nil || len(ws) != 2 || ws[0].DockerOK {
		t.Errorf("without docker: %v %+v", err, ws)
	}
}
