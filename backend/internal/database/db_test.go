package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"files/internal/database/migrations"
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

// addUserRows gives the user one row in every table that belongs to a user.
func addUserRows(t *testing.T, db *sql.DB, userID int64) {
	t.Helper()
	for _, statement := range []string{
		`INSERT INTO folder_prefs (user_id, path) VALUES (?, 'docs')`,
		`INSERT INTO watch_progress (user_id, file_path, position, duration, updated_at) VALUES (?, 'film.mkv', 1, 2, 'now')`,
		`INSERT INTO rename_presets (user_id, name, template, created_at) VALUES (?, 'dated', '{date}', 'now')`,
		`INSERT INTO sessions (token_hash, user_id, created_at, last_used_at, expires_at) VALUES ('hash-' || ?, ?, 0, 0, 0)`,
	} {
		args := []any{userID}
		if strings.Count(statement, "?") == 2 {
			args = append(args, userID)
		}
		if _, err := db.Exec(statement, args...); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

// userRowCount counts the rows of userID across the tables that belong to a user.
func userRowCount(t *testing.T, db *sql.DB, userID int64) int {
	t.Helper()
	total := 0
	for _, table := range []string{"folder_prefs", "watch_progress", "rename_presets", "sessions"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE user_id = ?`, userID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		total += count
	}
	return total
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	t.Setenv("USERS_ROOT", filepath.Join(t.TempDir(), "users"))
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func createUser(t *testing.T, db *sql.DB, username string) int64 {
	t.Helper()
	result, err := db.Exec(`INSERT INTO users (username, password, root_dir, is_admin) VALUES (?, 'hash', '/tmp', 0)`, username)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	return id
}

func TestDeletingAUserRemovesTheirRows(t *testing.T) {
	db := openTestDB(t)
	ann, anna := createUser(t, db, "ann"), createUser(t, db, "anna")
	addUserRows(t, db, ann)
	addUserRows(t, db, anna)

	if _, err := db.Exec(`DELETE FROM users WHERE id = ?`, ann); err != nil {
		t.Fatal(err)
	}
	if count := userRowCount(t, db, ann); count != 0 {
		t.Errorf("%d rows of the deleted user left", count)
	}
	if count := userRowCount(t, db, anna); count != 4 {
		t.Errorf("other user has %d rows, want 4", count)
	}
}

func TestRowsForMissingUsersAreRefused(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(`INSERT INTO folder_prefs (user_id, path) VALUES (999, 'docs')`); err == nil {
		t.Error("a row for a user that doesn't exist was stored")
	}
}

func TestMigrationRemovesRowsOfDeletedUsers(t *testing.T) {
	db := openTestDB(t)
	ann, anna := createUser(t, db, "ann"), createUser(t, db, "anna")
	addUserRows(t, db, ann)
	addUserRows(t, db, anna)
	// As before foreign keys were on: the user goes, their rows stay.
	// (One connection, so the pragma applies to everything below.)
	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id = ?`, ann); err != nil {
		t.Fatal(err)
	}
	if userRowCount(t, db, ann) != 4 {
		t.Fatal("setup: rows of the deleted user should still be there")
	}

	var cleanup migrations.Migration
	for _, migration := range migrations.All() {
		if migration.Name == "remove_rows_of_deleted_users" {
			cleanup = migration
		}
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanup.Up(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if count := userRowCount(t, db, ann); count != 0 {
		t.Errorf("%d rows of the deleted user left", count)
	}
	if count := userRowCount(t, db, anna); count != 4 {
		t.Errorf("other user has %d rows, want 4", count)
	}
}
