package settings

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_QRAssetLifecycle(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	app := db.NewApplication(t)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	for _, item := range []struct {
		channel string
		payload string
	}{
		{channel: "alipay", payload: "https://qr.alipay.com/asset-lifecycle"},
		{channel: "wxpay", payload: "wxp://f2f0?asset=lifecycle"},
	} {

		t.Run("V19.B.r01/"+item.channel, func(t *testing.T) {
			asset, err := app.Services.Settings.UploadQR(t.Context(), actor, item.channel, bytes.NewReader(qrPNG(t, item.payload)))
			if err != nil || asset.AssetID == nil || !asset.Enabled || asset.Channel != item.channel {
				t.Fatalf("upload valid %s QR = %#v, %v", item.channel, asset, err)
			}
			var storedPayload []byte
			var active bool
			if err := db.Pool.QueryRow(t.Context(), `SELECT qr_payload_cipher,active FROM media_assets WHERE id=$1 AND owner_merchant_id=$2`, *asset.AssetID, merchantID).Scan(&storedPayload, &active); err != nil || len(storedPayload) == 0 || !active {
				t.Fatalf("stored %s QR payload/active state: bytes=%d active=%t err=%v", item.channel, len(storedPayload), active, err)
			}
		})
	}
	t.Run("V19.B.r01/v01", func(t *testing.T) {
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM media_assets WHERE owner_merchant_id=$1 AND kind='qr' AND active=true`, merchantID).Scan(&count); err != nil || count != 2 {
			t.Fatalf("active valid QR slots=%d err=%v, want both channels", count, err)
		}
	})

	t.Run("V19.B.r02/invalid_payload_does_not_create_orphan", func(t *testing.T) {
		var before int
		if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM media_assets WHERE owner_merchant_id=$1 AND kind='qr'`, merchantID).Scan(&before); err != nil {
			t.Fatal("count QR assets before invalid upload")
		}
		_, err := app.Services.Settings.UploadQR(t.Context(), actor, "alipay", bytes.NewReader(qrPNG(t, "https://attacker.example.invalid/payload")))
		if err == nil {
			t.Fatal("QR with a payload from an unapproved provider host was accepted")
		}
		for name, invalid := range map[string][]byte{
			"wrong_channel": qrPNG(t, "https://qr.alipay.com/wrong-channel"),
			"corrupt_image": []byte("not an image"),
			"svg":           []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
			"oversize":      bytes.Repeat([]byte{0}, (5<<20)+1),
		} {
			if _, invalidErr := app.Services.Settings.UploadQR(t.Context(), actor, "wxpay", bytes.NewReader(invalid)); invalidErr == nil {
				t.Errorf("invalid QR input %s was accepted", name)
			}
		}
		var after int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM media_assets WHERE owner_merchant_id=$1 AND kind='qr'`, merchantID).Scan(&after); err != nil || after != before {
			t.Fatalf("invalid QR left an orphan asset: before=%d after=%d err=%v", before, after, err)
		}
	})

	t.Run("V19.B.r02/v01", func(t *testing.T) {
		if _, err := app.Services.Settings.UploadQR(t.Context(), actor, "alipay", bytes.NewReader(twoQRPNG(t))); err == nil {
			t.Fatal("image containing two QR codes was accepted")
		}
	})
}

func TestDB_QRUploadRegeneratesPNGWithoutFrameOrExif(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	t.Run("V19.B.r03/v01", func(t *testing.T) {
		const payload = "https://qr.alipay.com/exif-clean"
		source, err := png.Decode(bytes.NewReader(qrPNG(t, payload)))
		if err != nil {
			t.Fatal(err)
		}
		frame := image.NewRGBA(image.Rect(0, 0, 464, 484))
		draw.Draw(frame, frame.Bounds(), image.NewUniform(color.RGBA{R: 40, G: 140, B: 205, A: 255}), image.Point{}, draw.Src)
		draw.Draw(frame, source.Bounds().Add(image.Pt(40, 50)), source, image.Point{}, draw.Src)
		var framed bytes.Buffer
		if err := png.Encode(&framed, frame); err != nil {
			t.Fatal(err)
		}
		encoded := qrJPEGWithExif(t, framed.Bytes())
		asset, err := application.Services.Settings.UploadQR(t.Context(), actor, "alipay", bytes.NewReader(encoded))
		if err != nil || asset.AssetID == nil {
			t.Fatalf("upload JPEG with EXIF fixture: asset=%#v err=%v", asset, err)
		}
		media, err := application.Services.Settings.OpenAsset(t.Context(), *asset.AssetID, merchantID, "", "", "")
		if err != nil {
			t.Fatalf("read generated QR asset: %v", err)
		}
		defer func() { _ = media.Reader.Close() }() // Cleanup must not replace the operation result.
		cleaned, err := io.ReadAll(media.Reader)
		if err != nil {
			t.Fatalf("read generated QR bytes: %v", err)
		}
		if bytes.Contains(cleaned, []byte("Exif\x00\x00")) || bytes.Contains(cleaned, []byte("private-location-marker")) {
			t.Fatal("re-encoded image retained original EXIF metadata")
		}
		img, err := png.Decode(bytes.NewReader(cleaned))
		if err != nil || media.MIME != "image/png" || img.Bounds().Size() != (image.Point{X: 512, Y: 512}) {
			t.Fatalf("generated QR format/size: mime=%s err=%v", media.MIME, err)
		}
		for y := range 512 {
			for x := range 512 {
				p := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
				r, g, b, a := img.At(x, y).RGBA()
				if r != g || g != b || a != 65535 || (p.Y != 0 && p.Y != 255) {
					t.Fatal("generated QR contains pixels other than black and white")
				}
			}
		}
		bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := qrcode.NewQRCodeReader().DecodeWithoutHints(bitmap)
		if err != nil || decoded.GetText() != payload {
			t.Fatalf("regenerated QR payload did not match: %v", err)
		}
		checksum := sha256.Sum256(cleaned)
		var stored []byte
		if err := db.Pool.QueryRow(t.Context(), `SELECT sha256 FROM media_assets WHERE id=$1`, *asset.AssetID).Scan(&stored); err != nil || !bytes.Equal(stored, checksum[:]) {
			t.Fatalf("stored image checksum does not match sanitized bytes: stored=%x actual=%x err=%v", stored, checksum, err)
		}
	})
}

func qrJPEGWithExif(t *testing.T, pngBytes []byte) []byte {
	t.Helper()
	source, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("decode generated QR PNG: %v", err)
	}
	var jpegBytes bytes.Buffer
	if err := jpeg.Encode(&jpegBytes, source, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode EXIF source JPEG: %v", err)
	}
	marker := append([]byte("Exif\x00\x00"), []byte("private-location-marker")...)
	segmentLength := len(marker) + 2
	segment := []byte{0xff, 0xe1, byte(segmentLength >> 8), byte(segmentLength)}
	segment = append(segment, marker...)
	original := jpegBytes.Bytes()
	if len(original) < 2 || original[0] != 0xff || original[1] != 0xd8 {
		t.Fatal("JPEG encoder produced invalid SOI marker")
	}
	result := make([]byte, 0, len(original)+len(segment))
	result = append(result, original[:2]...)
	result = append(result, segment...)
	result = append(result, original[2:]...)
	return result
}

func qrPNG(t *testing.T, payload string) []byte {
	t.Helper()
	matrix, err := qrcode.NewQRCodeWriter().EncodeWithoutHint(payload, gozxing.BarcodeFormat_QR_CODE, 384, 384)
	if err != nil {
		t.Fatalf("encode independent QR fixture: %v", err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, matrix.GetWidth(), matrix.GetHeight()))
	for y := 0; y < matrix.GetHeight(); y++ {
		for x := 0; x < matrix.GetWidth(); x++ {
			if matrix.Get(x, y) {
				img.SetNRGBA(x, y, color.NRGBA{A: 255})
			} else {
				img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatalf("encode PNG fixture: %v", err)
	}
	return encoded.Bytes()
}

func twoQRPNG(t *testing.T) []byte {
	t.Helper()
	left, err := qrcode.NewQRCodeWriter().EncodeWithoutHint("https://qr.alipay.com/first", gozxing.BarcodeFormat_QR_CODE, 384, 384)
	if err != nil {
		t.Fatal(err)
	}
	right, err := qrcode.NewQRCodeWriter().EncodeWithoutHint("https://qr.alipay.com/second", gozxing.BarcodeFormat_QR_CODE, 384, 384)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 768, 384))
	for y := 0; y < 384; y++ {
		for x := 0; x < 768; x++ {
			black := left.Get(x, y)
			if x >= 384 {
				black = right.Get(x-384, y)
			}
			if black {
				img.SetNRGBA(x, y, color.NRGBA{A: 255})
			} else {
				img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestHTTP_QRAssetReplacementKeepsPendingSnapshot(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "media-snapshot-plan", 1000, 3650, []string{"alipay"}, nil)
	fixture := testsupport.NewHTTPFixture(t, db)
	db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	merchant := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	initialAsset, err := fixture.App.Services.Settings.UploadQR(t.Context(), merchant, "alipay", bytes.NewReader(qrPNG(t, "https://qr.alipay.com/initial")))
	if err != nil || initialAsset.AssetID == nil {
		t.Fatalf("write a real QR file for the pending order: %#v err=%v", initialAsset, err)
	}
	created, err := fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
		MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "pending-old-qr-asset", Name: "asset snapshot", OriginalCents: 100,
		NotifyURL: "https://merchant.example.test/notify", ReturnURL: "https://merchant.example.test/return", SourceIP: "192.0.2.50", Actor: merchant,
	})
	if err != nil {
		t.Fatalf("create pending order before replacement: %v", err)
	}
	before, err := fixture.App.Services.Payment.SelectChannel(t.Context(), created.TradeNo, "alipay")
	if err != nil || before.QRImageURL == nil {
		t.Fatalf("lock original QR asset: %#v err=%v", before, err)
	}
	replacement, err := fixture.App.Services.Settings.UploadQR(t.Context(), merchant, "alipay", bytes.NewReader(qrPNG(t, "https://qr.alipay.com/replacement")))
	if err != nil || replacement.AssetID == nil {
		t.Fatalf("replace active QR asset: %#v err=%v", replacement, err)
	}
	after, err := fixture.App.Services.Payment.SelectChannel(t.Context(), created.TradeNo, "alipay")
	if err != nil || after.QRImageURL == nil || *after.QRImageURL != *before.QRImageURL {
		t.Fatalf("pending QR snapshot changed after replacement: before=%v after=%v err=%v", before.QRImageURL, after.QRImageURL, err)
	}
	var active, oldActive bool
	if err := db.Pool.QueryRow(t.Context(), `SELECT active FROM media_assets WHERE id=$1`, *replacement.AssetID).Scan(&active); err != nil {
		t.Fatal("read replacement QR active state")
	}
	if err := db.Pool.QueryRow(t.Context(), `SELECT active FROM media_assets WHERE id=$1`, *initialAsset.AssetID).Scan(&oldActive); err != nil || !active || oldActive {
		t.Fatalf("QR active states replacement=%t original=%t err=%v", active, oldActive, err)
	}
	signed, err := url.Parse(*after.QRImageURL)
	if err != nil || signed.Path == "" {
		t.Fatalf("parse signed image URL %q: %v", *after.QRImageURL, err)
	}
	t.Run("V19.B.r05/v01", func(t *testing.T) {
		served := fixture.Request(t, http.MethodGet, signed.RequestURI(), nil, "guest")
		if served.Code != http.StatusOK || served.Header().Get("Content-Type") != "image/png" {
			t.Fatalf("original pending QR asset response status=%d type=%q body=%s", served.Code, served.Header().Get("Content-Type"), served.Body.String())
		}
	})
	t.Run("V19.B.r05/v02", func(t *testing.T) {
		if !strings.HasPrefix(signed.Path, "/media/") || strings.TrimPrefix(signed.Path, "/media/") == "" {
			t.Fatal("signed image path omitted its asset ID")
		}
		tamperedAsset := "/media/" + url.PathEscape(*replacement.AssetID) + "?" + signed.RawQuery
		assetResponse := fixture.Request(t, http.MethodGet, tamperedAsset, nil, "guest")
		if assetResponse.Code < http.StatusBadRequest || assetResponse.Code >= http.StatusInternalServerError {
			t.Fatalf("substituted asset ID response=%d body=%s", assetResponse.Code, assetResponse.Body.String())
		}
		query := signed.Query()
		query.Set("expires", "1")
		expired := fixture.Request(t, http.MethodGet, signed.Path+"?"+query.Encode(), nil, "guest")
		if expired.Code < http.StatusBadRequest || expired.Code >= http.StatusInternalServerError {
			t.Fatalf("expired signed image response=%d body=%s", expired.Code, expired.Body.String())
		}
	})
	t.Run("V19.B.r05/v03", func(t *testing.T) {
		response := fixture.Request(t, http.MethodGet, "/media/..%2F..%2Fgo.mod", nil, "guest")
		if response.Code == http.StatusOK || strings.Contains(response.Body.String(), "module vanillapay") {
			t.Fatalf("media path traversal response=%d body=%s", response.Code, response.Body.String())
		}
	})
	t.Run("V19.B.r04/v01", func(t *testing.T) {
		var active, oldActive bool
		if err := db.Pool.QueryRow(t.Context(), `SELECT active FROM media_assets WHERE id=$1`, *replacement.AssetID).Scan(&active); err != nil {
			t.Fatal("read replacement QR state")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT active FROM media_assets WHERE id=$1`, *initialAsset.AssetID).Scan(&oldActive); err != nil || !active || oldActive {
			t.Fatalf("QR replacement state new=%t old=%t err=%v", active, oldActive, err)
		}
	})
	t.Run("V19.B.r04/v02", func(t *testing.T) {
		if after.QRImageURL == nil || before.QRImageURL == nil || *after.QRImageURL != *before.QRImageURL {
			t.Fatalf("pending order did not keep old QR asset URL: before=%v after=%v", before.QRImageURL, after.QRImageURL)
		}
	})
}
