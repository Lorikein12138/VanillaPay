package settings

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_FaviconSourcesAndCache(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	admin := bootstrapRecoveryAdmin(t, fixture, "favicon")
	get := func(path string) (string, []byte) {
		t.Helper()
		res := fixture.Request(t, http.MethodGet, path, nil, "guest")
		etag := res.Header().Get("ETag")
		if res.Code != http.StatusOK || len(etag) != 66 || etag[0] != 34 || etag[65] != 34 || res.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("icon %s status=%d or missing strong cache headers", path, res.Code)
		}
		cached := fixture.RequestWithHeaders(t, http.MethodGet, path, nil, "guest", http.Header{"If-None-Match": {etag}})
		if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 || cached.Header().Get("ETag") != etag || cached.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("icon %s conditional status=%d", path, cached.Code)
		}
		return etag, res.Body.Bytes()
	}
	getVersion := func() string {
		t.Helper()
		res := fixture.Request(t, http.MethodGet, "/api/v1/public/config", nil, "guest")
		var env struct {
			Data struct {
				FaviconURL string `json:"favicon_url"`
			} `json:"data"`
		}
		if json.Unmarshal(res.Body.Bytes(), &env) != nil || env.Data.FaviconURL == "" {
			t.Fatal("read public favicon version")
		}
		return env.Data.FaviconURL
	}
	assertPNG := func(data []byte, size int, want color.Color) {
		t.Helper()
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil || img.Bounds().Size() != (image.Point{X: size, Y: size}) {
			t.Fatalf("favicon PNG size=%d decode=%v", size, err)
		}
		if want != nil && color.NRGBAModel.Convert(img.At(0, 0)) != want {
			t.Fatal("favicon did not center crop the selected image")
		}
	}
	defaultETag, defaultICO := get("/favicon.ico")
	if len(defaultICO) < 54 || binary.LittleEndian.Uint16(defaultICO[2:4]) != 1 || binary.LittleEndian.Uint16(defaultICO[4:6]) != 3 {
		t.Fatal("invalid ICO directory")
	}
	for i, size := range []int{16, 32, 48} {
		entry := defaultICO[6+16*i : 6+16*(i+1)]
		length, offset := int(binary.LittleEndian.Uint32(entry[8:12])), int(binary.LittleEndian.Uint32(entry[12:16]))
		if int(entry[0]) != size || int(entry[1]) != size || offset < 54 || length <= 0 || offset+length > len(defaultICO) {
			t.Fatalf("invalid ICO entry size=%d", size)
		}
		assertPNG(defaultICO[offset:offset+length], size, nil)
	}
	defaultVersion := getVersion()
	red := color.NRGBA{R: 240, A: 255}
	blue := color.NRGBA{B: 240, A: 255}
	imgBytes := func(fill color.Color) *bytes.Buffer {
		t.Helper()
		img := image.NewNRGBA(image.Rect(0, 0, 90, 30))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
		draw.Draw(img, image.Rect(30, 0, 60, 30), image.NewUniform(fill), image.Point{}, draw.Src)
		var data bytes.Buffer
		if err := png.Encode(&data, img); err != nil {
			t.Fatal(err)
		}
		return &data
	}
	service := fixture.App.Services.Settings
	actor := platform.Actor{Role: "admin"}
	logo, err := service.UploadLogo(t.Context(), actor, imgBytes(red))
	if err != nil {
		t.Fatal(err)
	}
	favicon, err := service.UploadFavicon(t.Context(), actor, imgBytes(blue))
	if err != nil {
		t.Fatal(err)
	}
	logoID, iconID := logo["asset_id"], favicon["asset_id"]
	if etag, _ := get("/favicon.ico"); etag != defaultETag || getVersion() != defaultVersion {
		t.Fatal("upload published an unsaved icon")
	}
	save := func(logo, icon any, status int, code string) {
		t.Helper()
		assertRecoveryAPIError(t, updateSettingsGroupHTTP(t, fixture, admin, "site", map[string]any{"site_name": "Icon Test", "logo_asset_id": logo, "favicon_asset_id": icon}), status, code)
	}
	save(logoID, logoID, http.StatusBadRequest, "VALIDATION_ERROR")
	save(logoID, nil, http.StatusOK, "")
	logoVersion := getVersion()
	if logoVersion == defaultVersion {
		t.Fatal("logo did not change public favicon version")
	}
	_, data := get("/favicon-32.png")
	assertPNG(data, 32, red)
	save(logoID, iconID, http.StatusOK, "")
	if getVersion() == logoVersion {
		t.Fatal("dedicated icon did not change public favicon version")
	}
	_, data = get("/apple-touch-icon.png")
	assertPNG(data, 180, blue)
	save(logoID, nil, http.StatusOK, "")
	_, data = get("/favicon-32.png")
	assertPNG(data, 32, red)
	save(nil, nil, http.StatusOK, "")
	if etag, _ := get("/favicon.ico"); etag != defaultETag || getVersion() != defaultVersion {
		t.Fatal("removal did not restore default icon")
	}
	corrupt := func(assetID string) {
		t.Helper()
		var path string
		if err := db.Pool.QueryRow(t.Context(), `SELECT file_path FROM media_assets WHERE id=$1`, assetID).Scan(&path); err != nil {
			t.Fatal("read icon asset path")
		}
		if err := os.WriteFile(filepath.Join(fixture.DataDir, filepath.FromSlash(path)), []byte("not an image"), 0o600); err != nil {
			t.Fatal("corrupt icon asset")
		}
	}
	save(logoID, iconID, http.StatusOK, "")
	corrupt(iconID)
	corrupt(logoID)
	if etag, _ := get("/favicon.ico"); etag != defaultETag {
		t.Fatal("undecodable icon sources did not fall back to the default icon")
	}
	freshLogo, err := service.UploadLogo(t.Context(), actor, imgBytes(red))
	if err != nil {
		t.Fatal(err)
	}
	freshIcon, err := service.UploadFavicon(t.Context(), actor, imgBytes(blue))
	if err != nil {
		t.Fatal(err)
	}
	save(freshLogo["asset_id"], freshIcon["asset_id"], http.StatusOK, "")
	corrupt(freshIcon["asset_id"])
	_, data = get("/favicon-32.png")
	assertPNG(data, 32, red)
}
