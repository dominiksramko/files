package migrations

import "database/sql"

func init() {
	Register(Migration{
		Version: "2026_02_27_000002",
		Name:    "content_hash_thumbnails",
		Up: func(tx *sql.Tx) error {
			// Replace the abs_path-keyed table with a content-hash-keyed one.
			// Existing records are dropped; thumbnails will be regenerated on
			// the next folder visit.
			if _, err := tx.Exec(`DROP TABLE IF EXISTS thumbnails`); err != nil {
				return err
			}
			_, err := tx.Exec(`
				CREATE TABLE thumbnails (
					content_hash TEXT NOT NULL PRIMARY KEY,
					thumb_file   TEXT NOT NULL,
					status       TEXT NOT NULL DEFAULT 'pending',
					created_at   TEXT NOT NULL DEFAULT (datetime('now'))
				)
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			if _, err := tx.Exec(`DROP TABLE IF EXISTS thumbnails`); err != nil {
				return err
			}
			_, err := tx.Exec(`
				CREATE TABLE thumbnails (
					abs_path   TEXT    NOT NULL PRIMARY KEY,
					file_mtime INTEGER NOT NULL,
					thumb_file TEXT    NOT NULL,
					status     TEXT    NOT NULL DEFAULT 'pending',
					created_at TEXT    NOT NULL DEFAULT (datetime('now'))
				)
			`)
			return err
		},
	})
}
