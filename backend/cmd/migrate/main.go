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

// buildContent returns the source for a new migration file.
// Backtick raw strings can't appear inside raw string literals in Go, so we
// assemble the template using a named constant.
func buildContent(version, name string) string {
	const tick = "`"
	return "package migrations\n\n" +
		"import \"database/sql\"\n\n" +
		"func init() {\n" +
		"\tRegister(Migration{\n" +
		"\t\tVersion: " + fmt.Sprintf("%q", version) + ",\n" +
		"\t\tName:    " + fmt.Sprintf("%q", name) + ",\n" +
		"\t\tUp: func(tx *sql.Tx) error {\n" +
		"\t\t\t_, err := tx.Exec(" + tick + tick + ")\n" +
		"\t\t\treturn err\n" +
		"\t\t},\n" +
		"\t\tDown: func(tx *sql.Tx) error {\n" +
		"\t\t\t_, err := tx.Exec(" + tick + tick + ")\n" +
		"\t\t\treturn err\n" +
		"\t\t},\n" +
		"\t})\n" +
		"}\n"
}
