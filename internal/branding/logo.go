package branding

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

// DefaultICOSizes lists the multi-DPI Windows icon dimensions embedded into cloud-manager.ico.
var DefaultICOSizes = []int{16, 20, 24, 32, 48, 256}

// RenderNRGBA renders the anti-aliased Cloud Manager badge logo at size x size pixels.
func RenderNRGBA(size int) *image.NRGBA {
	if size < 8 {
		size = 8
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const samples = 4
	invSamples := 1.0 / float64(samples*samples)

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var rSum, gSum, bSum, aSum float64
			for sy := 0; sy < samples; sy++ {
				ny := (float64(y) + (float64(sy)+0.5)/float64(samples)) / float64(size)
				for sx := 0; sx < samples; sx++ {
					nx := (float64(x) + (float64(sx)+0.5)/float64(samples)) / float64(size)
					sr, sg, sb, sa := sampleLogo(nx, ny)
					rSum += sr * sa
					gSum += sg * sa
					bSum += sb * sa
					aSum += sa
				}
			}
			aAvg := aSum * invSamples
			if aAvg <= 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: clampByte((rSum * invSamples) / aAvg),
				G: clampByte((gSum * invSamples) / aAvg),
				B: clampByte((bSum * invSamples) / aAvg),
				A: clampByte(aAvg * 255.0),
			})
		}
	}
	return img
}

// PNG encodes the Cloud Manager badge logo as a square transparent PNG.
func PNG(size int) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, RenderNRGBA(size)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ICO packages PNG-compressed Cloud Manager logo sizes into a valid Windows .ico binary.
func ICO(sizes ...int) ([]byte, error) {
	if len(sizes) == 0 {
		sizes = DefaultICOSizes
	}
	pngs := make([][]byte, 0, len(sizes))
	for _, s := range sizes {
		if s < 16 || s > 256 {
			return nil, fmt.Errorf("invalid ICO icon size %d (must be 16..256)", s)
		}
		p, err := PNG(s)
		if err != nil {
			return nil, err
		}
		pngs = append(pngs, p)
	}

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // type: icon
	_ = binary.Write(&buf, binary.LittleEndian, uint16(len(sizes)))

	offset := uint32(6 + 16*len(sizes))
	for i, s := range sizes {
		dim := byte(s)
		if s == 256 {
			dim = 0
		}
		buf.WriteByte(dim)
		buf.WriteByte(dim)
		buf.WriteByte(0)                                       // palette count
		buf.WriteByte(0)                                       // reserved
		_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // color planes
		_ = binary.Write(&buf, binary.LittleEndian, uint16(32))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(len(pngs[i])))
		_ = binary.Write(&buf, binary.LittleEndian, offset)
		offset += uint32(len(pngs[i]))
	}
	for _, p := range pngs {
		buf.Write(p)
	}
	return buf.Bytes(), nil
}

func sampleLogo(x, y float64) (r, g, b, a float64) {
	dBox := sdRoundRect(x, y, 0.5, 0.5, 0.45, 0.45, 0.20)
	if dBox > 0 {
		return 0, 0, 0, 0
	}

	// Diagonal Material 3 royal cobalt (#1D4ED8) to sky cyan (#0EA5E9) badge fill.
	t := clamp01(0.35*x + 0.65*(1.0-y))
	r = lerp(29, 14, t)
	g = lerp(78, 165, t)
	b = lerp(216, 233, t)

	// Subtle crisp rim glow so the tray icon stays distinct on dark & light taskbars.
	if dBox > -0.032 {
		rim := clamp01((dBox+0.032)/0.032) * 0.28
		r = lerp(r, 224, rim)
		g = lerp(g, 242, rim)
		b = lerp(b, 254, rim)
	}

	// Crisp white cloud silhouette centered in [0.21..0.79] x [0.25..0.71].
	dCloud := min4(
		sdCapsule(x, y, 0.32, 0.60, 0.68, 0.60, 0.11),
		sdCircle(x, y, 0.34, 0.53, 0.13),
		sdCircle(x, y, 0.48, 0.43, 0.18),
		sdCircle(x, y, 0.64, 0.51, 0.135),
	)
	if dCloud <= 0 {
		r, g, b = 255, 255, 255
	}

	// Dual sync arrows inside cloud: left upward chevron (#1D4ED8), right downward (#0284C7).
	dUp := min3(
		sdCapsule(x, y, 0.42, 0.63, 0.42, 0.49, 0.025),
		sdCapsule(x, y, 0.42, 0.49, 0.365, 0.545, 0.024),
		sdCapsule(x, y, 0.42, 0.49, 0.475, 0.545, 0.024),
	)
	if dUp <= 0 && dCloud <= -0.02 {
		return 29, 78, 216, 1
	}

	dDown := min3(
		sdCapsule(x, y, 0.58, 0.47, 0.58, 0.61, 0.025),
		sdCapsule(x, y, 0.58, 0.61, 0.525, 0.555, 0.024),
		sdCapsule(x, y, 0.58, 0.61, 0.635, 0.555, 0.024),
	)
	if dDown <= 0 && dCloud <= -0.02 {
		return 2, 132, 199, 1
	}

	return r, g, b, 1
}

func sdRoundRect(px, py, cx, cy, bx, by, r float64) float64 {
	qx := math.Abs(px-cx) - (bx - r)
	qy := math.Abs(py-cy) - (by - r)
	ox := math.Max(qx, 0)
	oy := math.Max(qy, 0)
	return math.Hypot(ox, oy) + math.Min(math.Max(qx, qy), 0) - r
}

func sdCircle(px, py, cx, cy, r float64) float64 {
	return math.Hypot(px-cx, py-cy) - r
}

func sdCapsule(px, py, ax, ay, bx, by, r float64) float64 {
	pax, pay := px-ax, py-ay
	bax, bay := bx-ax, by-ay
	denom := bax*bax + bay*bay
	var h float64
	if denom > 0 {
		h = clamp01((pax*bax + pay*bay) / denom)
	}
	return math.Hypot(pax-bax*h, pay-bay*h) - r
}

func min3(a, b, c float64) float64 {
	return math.Min(a, math.Min(b, c))
}

func min4(a, b, c, d float64) float64 {
	return math.Min(math.Min(a, b), math.Min(c, d))
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampByte(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(math.Round(v))
}
