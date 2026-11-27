package envfile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/iam-mkd/WorldTree/internal/compose"
)

func strp(s string) *string { return &s }

func springNotes() World {
	return World{
		ID: "abcd1234", Project: "wt-notes-abcd1234",
		Config: &compose.Config{Services: map[string]compose.Service{
			"app":      {Image: "notes-app"},
			"postgres": {Image: "postgres:17", Environment: map[string]*string{"POSTGRES_USER": strp("notes"), "POSTGRES_PASSWORD": strp("p@ss/w:rd"), "POSTGRES_DB": strp("notes")}},
			"redis":    {Image: "docker.io/valkey/valkey:9@sha256:abc"},
		}},
		Ports: []compose.PortRequest{
			{Key: "redis//6379/tcp", Service: "redis", Port: compose.Port{Target: 6379, Protocol: "tcp"}},
			{Key: "app//8080/tcp", Service: "app", Port: compose.Port{Target: 8080, Protocol: "tcp"}},
			{Key: "postgres/127.0.0.1/5432/tcp", Service: "postgres", Port: compose.Port{HostIP: "127.0.0.1", Target: 5432, Protocol: "tcp"}},
		},
		Leases: map[string]int{"redis//6379/tcp": 21002, "app//8080/tcp": 21000, "postgres/127.0.0.1/5432/tcp": 21001},
	}
}

func TestBuildSpringNotes(t *testing.T) {
	got := Build(springNotes())
	want := []Var{
		{"WORLDTREE_ID", "abcd1234"},
		{"WORLDTREE_PROJECT", "wt-notes-abcd1234"},
		{"WORLDTREE_PORT_APP_8080", "21000"},
		{"WORLDTREE_PORT_POSTGRES_5432", "21001"},
		{"WORLDTREE_PORT_REDIS_6379", "21002"},
		{"DATABASE_URL", "postgres://notes:p%40ss%2Fw%3Ard@127.0.0.1:21001/notes"},
		{"SPRING_DATASOURCE_URL", "jdbc:postgresql://127.0.0.1:21001/notes"},
		{"SPRING_DATASOURCE_USERNAME", "notes"},
		{"SPRING_DATASOURCE_PASSWORD", "p@ss/w:rd"},
		{"REDIS_URL", "redis://127.0.0.1:21002"},
		{"SPRING_DATA_REDIS_HOST", "127.0.0.1"},
		{"SPRING_DATA_REDIS_PORT", "21002"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Build:\n got %v\nwant %v", got, want)
	}
}

func TestBuildDefaultsAndNoDatabase(t *testing.T) {
	w := springNotes()
	pg := w.Config.Services["postgres"]
	pg.Environment = map[string]*string{"POSTGRES_PASSWORD": strp("x")} // user/db default to postgres
	w.Config.Services["postgres"] = pg
	vars := map[string]string{}
	for _, v := range Build(w) {
		vars[v.Key] = v.Value
	}
	if vars["DATABASE_URL"] != "postgres://postgres:x@127.0.0.1:21001/postgres" {
		t.Errorf("defaults: DATABASE_URL = %q", vars["DATABASE_URL"])
	}

	w.Ports = w.Ports[:2] // postgres no longer publishes 5432
	for _, v := range Build(w) {
		if strings.HasPrefix(v.Key, "DATABASE_URL") || strings.HasPrefix(v.Key, "SPRING_DATASOURCE") {
			t.Errorf("unpublished database still produced %s", v.Key)
		}
	}
}

func TestIPv6HostIsBracketed(t *testing.T) {
	w := springNotes()
	w.Ports[0].Port.HostIP = "::1"
	for _, v := range Build(w) {
		if v.Key == "REDIS_URL" && v.Value != "redis://[::1]:21002" {
			t.Errorf("REDIS_URL = %q", v.Value)
		}
	}
}

func TestRenderRoundTripsThroughComposeParser(t *testing.T) {
	vars := []Var{
		{"PLAIN", "abc"},
		{"SPACES_HASH", "a b # not a comment"},
		{"DOLLAR", "pa$$word ${HOME} $USER"},
		{"QUOTES", `it's "quoted" \back\slash $x`},
		{"EMPTY", ""},
	}
	got, err := Parse(Render(vars))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, vars) {
		t.Errorf("round trip:\n got %q\nwant %q\nfile:\n%s", got, vars, Render(vars))
	}
}

func TestFormat(t *testing.T) {
	vars := []Var{{"A", "it's"}}
	for format, want := range map[string]string{
		Sh:         `export A='it'\''s'` + "\n",
		PowerShell: `$env:A = 'it''s'` + "\n",
		Dotenv:     `A="it's"` + "\n",
	} {
		got, err := Format(vars, format)
		if err != nil || got != want {
			t.Errorf("%s: got %q, %v; want %q", format, got, err, want)
		}
	}
	if _, err := Format(vars, "fish"); err == nil {
		t.Error("unknown format accepted")
	}
}
