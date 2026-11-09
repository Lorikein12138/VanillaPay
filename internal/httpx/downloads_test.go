package httpx

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloads(t *testing.T) {
	dir := t.TempDir()
	const apk = "vanillapay-monitor-1.2.3.apk"
	manifest := `{"version":"1.2.3","file":"` + apk + `","sha256":"` + strings.Repeat("a", 64) +
		`","size":6,"published_at":"2026-11-09T00:00:00Z"}`
	for name, body := range map[string]string{apk: "abcdef", "latest.json": manifest, "private.txt": "private"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "vanillapay-monitor-2.0.0.apk"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		method, name, byteRange, body, contentType, cache string
		status                                            int
	}{
		{"GET", apk, "bytes=1-3", "bcd", "application/vnd.android.package-archive", "public, max-age=31536000, immutable", 206},
		{"HEAD", apk, "", "", "application/vnd.android.package-archive", "public, max-age=31536000, immutable", 200},
		{"GET", "latest.json", "", manifest, "application/json", "no-cache", 200},
		{"POST", apk, "", "", "", "", 405},
		{"GET", "vanillapay-monitor-2.0.0.apk", "", "", "", "", 404},
		{"GET", "private.txt", "", "", "", "", 404},
		{"GET", "sub/" + apk, "", "", "", "", 404},
		{"GET", "../" + apk, "", "", "", "", 404},
	} {
		t.Run(tt.method+"/"+tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, "/downloads/"+tt.name, nil)
			request.Header.Set("Range", tt.byteRange)
			response := httptest.NewRecorder()
			downloadHandler(dir)(response, request)
			if response.Code != tt.status {
				t.Fatalf("status=%d want=%d", response.Code, tt.status)
			}
			if tt.status >= 400 {
				return
			}
			if response.Body.String() != tt.body || response.Header().Get("Content-Type") != tt.contentType ||
				response.Header().Get("Cache-Control") != tt.cache || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("response=%q headers=%v", response.Body, response.Header())
			}
			if tt.name == apk && response.Header().Get("Content-Disposition") != `attachment; filename="`+apk+`"` {
				t.Fatal("APK must download as an attachment")
			}
		})
	}
	t.Run("symbolic link", func(t *testing.T) {
		const link = "vanillapay-monitor-3.0.0.apk"
		if err := os.Symlink(apk, filepath.Join(dir, link)); err != nil {
			t.Skipf("system cannot create symlinks: %v", err)
		}
		response := httptest.NewRecorder()
		downloadHandler(dir)(response, httptest.NewRequest(http.MethodGet, "/downloads/"+link, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("symbolic link served: %d", response.Code)
		}
	})
	for _, path := range []string{"/downloads", "/downloads/", "/downloads/latest.json", "/downloads/" + apk} {
		response := httptest.NewRecorder()
		NewRouter(nil).ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s with downloads disabled: %d", path, response.Code)
		}
	}
	for _, invalid := range []string{"{}", strings.Replace(manifest, "1.2.3", "1.2.4", 1),
		strings.Replace(manifest, `"size":6`, `"size":0`, 1), strings.Replace(manifest, "00:00:00Z", "00:00:00", 1)} {
		if validDownloadManifest([]byte(invalid)) {
			t.Fatalf("invalid manifest accepted: %s", invalid)
		}
	}
}
