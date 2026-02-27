package migrations

import "database/sql"

func init() {
	Register(Migration{
		Version: "2026_02_27_000001",
		Name:    "create_thumbnails_table",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS thumbnails (
					abs_path   TEXT    NOT NULL PRIMARY KEY,
					file_mtime INTEGER NOT NULL,
					thumb_file TEXT    NOT NULL,
					status     TEXT    NOT NULL DEFAULT 'pending',
					created_at TEXT    NOT NULL DEFAULT (datetime('now'))
				)
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE IF EXISTS thumbnails`)
			return err
		},
	})
}
