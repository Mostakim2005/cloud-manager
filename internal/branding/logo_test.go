package branding

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestRenderNRGBAAndPNG(t *testing.T) {
	for _, sz := range []int{16, 32, 48} {
		img := RenderNRGBA(sz)
		if img.Bounds().Dx() != sz || img.Bounds().Dy() != sz {
			t.Fatalf("expected %dx%d bounds, got %v", sz, sz, img.Bounds())
		}
		if corner := img.NRGBAAt(0, 0); corner.A != 0 {
			t.Fatalf("expected transparent outer corner at size %d, got alpha=%d", sz, corner.A)
		}
		center := img.NRGBAAt(sz/2, sz/2)
		if center.A < 200 {
			t.Fatalf("expected opaque cloud center at size %d, got %+v", sz, center)
		}

		raw, err := PNG(sz)
		if err != nil {
			t.Fatalf("PNG(%d) failed: %v", sz, err)
		}
		decoded, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("png.Decode(%d) failed: %v", sz, err)
		}
		if decoded.Bounds().Dx() != sz {
			t.Fatalf("decoded PNG size mismatch: got %d want %d", decoded.Bounds().Dx(), sz)
		}
	}
}

func TestICOPackaging(t *testing.T) {
	raw, err := ICO()
	if err != nil {
		t.Fatalf("ICO() failed: %v", err)
	}
	if len(raw) < 6+16*len(DefaultICOSizes) {
		t.Fatalf("ICO payload too small: %d bytes", len(raw))
	}
	if typ := binary.LittleEndian.Uint16(raw[2:4]); typ != 1 {
		t.Fatalf("expected ICO type=1, got %d", typ)
	}
	if count := binary.LittleEndian.Uint16(raw[4:6]); int(count) != len(DefaultICOSizes) {
		t.Fatalf("expected %d ICO entries, got %d", len(DefaultICOSizes), count)
	}
	lastEntry := 6 + 16*(len(DefaultICOSizes)-1)
	if raw[lastEntry] != 0 || raw[lastEntry+1] != 0 {
		t.Fatalf("expected 256x256 ICO entry width/height encoded as 0,0, got %d,%d", raw[lastEntry], raw[lastEntry+1])
	}
}
