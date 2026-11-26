package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/iam-mkd/WorldTree/internal/compose"
)

func runRender(t *testing.T, fixture string) (stdout, stderr string, err error) {
	t.Helper()
	if _, verr := compose.CheckVersion(context.Background()); verr != nil {
		t.Skipf("docker compose unavailable: %v", verr)
	}
	cmd := NewRootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"render", "../../testdata/compose/" + fixture, "--port-base", "23000"})
	err = cmd.Execute()
	return out.String(), errb.String(), err
}

func TestRenderSpringNotes(t *testing.T) {
	out, errOut, err := runRender(t, "spring-notes")
	if err != nil {
		t.Fatalf("%v\n%s", err, errOut)
	}
	for _, want := range []string{"container_name: !reset null", "ports: !override", `published: "23000"`, "dev.worldtree.world:"} {
		if !strings.Contains(out, want) {
			t.Errorf("override lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(errOut, "postgres             *:5432 -> 23001") {
		t.Errorf("port table missing:\n%s", errOut)
	}
}

func TestRenderFailsOnUnsupported(t *testing.T) {
	_, errOut, err := runRender(t, "edge")
	if err == nil {
		t.Fatal("want error for host networking / fixed subnet")
	}
	if !strings.Contains(errOut, "network-mode-host") || !strings.Contains(errOut, "fixed-subnet") {
		t.Errorf("findings missing:\n%s", errOut)
	}
}
