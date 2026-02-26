package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"

	"files/internal/database/migrations"
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

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func runMigrations(db *sql.DB) error {
	// Ensure the tracking table exists before anything else.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS migrations (
			version    TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Load already-applied versions.
	rows, err := db.Query(`SELECT version FROM migrations`)
	if err != nil {
		return fmt.Errorf("query migrations: %w", err)
	}
	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Apply any pending migrations in version order.
	for _, m := range migrations.All() {
		if applied[m.Version] {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("migration %s (%s): %w", m.Version, m.Name, err)
		}
		log.Printf("db: applied migration %s_%s", m.Version, m.Name)
	}

	return seedAdmin(db)
}

func applyMigration(db *sql.DB, m migrations.Migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := m.Up(tx); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`INSERT INTO migrations (version, name) VALUES (?, ?)`,
		m.Version, m.Name,
	); err != nil {
		return err
	}

	return tx.Commit()
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
