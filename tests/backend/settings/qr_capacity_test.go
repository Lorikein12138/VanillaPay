package settings

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

// A wallet code printed at error-correction level L can hold more than level M
// allows; the upload must still regenerate it instead of rejecting it.
func TestDB_QRUploadKeepsPayloadBeyondMediumCapacity(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	payload := "https://qr.alipay.com/" + strings.Repeat("q", 2400)
	matrix, err := qrcode.NewQRCodeWriter().Encode(payload, gozxing.BarcodeFormat_QR_CODE, 1110, 1110, map[gozxing.EncodeHintType]any{
		gozxing.EncodeHintType_ERROR_CORRECTION: "L",
		gozxing.EncodeHintType_MARGIN:           4,
	})
	if err != nil {
		t.Fatalf("encode level-L fixture: %v", err)
	}
	source := image.NewGray(image.Rect(0, 0, matrix.GetWidth(), matrix.GetHeight()))
	for y := range matrix.GetHeight() {
		for x := range matrix.GetWidth() {
			if !matrix.Get(x, y) {
				source.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	asset, err := application.Services.Settings.UploadQR(t.Context(), actor, "alipay", bytes.NewReader(encoded.Bytes()))
	if err != nil || asset.AssetID == nil {
		t.Fatalf("upload level-L QR beyond M capacity: asset=%#v err=%v", asset, err)
	}
	media, err := application.Services.Settings.OpenAsset(t.Context(), *asset.AssetID, merchantID, "", "", "")
	if err != nil {
		t.Fatalf("open regenerated QR: %v", err)
	}
	defer func() { _ = media.Reader.Close() }() // Cleanup must not replace the operation result.
	cleaned, err := io.ReadAll(media.Reader)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(cleaned))
	if err != nil || img.Bounds().Size() != (image.Point{X: 512, Y: 512}) {
		t.Fatalf("regenerated QR format/size: err=%v", err)
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := qrcode.NewQRCodeReader().Decode(bitmap, map[gozxing.DecodeHintType]any{gozxing.DecodeHintType_PURE_BARCODE: true})
	if err != nil || decoded.GetText() != payload {
		t.Fatalf("regenerated QR payload mismatch: err=%v", err)
	}
}
