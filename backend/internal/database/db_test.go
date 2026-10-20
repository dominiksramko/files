package database

import (
	"os"
	"path/filepath"
	"testing"
)

func openWithEnv(t *testing.T, env map[string]string) string {
	t.Helper()
	for key, value := range env {
		t.Setenv(key, value)
	}
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var rootDir string
	if err := db.QueryRow(`SELECT root_dir FROM users WHERE is_admin = 1`).Scan(&rootDir); err != nil {
		t.Fatal(err)
	}
	return rootDir
}

func TestAdminGetsAFolderByDefault(t *testing.T) {
	usersRoot := filepath.Join(t.TempDir(), "users")
	rootDir := openWithEnv(t, map[string]string{"USERS_ROOT": usersRoot, "ADMIN_USERNAME": "boss", "ADMIN_PATH": ""})
	if want := filepath.Join(usersRoot, "boss"); rootDir != want {
		t.Errorf("admin folder = %q, want %q", rootDir, want)
	}
	if info, err := os.Stat(rootDir); err != nil || !info.IsDir() {
		t.Errorf("admin folder not created: %v", err)
	}
}

func TestAdminPathIsUsedAsGiven(t *testing.T) {
	given := filepath.Join(t.TempDir(), "my files")
	rootDir := openWithEnv(t, map[string]string{"USERS_ROOT": t.TempDir(), "ADMIN_PATH": given})
	if rootDir != given {
		t.Errorf("admin folder = %q, want %q", rootDir, given)
	}
	if _, err := os.Stat(given); !os.IsNotExist(err) {
		t.Error("ADMIN_PATH was created; it should be used as given")
	}
}
