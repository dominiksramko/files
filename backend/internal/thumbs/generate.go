package thumbs

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	stdraw "image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"

	"files/internal/media"
	"path/filepath"
	"strconv"
	"strings"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
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

// outputs names the files one source's thumbnails are written to.
type outputs struct {
	square  string // ThumbMaxDim square, shown in file lists
	preview string // the whole picture within PreviewMaxDim, shown while the preview loads
}

// generateThumb decodes the image at srcPath, turns it upright and writes its
// thumbnails.
func generateThumb(srcPath string, dst outputs) error {
	src, err := decodeImageFile(srcPath)
	if err != nil {
		return err
	}
	return writeThumbs(applyOrientation(src, jpegOrientation(srcPath)), dst)
}

// decodeImageFile decodes the image file at path.
func decodeImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return src, nil
}

// writeThumbs writes both thumbnails of src: the preview keeps the whole
// picture, the square is a top-centre crop with transparent edges trimmed.
// Both are cut from one scaled-down copy, so the full-size picture — the
// costly part — is scaled only once. Transparent areas show a checkerboard.
func writeThumbs(src image.Image, dst outputs) error {
	working := scaleForThumbs(src)
	preview := flattenOnCheckerboard(fitWithin(working, PreviewMaxDim))
	square := squareCrop(flattenOnCheckerboard(alphaCrop(working)), ThumbMaxDim)

	if err := writeJPEG(dst.preview, preview, previewQuality); err != nil {
		return err
	}
	if err := writeJPEG(dst.square, square, thumbQuality); err != nil {
		os.Remove(dst.preview)
		return err
	}
	return nil
}

// writeJPEG encodes img to a new file at path, leaving nothing behind if it fails.
func writeJPEG(path string, img image.Image, quality int) error {
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	err = jpeg.Encode(out, img, &jpeg.Options{Quality: quality})
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}

// scaleForThumbs returns src scaled down only as far as both thumbnails allow:
// to fit within PreviewMaxDim, but with the short side kept at ThumbMaxDim or
// more, so the square isn't enlarged (a panorama's preview is then made
// smaller again). Pictures already smaller are returned as they are.
func scaleForThumbs(src image.Image) image.Image {
	b := src.Bounds()
	longSide, shortSide := max(b.Dx(), b.Dy()), min(b.Dx(), b.Dy())
	if shortSide == 0 {
		return src
	}
	scale := max(float64(PreviewMaxDim)/float64(longSide), float64(ThumbMaxDim)/float64(shortSide))
	if scale >= 1 {
		return src
	}
	return scaleTo(src, scaledSide(b.Dx(), scale), scaledSide(b.Dy(), scale))
}

// fitWithin returns src scaled down so neither side exceeds maxSide.
func fitWithin(src image.Image, maxSide int) image.Image {
	b := src.Bounds()
	longSide := max(b.Dx(), b.Dy())
	if longSide <= maxSide {
		return src
	}
	scale := float64(maxSide) / float64(longSide)
	return scaleTo(src, scaledSide(b.Dx(), scale), scaledSide(b.Dy(), scale))
}

// scaledSide is side scaled and rounded, but at least one pixel.
func scaledSide(side int, scale float64) int {
	return max(1, int(math.Round(float64(side)*scale)))
}

func scaleTo(src image.Image, width, height int) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

// generateMagickThumb uses ImageMagick's convert to rasterise srcPath to a
// temporary PNG, then writes its thumbnails.
//
// Frame/layer selection strategy:
//   - ICO: contains multiple independent size variants; we pick the largest frame.
//   - PSD/XCF: layered formats where all layers compose the final image; we load
//     all layers and flatten them together.
//   - Everything else: single-frame formats, [0] is used defensively.
func generateMagickThumb(ctx context.Context, srcPath, mime string, dst outputs) error {
	tmp, err := os.CreateTemp("", "magick-thumb-*.png")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	rasterSize := strconv.Itoa(PreviewMaxDim)

	// Build the convert argument list based on format:
	//   ICO  — composite all frames with the largest as base canvas, so all
	//           layers contribute correctly (largest-first avoids the base being
	//           clipped to a smaller frame's canvas).
	//   PSD/XCF — all layers loaded and flattened to produce the merged image.
	//   Others  — single-frame; [0] is used defensively.
	var args []string
	switch mime {
	case "image/x-icon":
		args = icoConvertArgs(ctx, srcPath, rasterSize, tmp.Name())
	case "image/vnd.adobe.photoshop", "image/x-xcf":
		args = []string{"-background", "none", srcPath, "-flatten", "-resize", rasterSize + "x" + rasterSize + ">", tmp.Name()}
	default:
		args = []string{"-background", "none", srcPath + "[0]", "-flatten", "-resize", rasterSize + "x" + rasterSize + ">", tmp.Name()}
	}
	if out, err := media.MagickConvert(ctx, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("convert: %w: %s", err, strings.TrimSpace(string(out)))
	}

	src, err := decodeImageFile(tmp.Name())
	if err != nil {
		return err
	}
	return writeThumbs(src, dst)
}

// icoConvertArgs builds the ImageMagick convert argument list for an ICO file.
//
// Strategy depends on whether the ICO is a "variations" file (same image at
// multiple sizes — pick only the largest frame) or a "composite" file
// (distinct images per layer — composite all frames together):
//   - Variations: all frames share the same aspect ratio; use only the largest.
//   - Composite: frames have different aspect ratios; place the largest first
//     as the base canvas then append all others and flatten.
func icoConvertArgs(ctx context.Context, srcPath, rasterSize, tmpPath string) []string {
	largestIdx, frames := ICOFrameDimensions(ctx, srcPath)

	if ICOIsComposite(frames) {
		args := []string{"-background", "none", fmt.Sprintf("%s[%d]", srcPath, largestIdx)}
		for i := range frames {
			if i != largestIdx {
				args = append(args, fmt.Sprintf("%s[%d]", srcPath, i))
			}
		}
		return append(args, "-flatten", "-resize", rasterSize+"x"+rasterSize+">", tmpPath)
	}

	// Variations: single best frame is sufficient.
	return []string{
		"-background", "none",
		fmt.Sprintf("%s[%d]", srcPath, largestIdx),
		"-resize", rasterSize + "x" + rasterSize + ">",
		tmpPath,
	}
}

// ICOFrameDimensions returns the dimensions of every frame in the ICO file and
// the index of the largest frame by pixel area. Falls back to (0, nil) on error.
func ICOFrameDimensions(ctx context.Context, srcPath string) (largestIdx int, frames []image.Point) {
	out, err := media.MagickIdentify(ctx, "-format", "%w %h\n", srcPath).Output()
	if err != nil {
		return 0, nil
	}
	bestArea := 0
	for i, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		var w, h int
		if _, err := fmt.Sscan(line, &w, &h); err == nil {
			frames = append(frames, image.Point{X: w, Y: h})
			if w*h > bestArea {
				largestIdx = i
				bestArea = w * h
			}
		}
	}
	return largestIdx, frames
}

// ICOIsComposite returns true when the ICO frames have meaningfully different
// aspect ratios, indicating each layer is a distinct image. Returns false when
// all frames share the same aspect ratio (same image at different sizes).
func ICOIsComposite(frames []image.Point) bool {
	if len(frames) <= 1 {
		return false
	}
	refW, refH := reducedRatio(frames[0].X, frames[0].Y)
	for _, frame := range frames[1:] {
		rw, rh := reducedRatio(frame.X, frame.Y)
		if rw != refW || rh != refH {
			return true
		}
	}
	return false
}

// reducedRatio returns w and h divided by their GCD, giving a canonical aspect
// ratio representation (e.g. 256×256 and 32×32 both reduce to 1×1).
func reducedRatio(w, h int) (int, int) {
	if w == 0 || h == 0 {
		return w, h
	}
	g := gcd(w, h)
	return w / g, h / g
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// generatePDFThumb rasterises the first page of the PDF at srcPath via
// pdftoppm and writes its thumbnails.
func generatePDFThumb(ctx context.Context, srcPath string, dst outputs) error {
	dir, err := os.MkdirTemp("", "pdf-thumb-*")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	rasterSize := strconv.Itoa(PreviewMaxDim)
	// -r sets DPI; 150 is enough for a 640px preview of a typical A4 page.
	// -f 1 -l 1 renders only the first page. Output files are <prefix>-1.png.
	if out, err := media.Command(ctx,
		"pdftoppm", "-r", "150", "-f", "1", "-l", "1", "-png",
		"-scale-to", rasterSize,
		srcPath, filepath.Join(dir, "p"),
	).CombinedOutput(); err != nil {
		return fmt.Errorf("pdftoppm: %w: %s", err, strings.TrimSpace(string(out)))
	}

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("pdftoppm produced no output")
	}
	src, err := decodeImageFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		return err
	}
	return writeThumbs(src, dst)
}

// generateSVGThumb rasterises the SVG at srcPath via rsvg-convert and writes
// its thumbnails.
func generateSVGThumb(ctx context.Context, srcPath string, dst outputs) error {
	tmp, err := os.CreateTemp("", "svg-thumb-*.png")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	// Rasterise at the preview's size, 2× the square's, so scaling it down is high quality.
	rasterSize := strconv.Itoa(PreviewMaxDim)
	if out, err := media.Command(ctx, "rsvg-convert", "--width", rasterSize, srcPath, "-o", tmp.Name()).CombinedOutput(); err != nil {
		return fmt.Errorf("rsvg-convert: %w: %s", err, strings.TrimSpace(string(out)))
	}

	src, err := decodeImageFile(tmp.Name())
	if err != nil {
		return err
	}
	return writeThumbs(src, dst)
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

// generateVideoThumb takes a frame from the middle of the video at srcPath
// with FFmpeg and writes its thumbnails.
func generateVideoThumb(ctx context.Context, srcPath string, dst outputs) error {
	// Ask ffprobe for the duration so we can seek to the middle.
	seekSec := videoDuration(ctx, srcPath) / 2

	frame, err := os.CreateTemp("", "video-thumb-*.png")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	frame.Close()
	defer os.Remove(frame.Name())

	// Within 2× the preview's size (never enlarged): enough for writeThumbs
	// to scale down well, without a 4K frame to encode and decode.
	maxSide := 2 * PreviewMaxDim
	filter := fmt.Sprintf(
		"scale=w='min(iw,%d)':h='min(ih,%d)':force_original_aspect_ratio=decrease",
		maxSide, maxSide,
	)

	// -ss before -i uses fast keyframe seeking (fine for thumbnails).
	cmd := media.FFmpeg(ctx, srcPath, []string{"-ss", fmt.Sprintf("%.3f", seekSec)},
		"-vframes", "1",
		"-vf", filter,
		"-y", // overwrite the (empty) temp file
		frame.Name(),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(out)))
	}

	src, err := decodeImageFile(frame.Name())
	if err != nil {
		return err
	}
	return writeThumbs(src, dst)
}

// videoDuration returns the duration in seconds via ffprobe, or 0 on error.
func videoDuration(ctx context.Context, path string) float64 {
	out, err := media.FFprobeContext(ctx, path,
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
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

// alphaCrop returns src cropped to the bounding box of non-transparent pixels.
// Images with no alpha channel (YCbCr, CMYK, Gray) are returned unchanged.
// If all pixels are transparent or the content already fills the canvas, src
// is also returned unchanged.
func alphaCrop(src image.Image) image.Image {
	switch src.ColorModel() {
	case color.YCbCrModel, color.CMYKModel, color.GrayModel, color.Gray16Model:
		return src
	}

	b := src.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X-1, b.Min.Y-1

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := src.At(x, y).RGBA()
			if a > 0 {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}

	// No visible pixels at all.
	if maxX < minX {
		return src
	}
	// Content already fills the canvas.
	if minX == b.Min.X && minY == b.Min.Y && maxX == b.Max.X-1 && maxY == b.Max.Y-1 {
		return src
	}

	rect := image.Rect(minX, minY, maxX+1, maxY+1)
	type subImager interface {
		SubImage(image.Rectangle) image.Image
	}
	if si, ok := src.(subImager); ok {
		return si.SubImage(rect)
	}

	// Fallback for image types that don't support SubImage.
	cropped := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	stdraw.Draw(cropped, cropped.Bounds(), src, rect.Min, stdraw.Src)
	return cropped
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
