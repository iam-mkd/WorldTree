package compose

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func touch(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDefaultFiles(t *testing.T) {
	t.Setenv("COMPOSE_FILE", "")
	for _, tc := range []struct {
		name  string
		files []string
		want  []string
	}{
		{"compose.yaml wins", []string{"docker-compose.yml", "compose.yaml"}, []string{"compose.yaml"}},
		{"legacy name", []string{"docker-compose.yml"}, []string{"docker-compose.yml"}},
		{"override kept", []string{"docker-compose.yml", "docker-compose.override.yml"}, []string{"docker-compose.yml", "docker-compose.override.yml"}},
		{"override yaml", []string{"compose.yaml", "compose.override.yaml"}, []string{"compose.yaml", "compose.override.yaml"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			touch(t, dir, tc.files...)
			got, err := DefaultFiles(dir)
			if err != nil {
				t.Fatal(err)
			}
			var want []string
			for _, w := range tc.want {
				want = append(want, filepath.Join(dir, w))
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

func TestDefaultFilesNone(t *testing.T) {
	t.Setenv("COMPOSE_FILE", "")
	if _, err := DefaultFiles(t.TempDir()); err == nil {
		t.Error("expected error for dir without compose file")
	}
}

func TestDefaultFilesHonoursComposeFile(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(t.TempDir(), "other.yml")
	t.Setenv("COMPOSE_PATH_SEPARATOR", ",")
	t.Setenv("COMPOSE_FILE", "base.yml,"+abs)
	got, err := DefaultFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "base.yml"), abs}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
