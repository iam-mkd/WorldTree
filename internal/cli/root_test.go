package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want := "worldtree dev (none) " + runtime.GOOS + "/" + runtime.GOARCH
	if got := strings.TrimSpace(out.String()); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"version", "extra"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for extra argument")
	}
}
