package media

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// directLaunch matches code starting a media tool itself rather than through
// this package: exec.Command / exec.CommandContext / media.Command naming one,
// also when the call is split over several lines.
var directLaunch = regexp.MustCompile(
	`(exec\.Command(Context)?|media\.Command)\(\s*(\w+,\s*|context\.\w+\(\),\s*)?"(ffmpeg|ffprobe|convert|identify|magick|nice)"`)

// TestMediaToolsOnlyRunHere fails when code outside this package launches
// ffmpeg, ffprobe or ImageMagick directly: those calls would miss the input
// guard and the ImageMagick policy. Use FFmpeg, FFprobe, MagickConvert or
// MagickIdentify instead.
func TestMediaToolsOnlyRunHere(t *testing.T) {
	backend, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	self, _ := filepath.Abs(".")
	var found []string
	filepath.WalkDir(backend, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() && (path == self || entry.Name() == "data" || strings.HasPrefix(entry.Name(), ".")) {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, match := range directLaunch.FindAllIndex(source, -1) {
			line := 1 + strings.Count(string(source[:match[0]]), "\n")
			rel, _ := filepath.Rel(backend, path)
			found = append(found, rel+":"+strconv.Itoa(line)+": "+strings.Join(strings.Fields(string(source[match[0]:match[1]])), " "))
		}
		return nil
	})
	if len(found) > 0 {
		t.Errorf("media tools launched outside internal/media (use media.FFmpeg / FFprobe / MagickConvert / MagickIdentify):\n%s", strings.Join(found, "\n"))
	}
}

func TestDirectLaunchPattern(t *testing.T) {
	for _, code := range []string{
		`exec.Command("ffmpeg", args...)`,
		"exec.Command(\n\t\t\"ffmpeg\", \"-v\", \"quiet\",",
		`exec.CommandContext(ctx, "ffprobe", "-i", path)`,
		`media.Command(ctx, "convert", args...)`,
		`exec.Command("nice", "-n", "10", "ffmpeg")`,
	} {
		if !directLaunch.MatchString(code) {
			t.Errorf("not caught: %s", code)
		}
	}
	for _, code := range []string{`exec.Command("7z", args...)`, `media.Command(ctx, "pdftoppm", "-r")`, `log.Printf("ffmpeg failed")`} {
		if directLaunch.MatchString(code) {
			t.Errorf("wrongly caught: %s", code)
		}
	}
}
