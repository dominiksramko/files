package thumbs

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
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
	dst := outputs{square: filepath.Join(dir, "square.jpg"), preview: filepath.Join(dir, "preview.jpg")}
	generators := map[string]func(ctx context.Context) error{
		"video":  func(ctx context.Context) error { return generateVideoThumb(ctx, src, dst) },
		"svg":    func(ctx context.Context) error { return generateSVGThumb(ctx, src, dst) },
		"pdf":    func(ctx context.Context) error { return generatePDFThumb(ctx, src, dst) },
		"magick": func(ctx context.Context) error { return generateMagickThumb(ctx, src, "image/heic", dst) },
		"ico":    func(ctx context.Context) error { return generateMagickThumb(ctx, src, "image/x-icon", dst) },
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

func TestScaleForThumbs(t *testing.T) {
	for _, test := range []struct {
		name          string
		width, height int
		want          image.Point
	}{
		{"portrait photo", 1170, 2080, image.Pt(360, 640)},
		{"landscape photo", 4000, 3000, image.Pt(640, 480)},
		{"panorama keeps the square's size", 3000, 500, image.Pt(1920, 320)},
		{"already small", 200, 100, image.Pt(200, 100)},
		{"exactly the preview's size", 640, 640, image.Pt(640, 640)},
	} {
		scaled := scaleForThumbs(image.NewNRGBA(image.Rect(0, 0, test.width, test.height)))
		if got := scaled.Bounds().Size(); got != test.want {
			t.Errorf("%s: %v, want %v", test.name, got, test.want)
		}
	}
}

func TestFitWithin(t *testing.T) {
	if got := fitWithin(image.NewNRGBA(image.Rect(0, 0, 1920, 320)), 640).Bounds().Size(); got != image.Pt(640, 107) {
		t.Errorf("panorama: %v, want 640×107", got)
	}
	small := image.NewNRGBA(image.Rect(0, 0, 300, 200))
	if got := fitWithin(small, 640); got != image.Image(small) {
		t.Error("a picture that fits was scaled")
	}
}

// testOutputs names thumbnails in a fresh folder.
func testOutputs(t *testing.T) outputs {
	dir := t.TempDir()
	return outputs{square: filepath.Join(dir, "square.jpg"), preview: filepath.Join(dir, "preview.jpg")}
}

// decodeJPEG decodes the thumbnail at path.
func decodeJPEG(t *testing.T, path string) image.Image {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img, err := jpeg.Decode(file)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return img
}

// assertColorNear fails unless the pixel at x, y is within JPEG noise of want.
func assertColorNear(t *testing.T, what string, img image.Image, x, y int, want color.NRGBA) {
	t.Helper()
	red, green, blue, _ := img.At(x, y).RGBA()
	got := [3]int{int(red >> 8), int(green >> 8), int(blue >> 8)}
	for channel, value := range [3]int{int(want.R), int(want.G), int(want.B)} {
		if diff := got[channel] - value; diff < -40 || diff > 40 {
			t.Errorf("%s: colour %v, want about %v", what, got, want)
			return
		}
	}
}

func assertSize(t *testing.T, what string, img image.Image, want image.Point) {
	t.Helper()
	if got := img.Bounds().Size(); got != want {
		t.Errorf("%s: %v, want %v", what, got, want)
	}
}

func TestWriteThumbsKeepsTheWholePictureInThePreview(t *testing.T) {
	red, blue := color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255}
	src := image.NewNRGBA(image.Rect(0, 0, 1170, 2080))
	for y := range 2080 {
		for x := range 1170 {
			if y < 1040 {
				src.SetNRGBA(x, y, red)
			} else {
				src.SetNRGBA(x, y, blue)
			}
		}
	}
	dst := testOutputs(t)
	if err := writeThumbs(src, dst); err != nil {
		t.Fatal(err)
	}

	preview := decodeJPEG(t, dst.preview)
	assertSize(t, "preview", preview, image.Pt(360, 640))
	assertColorNear(t, "preview top", preview, 180, 10, red)
	assertColorNear(t, "preview bottom", preview, 180, 630, blue)

	square := decodeJPEG(t, dst.square)
	assertSize(t, "square", square, image.Pt(ThumbMaxDim, ThumbMaxDim))
	assertColorNear(t, "square, cut from the top", square, 160, 250, red)
}

func TestWriteThumbsShowsTransparencyOnACheckerboard(t *testing.T) {
	green := color.NRGBA{G: 255, A: 255}
	src := image.NewNRGBA(image.Rect(0, 0, 400, 800)) // transparent but for a square in the middle
	for y := 350; y < 450; y++ {
		for x := 150; x < 250; x++ {
			src.SetNRGBA(x, y, green)
		}
	}
	dst := testOutputs(t)
	if err := writeThumbs(src, dst); err != nil {
		t.Fatal(err)
	}

	preview := decodeJPEG(t, dst.preview)
	assertSize(t, "preview keeps the transparent edges", preview, image.Pt(320, 640))
	assertColorNear(t, "preview corner", preview, 2, 2, color.NRGBA{R: 255, G: 255, B: 255})
	assertColorNear(t, "preview middle", preview, 160, 320, green)

	square := decodeJPEG(t, dst.square)
	assertColorNear(t, "square, transparent edges trimmed", square, 20, 20, green)
}

func TestWriteThumbsLeavesNothingWhenItFails(t *testing.T) {
	dst := testOutputs(t)
	dst.square = filepath.Join(t.TempDir(), "missing folder", "square.jpg")
	if err := writeThumbs(image.NewNRGBA(image.Rect(0, 0, 100, 100)), dst); err == nil {
		t.Fatal("writing into a missing folder succeeded")
	}
	if _, err := os.Stat(dst.preview); !os.IsNotExist(err) {
		t.Errorf("preview left behind (stat error: %v)", err)
	}
}

func TestGenerateThumb(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "photo.png")
	file, _ := os.Create(source)
	png.Encode(file, image.NewNRGBA(image.Rect(0, 0, 1600, 900)))
	file.Close()

	dst := testOutputs(t)
	if err := generateThumb(source, dst); err != nil {
		t.Fatal(err)
	}
	assertSize(t, "preview", decodeJPEG(t, dst.preview), image.Pt(640, 360))
	assertSize(t, "square", decodeJPEG(t, dst.square), image.Pt(ThumbMaxDim, ThumbMaxDim))

	notAnImage := filepath.Join(dir, "broken.png")
	os.WriteFile(notAnImage, []byte("not an image"), 0o644)
	dst = testOutputs(t)
	if err := generateThumb(notAnImage, dst); err == nil {
		t.Error("a broken image made thumbnails")
	}
	for _, path := range []string{dst.square, dst.preview} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s written for a broken image", path)
		}
	}
}

func TestGenerateVideoThumbKeepsTheVideosShape(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("needs ffmpeg and ffprobe")
		}
	}
	video := filepath.Join(t.TempDir(), "wide.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=1880x800:rate=10", video).CombinedOutput(); err != nil {
		t.Fatalf("make video: %v %s", err, out)
	}

	dst := testOutputs(t)
	if err := generateVideoThumb(context.Background(), video, dst); err != nil {
		t.Fatal(err)
	}
	assertSize(t, "preview", decodeJPEG(t, dst.preview), image.Pt(640, 272))
	assertSize(t, "square", decodeJPEG(t, dst.square), image.Pt(ThumbMaxDim, ThumbMaxDim))
}
