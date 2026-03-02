package migrations

import "database/sql"

func init() {
	Register(Migration{
		Version: "2026_03_02_000001",
		Name:    "add_display_name_to_users",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT ''`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`ALTER TABLE users DROP COLUMN display_name`)
			return err
		},
	})
}
