package migrations

import "database/sql"

func init() {
	Register(Migration{
		Version: "2026_03_11_000001",
		Name:    "add_rename_presets",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS rename_presets (
					id         INTEGER PRIMARY KEY AUTOINCREMENT,
					user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					name       TEXT    NOT NULL,
					template   TEXT    NOT NULL,
					created_at TEXT    NOT NULL,
					UNIQUE(user_id, name)
				)
			`)
			if err != nil {
				return err
			}
			_, err = tx.Exec(`ALTER TABLE folder_prefs ADD COLUMN last_rename_template TEXT NOT NULL DEFAULT ''`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			if _, err := tx.Exec(`DROP TABLE IF EXISTS rename_presets`); err != nil {
				return err
			}
			_, err := tx.Exec(`ALTER TABLE folder_prefs DROP COLUMN last_rename_template`)
			return err
		},
	})
}
