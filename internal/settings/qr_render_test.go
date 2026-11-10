package settings

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"vanillapay/internal/platform"
)

// Version 40 byte-mode capacity is 2331 bytes at M and 2953 bytes at L.
func TestUnit_RenderQRFallsBackToLowCorrection(t *testing.T) {
	prefix := "https://qr.alipay.com/"
	cases := map[string]string{
		"short ascii":    prefix + "fkx12345",
		"ascii beyond M": prefix + strings.Repeat("a", 2500-len(prefix)),
		"utf8 beyond M":  prefix + strings.Repeat("码", 800),
		"utf8 within M":  prefix + "收款-" + strings.Repeat("b", 64),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			data, err := renderQR(payload)
			if err != nil {
				t.Fatalf("renderQR(%d bytes): %v", len(payload), err)
			}
			assertCleanQR(t, data, payload)
		})
	}
	t.Run("beyond L is rejected", func(t *testing.T) {
		_, err := renderQR(prefix + strings.Repeat("a", 3000))
		var appErr *platform.Error
		if !errors.As(err, &appErr) || appErr.Code != "INVALID_QR" {
			t.Fatalf("renderQR beyond L = %v, want INVALID_QR", err)
		}
	})
}

// assertCleanQR checks the regenerated PNG is 512x512, keeps a four-module
// quiet zone on every side and decodes back to exactly payload.
func assertCleanQR(t *testing.T, data []byte, payload string) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Size() != (image.Point{X: 512, Y: 512}) {
		t.Fatalf("regenerated QR: err=%v size=%v", err, img.Bounds().Size())
	}
	black := func(x, y int) bool { r, _, _, _ := img.At(x, y).RGBA(); return r == 0 }
	minX, minY, maxX, maxY := 512, 512, -1, -1
	for y := range 512 {
		for x := range 512 {
			if black(x, y) {
				minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
			}
		}
	}
	run := 0
	for x := minX; x <= maxX && black(x, minY); x++ {
		run++
	}
	module := run / 7 // The top edge of the finder pattern is seven modules.
	if module < 1 || run%7 != 0 || minX < 4*module || minY < 4*module || 511-maxX < 4*module || 511-maxY < 4*module {
		t.Fatalf("quiet zone too small: module=%d bbox=(%d,%d)-(%d,%d)", module, minX, minY, maxX, maxY)
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := qrcode.NewQRCodeReader().Decode(bitmap, map[gozxing.DecodeHintType]any{gozxing.DecodeHintType_PURE_BARCODE: true})
	if err != nil || decoded.GetText() != payload {
		t.Fatalf("regenerated QR does not decode to the payload: err=%v", err)
	}
}
