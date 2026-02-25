package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"
)

func Open(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite is single-writer

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT    NOT NULL UNIQUE,
			password TEXT    NOT NULL,
			root_dir TEXT    NOT NULL DEFAULT '',
			is_admin INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS folder_prefs (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id   INTEGER NOT NULL,
			path      TEXT    NOT NULL DEFAULT '',
			view_mode TEXT    NOT NULL DEFAULT 'list',
			UNIQUE(user_id, path),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);
	`)
	if err != nil {
		return err
	}

	return seedAdmin(db)
}

func seedAdmin(db *sql.DB) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = db.Exec(
		`INSERT INTO users (username, password, root_dir, is_admin) VALUES (?, ?, ?, 1)`,
		"admin", string(hash), "",
	)
	return err
}
