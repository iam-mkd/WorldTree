package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// TestComposeAcceptsOverride renders overrides for two worlds of every fixture and
// feeds each back through the real `docker compose config` (needs the docker CLI with
// Compose >= 2.24, not a daemon). It checks what compose itself will do, not our YAML:
//   - no fixed container_name survives
//   - every published host port is a leased one, and the two worlds share none
//   - every service, non-external volume and network carries the world label
//   - the two worlds share no project, volume or network name (except external ones)
func TestComposeAcceptsOverride(t *testing.T) {
	if _, err := CheckVersion(context.Background()); err != nil {
		t.Skipf("docker compose unavailable: %v", err)
	}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			dir := fixtureDir(t, name)
			a := mergedWorld(t, dir, "aaaa1111", 21000)
			b := mergedWorld(t, dir, "bbbb2222", 22000)

			if a.Name == b.Name {
				t.Errorf("both worlds use project %q", a.Name)
			}
			portsA, portsB := publishedPorts(a), publishedPorts(b)
			for p := range portsA {
				if portsB[p] {
					t.Errorf("host port %s published by both worlds", p)
				}
			}
			namesA := resourceNames(a)
			for n := range resourceNames(b) {
				if namesA[n] {
					t.Errorf("volume/network %q shared by both worlds", n)
				}
			}
		})
	}
}

func mergedWorld(t *testing.T, dir, id string, base int) *Config {
	t.Helper()
	ctx := context.Background()
	raw, err := LoadConfig(ctx, dir, composeFiles(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{WorldID: id, Project: "wt-" + filepath.Base(dir) + "-" + id, ProjectDir: dir}
	plan, err := NewPlan(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	leases := sequentialLeases(plan, base)
	override, err := plan.Render(leases)
	if err != nil {
		t.Fatal(err)
	}
	ovPath := filepath.Join(t.TempDir(), "override.yml")
	if err := os.WriteFile(ovPath, override, 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"compose", "--project-directory", dir}
	for _, f := range composeFiles(t, dir) {
		args = append(args, "-f", f)
	}
	args = append(args, "-f", ovPath, "--profile", "*", "config", "--format", "json")
	cmd := exec.CommandContext(ctx, "docker", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("compose rejected the override: %v\n%s\noverride:\n%s", err, errb.String(), override)
	}
	merged, err := ParseConfig(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if merged.Name != opts.Project {
		t.Errorf("project = %q, want %q", merged.Name, opts.Project)
	}
	leased := map[string]bool{}
	for _, p := range leases {
		leased[strconv.Itoa(p)] = true
	}
	for sname, s := range merged.Services {
		if s.ContainerName != "" {
			t.Errorf("%s: container_name %q survived", sname, s.ContainerName)
		}
		if s.NetworkMode == "host" {
			continue
		}
		for _, p := range s.Ports {
			if p.Published != "" && !leased[p.Published] {
				t.Errorf("%s: published port %s is not a leased port", sname, p.Published)
			}
		}
	}
	labels := mergedLabels(t, out.Bytes())
	for key, v := range labels {
		if v != id {
			t.Errorf("%s: label %s = %q, want %q", key, LabelWorld, v, id)
		}
	}
	for sname := range merged.Services {
		if _, ok := labels["service/"+sname]; !ok {
			t.Errorf("service %s has no world label", sname)
		}
	}
	for k, v := range merged.Volumes {
		if _, ok := labels["volume/"+k]; !ok && !v.External {
			t.Errorf("volume %s has no world label", k)
		}
	}
	for k, n := range merged.Networks {
		if _, ok := labels["network/"+k]; !ok && !n.External {
			t.Errorf("network %s has no world label", k)
		}
	}
	return merged
}

func composeFiles(t *testing.T, dir string) []string {
	t.Helper()
	for _, f := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return []string{filepath.Join(dir, f)}
		}
	}
	t.Fatalf("no compose file in %s", dir)
	return nil
}

func publishedPorts(c *Config) map[string]bool {
	m := map[string]bool{}
	for _, s := range c.Services {
		if s.NetworkMode == "host" {
			continue
		}
		for _, p := range s.Ports {
			if p.Published != "" {
				m[fmt.Sprintf("%s:%s/%s", p.HostIP, p.Published, p.Protocol)] = true
			}
		}
	}
	return m
}

func resourceNames(c *Config) map[string]bool {
	m := map[string]bool{}
	for _, v := range c.Volumes {
		if !v.External {
			m[v.Name] = true
		}
	}
	for _, n := range c.Networks {
		if !n.External {
			m[n.Name] = true
		}
	}
	return m
}

// mergedLabels returns "service/<name>", "volume/<key>", "network/<key>" -> world label value.
func mergedLabels(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	type labelled struct {
		Labels map[string]string `json:"labels"`
	}
	var doc struct {
		Services map[string]labelled `json:"services"`
		Volumes  map[string]labelled `json:"volumes"`
		Networks map[string]labelled `json:"networks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for kind, m := range map[string]map[string]labelled{"service": doc.Services, "volume": doc.Volumes, "network": doc.Networks} {
		for k, v := range m {
			if l, ok := v.Labels[LabelWorld]; ok {
				out[kind+"/"+k] = l
			}
		}
	}
	return out
}
