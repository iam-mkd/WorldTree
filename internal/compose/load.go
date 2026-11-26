package compose

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// MinVersion is the oldest Compose that understands !reset and !override.
var MinVersion = [2]int{2, 24}

// LoadConfig runs `docker compose config --format json` in dir with every profile
// enabled, so profiled services are rewritten too (activation is unchanged).
// Compose resolves include, extends, anchors, .env interpolation and relative paths.
// The output can contain resolved secrets: callers keep it in memory only.
// It does not need a running Docker daemon.
func LoadConfig(ctx context.Context, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "--project-directory", dir,
		"--profile", "*", "config", "--format", "json")
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker compose config in %s: %w: %s", dir, err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// CheckVersion returns the installed Compose version, or an error if it is
// missing or older than MinVersion.
func CheckVersion(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "compose", "version", "--short").Output()
	if err != nil {
		return "", fmt.Errorf("docker compose not found (need >= %d.%d): %w", MinVersion[0], MinVersion[1], err)
	}
	v := strings.TrimSpace(string(out))
	ok, err := versionAtLeast(v, MinVersion[0], MinVersion[1])
	if err != nil {
		return v, err
	}
	if !ok {
		return v, fmt.Errorf("docker compose %s is too old: worldtree needs >= %d.%d for !reset/!override", v, MinVersion[0], MinVersion[1])
	}
	return v, nil
}

// versionAtLeast parses "v2.24.0", "2.29.1-desktop.1" or "5.7.1".
func versionAtLeast(v string, major, minor int) (bool, error) {
	parts := strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3)
	if len(parts) < 2 {
		return false, fmt.Errorf("cannot parse compose version %q", v)
	}
	ma, err1 := strconv.Atoi(parts[0])
	mi, err2 := strconv.Atoi(strings.SplitN(parts[1], "-", 2)[0])
	if err1 != nil || err2 != nil {
		return false, fmt.Errorf("cannot parse compose version %q", v)
	}
	return ma > major || (ma == major && mi >= minor), nil
}
