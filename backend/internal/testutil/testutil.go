// Package testutil has helpers shared by tests in several packages. Only test
// files import it.
package testutil

import (
	"database/sql"
	"path/filepath"
	"testing"

	"files/internal/database"

	"golang.org/x/crypto/bcrypt"
)

// OpenDB opens a fresh database in a temporary folder, with every migration
// applied (and the seeded admin user).
func OpenDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// Password is the password CreateUser gives every user.
const Password = "correct horse"

// CreateUser adds a user with Password and returns their id.
func CreateUser(t *testing.T, db *sql.DB, username string, isAdmin bool, rootDir string) int64 {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(Password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	result, err := db.Exec(`INSERT INTO users (username, password, root_dir, is_admin) VALUES (?, ?, ?, ?)`, username, string(hash), rootDir, isAdmin)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	return id
}
