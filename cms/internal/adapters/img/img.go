// Package img decodes uploaded images (JPEG, PNG, WebP), guards against
// decompression bombs, and produces resized JPEG renditions.
package img

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // register PNG
	"strconv"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // register WebP

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// Limits.
const (
	MaxBytes  = 15 << 20
	MaxPixels = 50_000_000
	MinWidth  = 400
	Quality   = 82
)

// Error is a problem with an upload, shown to Moderators in Polish or English.
type Error struct{ PL, EN string }

func (e *Error) Error() string { return e.EN }

// Localized returns the Polish and English message.
func (e *Error) Localized() (pl, en string) { return e.PL, e.EN }

// Errors shown to Moderators.
var (
	ErrTooLarge     = &Error{"plik jest za duży (maks. 15 MB)", "the file is too large (max. 15 MB)"}
	ErrTooManyPx    = &Error{"obraz ma za dużo pikseli (maks. 50 Mpx)", "the image has too many pixels (max. 50 Mpx)"}
	ErrHEIC         = &Error{"format HEIC nie jest obsługiwany: zapisz zdjęcie jako JPEG lub PNG", "HEIC is not supported: save the photo as JPEG or PNG"}
	ErrUnsupported  = &Error{"nieobsługiwany format obrazu: użyj JPEG, PNG lub WebP", "unsupported image format: use JPEG, PNG or WebP"}
	ErrTooSmall     = &Error{"obraz jest za mały (min. 400 px szerokości)", "the image is too small (min. 400 px wide)"}
	ErrNotLandscape = &Error{"zdjęcie główne musi być poziome", "the cover image must be landscape"}
)

// Widths of the Cover Image variants shown on Sites.
var CoverWidths = []int{480, 960, 1600}

// InstagramWidth is the width of the Instagram rendition.
const InstagramWidth = 1080

// Processor implements ports.ImageProcessor.
type Processor struct{}

// isHEIC detects the ISO BMFF "ftyp" box of HEIC/HEIF files.
func isHEIC(b []byte) bool {
	if len(b) < 12 || string(b[4:8]) != "ftyp" {
		return false
	}
	switch string(b[8:12]) {
	case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1", "avif":
		return true
	}
	return false
}

// Process implements ports.ImageProcessor. Cover: variants at CoverWidths
// (never upscaled) plus "ig", a crop within Instagram's 4:5..1.91:1 range.
// Social: "ig" cropped to 1:1 or 4:5, plus a 480 preview.
func (Processor) Process(data []byte, kind ports.ImageKind) ([]ports.ImageRendition, error) {
	if len(data) > MaxBytes {
		return nil, ErrTooLarge
	}
	if isHEIC(data) {
		return nil, ErrHEIC
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrUnsupported
	}
	switch format {
	case "jpeg", "png", "webp":
	default:
		return nil, ErrUnsupported
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, ErrTooManyPx
	}
	if cfg.Width < MinWidth {
		return nil, ErrTooSmall
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrUnsupported
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	var out []ports.ImageRendition
	switch kind {
	case ports.ImageCover:
		if w < h {
			return nil, ErrNotLandscape
		}
		for _, tw := range CoverWidths {
			if tw > w && tw != CoverWidths[0] {
				continue
			}
			r, err := render(src, b, min(tw, w), strconv.Itoa(tw))
			if err != nil {
				return nil, err
			}
			out = append(out, r)
		}
		crop := cropToRatio(b, 1.0/1.91, 5.0/4.0)
		r, err := render(src, crop, min(InstagramWidth, crop.Dx()), "ig")
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	case ports.ImageSocial:
		// Portrait-ish images become 4:5, the rest 1:1.
		target := 1.0
		if float64(h)/float64(w) >= 1.125 {
			target = 1.25
		}
		crop := cropToRatio(b, target, target)
		ig, err := render(src, crop, min(InstagramWidth, crop.Dx()), "ig")
		if err != nil {
			return nil, err
		}
		prev, err := render(src, crop, min(480, crop.Dx()), "480")
		if err != nil {
			return nil, err
		}
		out = append(out, prev, ig)
	default:
		return nil, fmt.Errorf("img: unknown kind %q", kind)
	}
	return out, nil
}

// cropToRatio center-crops b so that height/width lies in [minR, maxR].
func cropToRatio(b image.Rectangle, minR, maxR float64) image.Rectangle {
	w, h := b.Dx(), b.Dy()
	r := float64(h) / float64(w)
	switch {
	case r < minR: // too wide: trim the sides
		nw := int(float64(h) / minR)
		x := b.Min.X + (w-nw)/2
		return image.Rect(x, b.Min.Y, x+nw, b.Max.Y)
	case r > maxR: // too tall: trim top and bottom
		nh := int(float64(w) * maxR)
		y := b.Min.Y + (h-nh)/2
		return image.Rect(b.Min.X, y, b.Max.X, y+nh)
	}
	return b
}

func render(src image.Image, crop image.Rectangle, width int, name string) (ports.ImageRendition, error) {
	height := int(float64(crop.Dy()) * float64(width) / float64(crop.Dx()))
	if height < 1 {
		height = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	// White background for transparent PNG/WebP (JPEG has no alpha).
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: Quality}); err != nil {
		return ports.ImageRendition{}, err
	}
	return ports.ImageRendition{Name: name, Width: width, Height: height, Data: buf.Bytes()}, nil
}
