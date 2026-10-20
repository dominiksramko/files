package migrations

import "database/sql"

func init() {
	Register(Migration{
		Version: "2026_10_20_091502",
		Name:    "create_sessions_table",
		Up: func(tx *sql.Tx) error {
			// One row per login. The browser holds a random token in a cookie;
			// only its SHA-256 is stored, so the table alone can't be used to log in.
			// Times are Unix seconds.
			if _, err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS sessions (
					token_hash   TEXT    PRIMARY KEY,
					user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					created_at   INTEGER NOT NULL,
					last_used_at INTEGER NOT NULL,
					expires_at   INTEGER NOT NULL
				)
			`); err != nil {
				return err
			}
			_, err := tx.Exec(`CREATE INDEX IF NOT EXISTS sessions_user_id ON sessions(user_id)`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE IF EXISTS sessions`)
			return err
		},
	})
}
