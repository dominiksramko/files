package migrations

import "database/sql"

func init() {
	Register(Migration{
		Version: "2026_02_26_000001",
		Name:    "create_users_table",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS users (
					id       INTEGER PRIMARY KEY AUTOINCREMENT,
					username TEXT    NOT NULL UNIQUE,
					password TEXT    NOT NULL,
					root_dir TEXT    NOT NULL DEFAULT '',
					is_admin INTEGER NOT NULL DEFAULT 0
				)
			`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE IF EXISTS users`)
			return err
		},
	})
}
