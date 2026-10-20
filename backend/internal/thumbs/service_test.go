package thumbs

import (
	"errors"
	"strings"
	"testing"
)

func TestRunRecovered(t *testing.T) {
	if err := runRecovered(func() error { return nil }); err != nil {
		t.Errorf("success: err = %v", err)
	}

	failure := errors.New("not an image")
	if err := runRecovered(func() error { return failure }); err != failure {
		t.Errorf("error: err = %v, want %v", err, failure)
	}

	err := runRecovered(func() error {
		var pixels []byte
		_ = pixels[10] // a decoder indexing past the data it got
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "index out of range") {
		t.Errorf("panic: err = %v, want the panic as an error", err)
	}
}
