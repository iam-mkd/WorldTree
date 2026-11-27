package compose

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// go test ./internal/compose -update    re-captures config.json (needs the docker CLI,
// not a daemon) and rewrites the golden override.yml / findings.txt files.
var update = flag.Bool("update", false, "re-capture compose config JSON and rewrite golden files")

// fixtures live in <repo>/testdata/compose; their captured config and goldens in ./testdata.
var fixtures = []string{"spring-notes", "edge", "edge-names", "shape-immich", "shape-mastodon", "shape-paperless"}

const fakeRoot = "/work" // captured JSON has the fixture's absolute dir replaced by /work/<fixture>

func fixtureDir(t *testing.T, name string) string {
	t.Helper()
	d, err := filepath.Abs(filepath.Join("..", "..", "testdata", "compose", name))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func testOptions(dir string) Options {
	return Options{
		WorldID:    "0123abcd",
		Project:    "wt-" + filepath.Base(dir) + "-0123abcd",
		ProjectDir: dir,
		IsFile:     func(p string) bool { return strings.HasSuffix(p, ".conf") },
	}
}

// sequentialLeases stands in for the port allocator (day 3).
func sequentialLeases(p *Plan, base int) map[string]int {
	m := map[string]int{}
	for i, r := range p.Ports {
		m[r.Key] = base + i
	}
	return m
}

func renderFindings(p *Plan) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "ports to lease:\n")
	for _, r := range p.Ports {
		fmt.Fprintf(&b, "  %s (published %s)\n", r.Key, r.Port.Published)
	}
	fmt.Fprintf(&b, "findings:\n")
	for _, f := range p.Findings {
		fmt.Fprintf(&b, "  %s\n", f)
	}
	return b.Bytes()
}

func TestGolden(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			golden := filepath.Join("testdata", name)
			cfgPath := filepath.Join(golden, "config.json")
			if *update {
				dir := fixtureDir(t, name)
				files, err := DefaultFiles(dir)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := LoadConfig(context.Background(), dir, files)
				if err != nil {
					t.Fatal(err)
				}
				raw = bytes.ReplaceAll(raw, []byte(filepath.ToSlash(fixtureDir(t, name))), []byte(fakeRoot+"/"+name))
				writeFile(t, cfgPath, raw)
			}
			raw, err := os.ReadFile(cfgPath)
			if err != nil {
				t.Fatalf("%v (run with -update to capture)", err)
			}
			cfg, err := ParseConfig(raw)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := NewPlan(cfg, testOptions(fakeRoot+"/"+name))
			if err != nil {
				t.Fatal(err)
			}
			out, err := plan.Render(sequentialLeases(plan, 20000))
			if err != nil {
				t.Fatal(err)
			}
			compareGolden(t, filepath.Join(golden, "override.yml"), out)
			compareGolden(t, filepath.Join(golden, "findings.txt"), renderFindings(plan))
		})
	}
}

func compareGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		writeFile(t, path, got)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from golden (re-run with -update if intended)\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
