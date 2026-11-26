// Package world holds world identity and (from day 4) the up/down state machine.
package world

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"
)

// ID is the world id: the first 8 hex chars of sha256 of the absolute, cleaned dir.
// Same dir, same id, so re-running `up` reconciles instead of creating a new world.
func ID(absDir string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(filepath.Clean(absDir))))
	return hex.EncodeToString(sum[:])[:8]
}

var unsafe = regexp.MustCompile(`[^a-z0-9_-]+`)

// ProjectName returns the compose project name wt-<name>-<id>. name is usually
// <repo>-<branch>; it is lowercased, reduced to compose's allowed characters and
// truncated so the result stays readable in `docker ps`.
func ProjectName(name, id string) string {
	s := unsafe.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-_")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-_")
	}
	if s == "" {
		s = "world"
	}
	return "wt-" + s + "-" + id
}
