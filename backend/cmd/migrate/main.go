// migrate is a dev tool that scaffolds a new database migration file.
//
// Usage (run from the backend/ directory):
//
//	go run ./cmd/migrate <migration_name>
//
// Example:
//
//	go run ./cmd/migrate add_email_to_users
//
// Creates:
//
//	internal/database/migrations/2026_02_26_152341_add_email_to_users.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/migrate <migration_name>")
		fmt.Fprintln(os.Stderr, "example: go run ./cmd/migrate add_email_to_users")
		os.Exit(1)
	}

	// Normalise the name: lowercase, spaces/hyphens → underscores.
	name := strings.ToLower(strings.Join(os.Args[1:], "_"))
	name = strings.NewReplacer(" ", "_", "-", "_").Replace(name)

	version := time.Now().Format("2006_01_02_150405")
	filename := version + "_" + name + ".go"
	outDir := filepath.Join("internal", "database", "migrations")
	outPath := filepath.Join(outDir, filename)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if _, err := fmt.Fprint(f, buildContent(version, name)); err != nil {
		fmt.Fprintf(os.Stderr, "error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("created: %s\n", outPath)
}

// buildContent returns the source for a new migration file. Until its Up and
// Down are written they fail rather than do nothing: the dev server reloads,
// and migrates, as soon as the file is saved, and an empty migration would be
// recorded as applied and never run again.
func buildContent(version, name string) string {
	return fmt.Sprintf(`package migrations

import (
	"database/sql"
	"errors"
)

func init() {
	Register(Migration{
		Version: %q,
		Name:    %q,
		Up: func(tx *sql.Tx) error {
			return errors.New("migration not written yet")
		},
		Down: func(tx *sql.Tx) error {
			return errors.New("migration not written yet")
		},
	})
}
`, version, name)
}
