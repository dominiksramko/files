package thumbs

import (
	"image"
	"io"
	"os"
)

// jpegOrientation reads the EXIF orientation tag (0x0112) from a JPEG file.
// Returns 1 (no rotation needed) if the tag is absent or unreadable.
func jpegOrientation(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 1
	}
	defer f.Close()

	var soi [2]byte
	if _, err := io.ReadFull(f, soi[:]); err != nil || soi[0] != 0xFF || soi[1] != 0xD8 {
		return 1
	}

	buf := make([]byte, 2)
	for {
		if _, err := io.ReadFull(f, buf); err != nil {
			return 1
		}
		if buf[0] != 0xFF {
			return 1
		}
		marker := buf[1]

		if _, err := io.ReadFull(f, buf); err != nil {
			return 1
		}
		segLen := int(buf[0])<<8 | int(buf[1]) - 2
		if segLen < 0 {
			return 1
		}

		if marker == 0xE1 { // APP1 — candidate for EXIF
			seg := make([]byte, segLen)
			if _, err := io.ReadFull(f, seg); err != nil {
				return 1
			}
			if o := parseEXIFOrientation(seg); o != 0 {
				return o
			}
			continue
		}

		if marker == 0xDA { // SOS — compressed data starts, no more markers
			return 1
		}

		if _, err := io.CopyN(io.Discard, f, int64(segLen)); err != nil {
			return 1
		}
	}
}

// parseEXIFOrientation extracts the orientation value from a raw APP1 segment.
// Returns 0 if this APP1 doesn't contain EXIF data or the tag is absent.
func parseEXIFOrientation(app1 []byte) int {
	if len(app1) < 6 || string(app1[:4]) != "Exif" || app1[4] != 0 || app1[5] != 0 {
		return 0
	}
	data := app1[6:]
	if len(data) < 8 {
		return 0
	}

	var le bool
	switch string(data[:2]) {
	case "II":
		le = true
	case "MM":
		le = false
	default:
		return 0
	}

	u16 := func(b []byte) uint16 {
		if le {
			return uint16(b[0]) | uint16(b[1])<<8
		}
		return uint16(b[0])<<8 | uint16(b[1])
	}
	u32 := func(b []byte) uint32 {
		if le {
			return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
		}
		return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	}

	ifd0 := u32(data[4:8])
	if int(ifd0)+2 > len(data) {
		return 0
	}
	n := int(u16(data[ifd0 : ifd0+2]))
	for i := 0; i < n; i++ {
		off := int(ifd0) + 2 + i*12
		if off+12 > len(data) {
			break
		}
		if u16(data[off:off+2]) == 0x0112 { // Orientation tag
			return int(u16(data[off+8 : off+10]))
		}
	}
	return 0
}

// applyOrientation rotates/flips img to match the EXIF orientation value.
// Orientation 1 (or anything out of range) returns img unchanged.
//
//	1 = normal       2 = flip H       3 = rotate 180
//	4 = flip V       5 = transpose    6 = rotate 90 CW
//	7 = transverse   8 = rotate 270 CW
func applyOrientation(img image.Image, o int) image.Image {
	switch o {
	case 2:
		return flipH(img)
	case 3:
		return rotateImg(img, 180)
	case 4:
		return flipV(img)
	case 5:
		return flipH(rotateImg(img, 270))
	case 6:
		return rotateImg(img, 90)
	case 7:
		return flipH(rotateImg(img, 90))
	case 8:
		return rotateImg(img, 270)
	}
	return img
}

func rotateImg(img image.Image, deg int) image.Image {
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	var dstW, dstH int
	if deg == 90 || deg == 270 {
		dstW, dstH = srcH, srcW
	} else {
		dstW, dstH = srcW, srcH
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			var sx, sy int
			switch deg {
			case 90:
				sx = b.Min.X + y
				sy = b.Min.Y + srcH - 1 - x
			case 180:
				sx = b.Min.X + srcW - 1 - x
				sy = b.Min.Y + srcH - 1 - y
			case 270:
				sx = b.Min.X + srcW - 1 - y
				sy = b.Min.Y + x
			}
			dst.Set(x, y, img.At(sx, sy))
		}
	}
	return dst
}

func flipH(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, y, img.At(b.Min.X+w-1-x, b.Min.Y+y))
		}
	}
	return dst
}

func flipV(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, y, img.At(b.Min.X+x, b.Min.Y+h-1-y))
		}
	}
	return dst
}
