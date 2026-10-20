package thumbs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// hangingTools puts tools on PATH that never finish, like a real tool stuck
// on a malformed file.
func hangingTools(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, tool := range []string{"ffmpeg", "ffprobe", "convert", "identify", "rsvg-convert", "pdftoppm"} {
		os.WriteFile(filepath.Join(dir, tool), []byte("#!/bin/sh\nexec sleep 60\n"), 0o755)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestThumbnailToolsAreStoppedAtTheDeadline(t *testing.T) {
	hangingTools(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "input")
	os.WriteFile(src, []byte("x"), 0o644)
	generators := map[string]func(ctx context.Context) error{
		"video": func(ctx context.Context) error { return generateVideoThumb(ctx, src, filepath.Join(dir, "v.jpg"), 64) },
		"svg":   func(ctx context.Context) error { return generateSVGThumb(ctx, src, filepath.Join(dir, "s.jpg"), 64) },
		"pdf":   func(ctx context.Context) error { return generatePDFThumb(ctx, src, filepath.Join(dir, "p.jpg"), 64) },
		"magick": func(ctx context.Context) error {
			return generateMagickThumb(ctx, src, filepath.Join(dir, "m.jpg"), "image/heic", 64)
		},
		"ico": func(ctx context.Context) error {
			return generateMagickThumb(ctx, src, filepath.Join(dir, "i.jpg"), "image/x-icon", 64)
		},
	}
	for name, generate := range generators {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		start := time.Now()
		err := generate(ctx)
		cancel()
		if err == nil {
			t.Errorf("%s: a hung tool reported success", name)
		}
		if took := time.Since(start); took > 3*time.Second {
			t.Errorf("%s: took %v after a 300 ms deadline", name, took)
		}
	}
}
