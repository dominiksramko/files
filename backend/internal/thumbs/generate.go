package thumbs

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
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

// fileContentHash returns a fast, stable hash for the file at path.
//
// It hashes: 8-byte little-endian file size, the first 64 KiB, and the last
// 64 KiB (for files ≤ 128 KiB the entire content is used). This means:
//   - Moving or renaming a file produces the same hash.
//   - Identical files at different paths share one thumbnail.
//   - Large video files are hashed cheaply (128 KiB read at most).
func fileContentHash(path string) (string, error) {
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
