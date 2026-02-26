package migrations

import "database/sql"

func init() {
	Register(Migration{
		Version: "2026_02_26_000002",
		Name:    "create_folder_prefs_table",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS folder_prefs (
					id        INTEGER PRIMARY KEY AUTOINCREMENT,
					user_id   INTEGER NOT NULL,
					path      TEXT    NOT NULL DEFAULT '',
					view_mode TEXT    NOT NULL DEFAULT 'list',
					UNIQUE(user_id, path),
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				)
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE IF EXISTS folder_prefs`)
			return err
		},
	})
}
