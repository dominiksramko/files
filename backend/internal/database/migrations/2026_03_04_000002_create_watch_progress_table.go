package migrations

import "database/sql"

func init() {
	Register(Migration{
		Version: "2026_03_04_000002",
		Name:    "create_watch_progress_table",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS watch_progress (
					id         INTEGER PRIMARY KEY AUTOINCREMENT,
					user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					file_path  TEXT    NOT NULL,
					position   REAL    NOT NULL,
					duration   REAL    NOT NULL,
					updated_at TEXT    NOT NULL,
					UNIQUE(user_id, file_path)
				)
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE IF EXISTS watch_progress`)
			return err
		},
	})
}
