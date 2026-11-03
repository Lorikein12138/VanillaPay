package settings

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestUnit_ReadCleanImageReencodesPNGAndRejectsSVG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	cleaned, format, decoded, err := readCleanImage(bytes.NewReader(encoded.Bytes()))
	if err != nil || format != "png" || decoded.Bounds().Dx() != 2 || len(cleaned) == 0 {
		t.Fatalf("readCleanImage(PNG) = format %q, err %v", format, err)
	}
	if _, _, _, err := readCleanImage(bytes.NewBufferString(`<svg xmlns="http://www.w3.org/2000/svg"/>`)); err == nil {
		t.Fatal("SVG upload was accepted")
	}
}
