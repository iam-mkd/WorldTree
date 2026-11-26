package compose

import (
	"strings"
	"testing"
)

func TestVersionAtLeast(t *testing.T) {
	for v, want := range map[string]bool{
		"2.24.0": true, "v2.24.0": true, "2.29.1-desktop.1": true, "5.7.1": true,
		"2.23.3": false, "1.29.2": false, "2.24-rc1": true,
	} {
		got, err := versionAtLeast(v, 2, 24)
		if err != nil || got != want {
			t.Errorf("versionAtLeast(%q) = %v, %v; want %v", v, got, err, want)
		}
	}
	if _, err := versionAtLeast("garbage", 2, 24); err == nil {
		t.Error("want error for unparseable version")
	}
}

func TestWithin(t *testing.T) {
	cases := []struct {
		p, dir string
		want   bool
	}{
		{"/w/app/data", "/w/app", true},
		{"/w/app", "/w/app", false},
		{"/w/application", "/w/app", false}, // prefix of the name, not the path
		{"/w/app/../other", "/w/app", false},
		{"/dev/shm", "/dev", true},
	}
	for _, c := range cases {
		if got := within(c.p, c.dir); got != c.want {
			t.Errorf("within(%q, %q) = %v, want %v", c.p, c.dir, got, c.want)
		}
	}
}

func TestNewPlanRequiresIdentity(t *testing.T) {
	if _, err := NewPlan(&Config{Name: "x"}, Options{Project: "p", ProjectDir: "/w"}); err == nil {
		t.Error("want error without WorldID")
	}
}

func TestRenderRefusesMissingLease(t *testing.T) {
	cfg := &Config{Name: "x", Services: map[string]Service{
		"db": {Ports: []Port{{Target: 5432, Published: "5432", Protocol: "tcp"}}},
	}}
	p, err := NewPlan(cfg, Options{WorldID: "id", Project: "wt-x-id", ProjectDir: "/w/x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Render(map[string]int{}); err == nil || !strings.Contains(err.Error(), "db//5432/tcp") {
		t.Errorf("want missing-lease error naming the key, got %v", err)
	}
}

func TestDuplicatePortKeysAreDistinct(t *testing.T) {
	cfg := &Config{Name: "x", Services: map[string]Service{
		"web": {Ports: []Port{
			{Target: 80, Published: "80", Protocol: "tcp"},
			{Target: 80, Published: "8080", Protocol: "tcp"},
		}},
	}}
	p, err := NewPlan(cfg, Options{WorldID: "id", Project: "wt-x-id", ProjectDir: "/w/x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Ports) != 2 || p.Ports[0].Key == p.Ports[1].Key {
		t.Fatalf("want 2 distinct keys, got %+v", p.Ports)
	}
	if p.Ports[1].Key != "web//80/tcp#2" {
		t.Errorf("second key = %q", p.Ports[1].Key)
	}
}

func TestBindInsideProjectIsUntouched(t *testing.T) {
	cfg := &Config{Name: "x", Services: map[string]Service{
		"db": {Volumes: []Mount{{Type: "bind", Source: "/w/x/pgdata", Target: "/var/lib/postgresql/data"}}},
	}}
	p, err := NewPlan(cfg, Options{WorldID: "id", Project: "wt-x-id", ProjectDir: "/w/x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.services["db"].binds) != 0 || len(p.Findings) != 0 {
		t.Errorf("bind inside project should be left alone: %+v %+v", p.services["db"], p.Findings)
	}
}
