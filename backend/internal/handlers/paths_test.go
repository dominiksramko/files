package handlers

import (
	"path/filepath"
	"testing"
)

func TestSafePath(t *testing.T) {
	const root = "/srv/users/ann"
	tests := []struct {
		rel  string
		want string // "" when the path must be rejected
	}{
		{"", root},
		{".", root},
		{"docs/plan.md", root + "/docs/plan.md"},
		{"/docs/plan.md", root + "/docs/plan.md"}, // leading slash stays inside root
		{"docs/../notes.txt", root + "/notes.txt"},
		{"../ann/notes.txt", root + "/notes.txt"}, // back into the same folder
		{"..notes", root + "/..notes"},            // a name starting with dots, not a parent
		{"..", ""},
		{"../anna", ""}, // neighbour whose name starts with "ann"
		{"../anna/secret.txt", ""},
		{"../ann-backup/file", ""},
		{"docs/../../anna/secret.txt", ""},
		{"../../../etc/passwd", ""},
	}
	for _, test := range tests {
		got, err := safePath(root, test.rel)
		if test.want == "" {
			if err == nil {
				t.Errorf("safePath(%q) = %q, want an error", test.rel, got)
			}
			continue
		}
		if err != nil || got != test.want {
			t.Errorf("safePath(%q) = %q, %v; want %q", test.rel, got, err, test.want)
		}
	}
}

func TestSafePathTrailingSlashRoot(t *testing.T) {
	got, err := safePath("/srv/users/ann/", "../anna/secret.txt")
	if err == nil {
		t.Errorf("safePath with a trailing-slash root reached %q", got)
	}
}

func TestSafePathFilesystemRoot(t *testing.T) {
	// A user whose home is "/" may reach anything, and nothing escapes it.
	for rel, want := range map[string]string{"etc/hosts": "/etc/hosts", "..": "/", "": "/"} {
		if got, err := safePath("/", rel); err != nil || got != want {
			t.Errorf("safePath(/, %q) = %q, %v; want %q", rel, got, err, want)
		}
	}
}

func TestSafePathWithoutRoot(t *testing.T) {
	if _, err := safePath("", "notes.txt"); err == nil {
		t.Error("safePath with no root configured succeeded")
	}
}

func TestSafeChildPath(t *testing.T) {
	const root = "/srv/users/ann"
	for _, rel := range []string{"", ".", "/", "docs/..", "../ann", "../anna", "../anna/secret.txt", ".."} {
		if got, err := safeChildPath(root, rel); err == nil {
			t.Errorf("safeChildPath(%q) = %q, want an error", rel, got)
		}
	}
	for rel, want := range map[string]string{"notes.txt": root + "/notes.txt", "docs/plan.md": root + "/docs/plan.md"} {
		if got, err := safeChildPath(root, rel); err != nil || got != want {
			t.Errorf("safeChildPath(%q) = %q, %v; want %q", rel, got, err, want)
		}
	}
}

func TestSafeChildPaths(t *testing.T) {
	const root = "/srv/users/ann"
	got, err := safeChildPaths(root, []string{"notes.txt", "docs"})
	if err != nil || len(got) != 2 || got[0] != filepath.Join(root, "notes.txt") || got[1] != filepath.Join(root, "docs") {
		t.Errorf("safeChildPaths = %q, %v", got, err)
	}
	if got, err := safeChildPaths(root, []string{"notes.txt", ""}); err == nil {
		t.Errorf("safeChildPaths with root among the paths = %q, want an error", got)
	}
}

func TestIsWithin(t *testing.T) {
	tests := []struct {
		dir, abs string
		want     bool
	}{
		{"/srv/users/ann", "/srv/users/ann", true},
		{"/srv/users/ann", "/srv/users/ann/docs", true},
		{"/srv/users/ann", "/srv/users/anna", false},
		{"/srv/users/ann", "/srv/users", false},
		{"/srv/users/ann", "/srv/users/ann/..data", true},
		{"/", "/etc", true},
	}
	for _, test := range tests {
		if got := isWithin(test.dir, test.abs); got != test.want {
			t.Errorf("isWithin(%q, %q) = %v, want %v", test.dir, test.abs, got, test.want)
		}
	}
}
