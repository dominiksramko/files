package migrations

import "database/sql"

func init() {
	Register(Migration{
		Version: "2026_03_04_000001",
		Name:    "add_subtitle_prefs_to_users",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`ALTER TABLE users ADD COLUMN subtitle_prefs TEXT NOT NULL DEFAULT '{}'`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`ALTER TABLE users DROP COLUMN subtitle_prefs`)
			return err
		},
	})
}
