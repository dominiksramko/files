package migrations

import "database/sql"

func init() {
	Register(Migration{
		Version: "2026_10_20_135021",
		Name:    "remove_rows_of_deleted_users",
		Up: func(tx *sql.Tx) error {
			// Foreign keys were off until now, so ON DELETE CASCADE never ran:
			// deleted users left their rows behind. Remove them before the
			// keys are enforced.
			for _, table := range []string{"folder_prefs", "watch_progress", "rename_presets", "sessions"} {
				if _, err := tx.Exec(`DELETE FROM ` + table + ` WHERE user_id NOT IN (SELECT id FROM users)`); err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(tx *sql.Tx) error {
			// The removed rows belonged to users who no longer exist.
			return nil
		},
	})
}
