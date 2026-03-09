package thumbs

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	stdraw "image/draw"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// FileContentHash returns a fast, stable hash for the file at path.
//
// It hashes: 8-byte little-endian file size, the first 64 KiB, and the last
// 64 KiB (for files ≤ 128 KiB the entire content is used). This means:
//   - Moving or renaming a file produces the same hash.
//   - Identical files at different paths share one thumbnail.
//   - Large video files are hashed cheaply (128 KiB read at most).
func FileContentHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat: %w", err)
	}

	h := sha256.New()

	var sizeBuf [8]byte
	binary.LittleEndian.PutUint64(sizeBuf[:], uint64(fi.Size()))
	h.Write(sizeBuf[:])

	const chunk = 64 << 10 // 64 KiB

	if fi.Size() <= chunk*2 {
		if _, err := io.Copy(h, f); err != nil {
			return "", fmt.Errorf("hash: %w", err)
		}
	} else {
		head := make([]byte, chunk)
		if _, err := io.ReadFull(f, head); err != nil {
			return "", fmt.Errorf("hash head: %w", err)
		}
		h.Write(head)

		if _, err := f.Seek(-chunk, io.SeekEnd); err != nil {
			return "", fmt.Errorf("seek tail: %w", err)
		}
		tail := make([]byte, chunk)
		if _, err := io.ReadFull(f, tail); err != nil {
			return "", fmt.Errorf("hash tail: %w", err)
		}
		h.Write(tail)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// generateThumb decodes the image at srcPath, crops a top-center square, scales
// it to size×size pixels, and writes a JPEG thumbnail to dstPath.
func generateThumb(srcPath, dstPath string, size int) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	thumb := squareCrop(src, size)

	out, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer out.Close()

	if err := jpeg.Encode(out, thumb, &jpeg.Options{Quality: 80}); err != nil {
		os.Remove(dstPath)
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}

// generateMagickThumb uses ImageMagick's convert to rasterise srcPath to a
// temporary PNG, then composites onto a checkerboard and encodes to JPEG.
//
// Frame/layer selection strategy:
//   - ICO: contains multiple independent size variants; we pick the largest frame.
//   - PSD/XCF: layered formats where all layers compose the final image; we load
//     all layers and flatten them together.
//   - Everything else: single-frame formats, [0] is used defensively.
func generateMagickThumb(srcPath, dstPath, mime string, size int) error {
	tmp, err := os.CreateTemp("", "magick-thumb-*.png")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	rasterSize := strconv.Itoa(size * 2)

	// Build the convert argument list based on format:
	//   ICO  — composite all frames with the largest as base canvas, so all
	//           layers contribute correctly (largest-first avoids the base being
	//           clipped to a smaller frame's canvas).
	//   PSD/XCF — all layers loaded and flattened to produce the merged image.
	//   Others  — single-frame; [0] is used defensively.
	var args []string
	switch mime {
	case "image/x-icon":
		args = icoConvertArgs(srcPath, rasterSize, tmp.Name())
	case "image/vnd.adobe.photoshop", "image/x-xcf":
		args = []string{"-background", "none", srcPath, "-flatten", "-resize", rasterSize + "x" + rasterSize + ">", tmp.Name()}
	default:
		args = []string{"-background", "none", srcPath + "[0]", "-flatten", "-resize", rasterSize + "x" + rasterSize + ">", tmp.Name()}
	}
	if out, err := exec.Command("convert", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("convert: %w: %s", err, strings.TrimSpace(string(out)))
	}

	f, err := os.Open(tmp.Name())
	if err != nil {
		return fmt.Errorf("open converted: %w", err)
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	composited := flattenOnCheckerboard(src)
	thumb := squareCrop(composited, size)

	out, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer out.Close()

	if err := jpeg.Encode(out, thumb, &jpeg.Options{Quality: 80}); err != nil {
		os.Remove(dstPath)
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}

// icoConvertArgs builds the ImageMagick convert argument list for an ICO file.
// It places the largest frame first (so it becomes the base canvas) then
// appends all remaining frames; -flatten composites them in order.
// This handles both the common case (multiple size variants of the same image)
// and the less common case (complementary layers that must be composited).
func icoConvertArgs(srcPath, rasterSize, tmpPath string) []string {
	largest, count := LargestICOFrame(srcPath)

	args := []string{"-background", "none", fmt.Sprintf("%s[%d]", srcPath, largest)}
	for i := 0; i < count; i++ {
		if i != largest {
			args = append(args, fmt.Sprintf("%s[%d]", srcPath, i))
		}
	}
	args = append(args, "-flatten", "-resize", rasterSize+"x"+rasterSize+">", tmpPath)
	return args
}

// LargestICOFrame returns the index of the frame with the greatest pixel area
// and the total frame count. Falls back to (0, 1) on any error.
func LargestICOFrame(srcPath string) (idx, count int) {
	out, err := exec.Command("identify", "-format", "%w %h\n", srcPath).Output()
	if err != nil {
		return 0, 1
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	bestArea := 0
	for i, line := range lines {
		var w, h int
		if _, err := fmt.Sscan(line, &w, &h); err == nil && w*h > bestArea {
			idx = i
			bestArea = w * h
		}
	}
	return idx, len(lines)
}

// generateSVGThumb rasterises the SVG at srcPath via rsvg-convert, composites
// the result onto a checkerboard background (so transparent areas are visible
// in the JPEG thumbnail), then crops and encodes to dstPath.
func generateSVGThumb(srcPath, dstPath string, size int) error {
	tmp, err := os.CreateTemp("", "svg-thumb-*.png")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	// Rasterise at 2× the target size so the subsequent downscale is high quality.
	rasterSize := strconv.Itoa(size * 2)
	if out, err := exec.Command("rsvg-convert", "--width", rasterSize, srcPath, "-o", tmp.Name()).CombinedOutput(); err != nil {
		return fmt.Errorf("rsvg-convert: %w: %s", err, strings.TrimSpace(string(out)))
	}

	f, err := os.Open(tmp.Name())
	if err != nil {
		return fmt.Errorf("open rasterized png: %w", err)
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode png: %w", err)
	}

	composited := flattenOnCheckerboard(src)
	thumb := squareCrop(composited, size)

	out, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer out.Close()

	if err := jpeg.Encode(out, thumb, &jpeg.Options{Quality: 80}); err != nil {
		os.Remove(dstPath)
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}

// flattenOnCheckerboard composites src over a white/light-grey checkerboard,
// making transparent areas visible in the opaque JPEG output.
func flattenOnCheckerboard(src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewNRGBA(b)

	const tile = 16
	light := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	dark := color.NRGBA{R: 204, G: 204, B: 204, A: 255}

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if ((x-b.Min.X)/tile+(y-b.Min.Y)/tile)%2 == 0 {
				dst.SetNRGBA(x, y, light)
			} else {
				dst.SetNRGBA(x, y, dark)
			}
		}
	}

	stdraw.Draw(dst, b, src, b.Min, stdraw.Over)
	return dst
}

// generateVideoThumb extracts a frame from the middle of the video at srcPath
// and writes a size×size JPEG thumbnail to dstPath using FFmpeg.
func generateVideoThumb(srcPath, dstPath string, size int) error {
	// Ask ffprobe for the duration so we can seek to the middle.
	seekSec := videoDuration(srcPath) / 2

	// Scale so the shortest side = size, then crop size×size from the top-centre.
	// (iw-ow)/2 centres horizontally; y=0 anchors to the top.
	filter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d:(iw-ow)/2:0",
		size, size, size, size,
	)

	// -ss before -i uses fast keyframe seeking (fine for thumbnails).
	args := []string{
		"-ss", fmt.Sprintf("%.3f", seekSec),
		"-i", srcPath,
		"-vframes", "1",
		"-vf", filter,
		"-q:v", "2", // JPEG quality (2 = high)
		"-y",        // overwrite output
		dstPath,
	}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// videoDuration returns the duration in seconds via ffprobe, or 0 on error.
func videoDuration(path string) float64 {
	out, err := exec.Command(
		"ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return 0
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// squareCrop takes a top-center square crop of src and scales it to size×size.
// The crop square has side length = min(width, height), positioned at the
// horizontal centre and the top of the image. This keeps subjects that are
// near the top (faces, horizons) in frame regardless of aspect ratio.
func squareCrop(src image.Image, size int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	// Largest square that fits, anchored top-centre.
	side := min(w, h)
	x0 := b.Min.X + (w-side)/2
	y0 := b.Min.Y
	srcRect := image.Rect(x0, y0, x0+side, y0+side)

	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	// Scale the crop rectangle directly to the destination — one GPU-friendly pass.
	draw.BiLinear.Scale(dst, dst.Bounds(), src, srcRect, draw.Over, nil)
	return dst
}
