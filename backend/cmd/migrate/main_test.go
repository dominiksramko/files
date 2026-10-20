package main

import (
	"go/format"
	"strings"
	"testing"
)

func TestBuildContent(t *testing.T) {
	content := buildContent("2026_10_20_120000", "add_email_to_users")

	formatted, err := format.Source([]byte(content))
	if err != nil {
		t.Fatalf("generated file doesn't parse: %v\n%s", err, content)
	}
	if string(formatted) != content {
		t.Errorf("generated file isn't gofmt-formatted:\n%s", content)
	}
	for _, want := range []string{`Version: "2026_10_20_120000"`, `Name:    "add_email_to_users"`} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %s", want)
		}
	}
	// An unwritten migration must fail, not be recorded as applied.
	if strings.Count(content, `return errors.New("migration not written yet")`) != 2 {
		t.Errorf("Up and Down should fail until written:\n%s", content)
	}
}
