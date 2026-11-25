package qrsvg

import (
	"fmt"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// RenderSVG renders payload into a theme-safe, high-contrast SVG QR code.
func RenderSVG(payload string, size int) (string, error) {
	if size <= 0 {
		size = 220
	}
	q, err := qrcode.New(payload, qrcode.Medium)
	if err != nil {
		return "", err
	}
	bm := q.Bitmap()
	n := len(bm)
	if n == 0 {
		return "", fmt.Errorf("empty qr bitmap")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" role="img" aria-label="Pairing QR Code">`, n, n, size, size)
	sb.WriteString(`<rect width="100%" height="100%" fill="#ffffff"/>`)
	sb.WriteString(`<path fill="#0f172a" d="`)
	for y, row := range bm {
		for x, dark := range row {
			if dark {
				fmt.Fprintf(&sb, "M%d,%dh1v1h-1z", x, y)
			}
		}
	}
	sb.WriteString(`"/></svg>`)
	return sb.String(), nil
}
