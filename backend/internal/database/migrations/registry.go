package migrations

import (
	"database/sql"
	"sort"
)

// Migration describes a single, ordered schema change.
type Migration struct {
	Version string // YYYY_MM_DD_HHMMSS — determines execution order
	Name    string // snake_case description, e.g. "create_users_table"
	Up      func(tx *sql.Tx) error
	Down    func(tx *sql.Tx) error
}

var registry []Migration

// Register adds a migration to the registry.
// Called automatically from each migration file's init().
func Register(m Migration) {
	registry = append(registry, m)
}

// All returns all registered migrations sorted by version ascending.
func All() []Migration {
	out := make([]Migration, len(registry))
	copy(out, registry)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Version < out[j].Version
	})
	return out
}
