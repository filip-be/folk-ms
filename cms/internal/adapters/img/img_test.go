package img

import (
	"bytes"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func pngOf(w, h int) []byte {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			m.Set(x, y, color.NRGBA{uint8(x), uint8(y), 100, 128})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, m)
	return b.Bytes()
}

func TestCover(t *testing.T) {
	out, err := Processor{}.Process(pngOf(1200, 500), ports.ImageCover)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]ports.ImageRendition{}
	for _, r := range out {
		names[r.Name] = r
		if _, err := jpeg.Decode(bytes.NewReader(r.Data)); err != nil {
			t.Fatalf("%s is not JPEG", r.Name)
		}
	}
	if _, ok := names["1600"]; ok {
		t.Fatal("must not upscale to 1600")
	}
	if r := names["960"]; r.Width != 960 || r.Height != 400 {
		t.Fatalf("960: %dx%d", r.Width, r.Height)
	}
	ig := names["ig"]
	ratio := float64(ig.Height) / float64(ig.Width)
	if ratio < 1/1.91-0.01 {
		t.Fatalf("ig crop too wide: %dx%d", ig.Width, ig.Height)
	}
}

func TestSocialCrops(t *testing.T) {
	out, err := Processor{}.Process(pngOf(800, 1200), ports.ImageSocial)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range out {
		if r.Name == "ig" && (r.Width != 800 || r.Height != 1000) {
			t.Fatalf("4:5 crop: %dx%d", r.Width, r.Height)
		}
	}
	out, _ = Processor{}.Process(pngOf(900, 850), ports.ImageSocial)
	for _, r := range out {
		if r.Name == "ig" && r.Width != r.Height {
			t.Fatalf("1:1 crop: %dx%d", r.Width, r.Height)
		}
	}
}

func TestRejections(t *testing.T) {
	heic := append([]byte{0, 0, 0, 24}, []byte("ftypheic0000")...)
	cases := map[string]struct {
		data []byte
		kind ports.ImageKind
		want error
	}{
		"heic":      {heic, ports.ImageCover, ErrHEIC},
		"garbage":   {[]byte("not an image"), ports.ImageCover, ErrUnsupported},
		"small":     {pngOf(100, 50), ports.ImageCover, ErrTooSmall},
		"portrait":  {pngOf(500, 900), ports.ImageCover, ErrNotLandscape},
		"too large": {make([]byte, MaxBytes+1), ports.ImageCover, ErrTooLarge},
	}
	for name, c := range cases {
		if _, err := (Processor{}).Process(c.data, c.kind); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v want %v", name, err, c.want)
		}
	}
}

func TestDecompressionBomb(t *testing.T) {
	// A PNG header claiming 20000x20000 pixels: rejected from the header,
	// before any pixel is decoded.
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewGray(image.Rect(0, 0, 1, 1)))
	data := b.Bytes()
	// IHDR width/height live at bytes 16..24.
	put := func(off int, v uint32) {
		data[off], data[off+1], data[off+2], data[off+3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
	}
	put(16, 20000)
	put(20, 20000)
	put(29, crc32.ChecksumIEEE(data[12:29])) // IHDR CRC
	if _, err := (Processor{}).Process(data, ports.ImageCover); !errors.Is(err, ErrTooManyPx) {
		t.Fatalf("got %v", err)
	}
}
