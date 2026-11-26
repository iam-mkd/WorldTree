package world

import "testing"

func TestIDIsStableAndShort(t *testing.T) {
	a, b := ID("/w/app"), ID("/w/app/")
	if a != b || len(a) != 8 {
		t.Errorf("ID not stable/short: %q %q", a, b)
	}
	if ID("/w/app") == ID("/w/app2") {
		t.Error("different dirs, same id")
	}
}

func TestProjectName(t *testing.T) {
	for in, want := range map[string]string{
		"WorldTree-feat/Login": "wt-worldtree-feat-login-abcd1234",
		"__":                   "wt-world-abcd1234",
		"spring.notes_main":    "wt-spring-notes_main-abcd1234",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbb": "wt-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-abcd1234",
	} {
		if got := ProjectName(in, "abcd1234"); got != want {
			t.Errorf("ProjectName(%q) = %q, want %q", in, got, want)
		}
	}
}
