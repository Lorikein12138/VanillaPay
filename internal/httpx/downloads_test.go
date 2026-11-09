package httpx

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
			downloadHandler(dir, "")(response, request)
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
		downloadHandler(dir, "")(response, httptest.NewRequest(http.MethodGet, "/downloads/"+link, nil))
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

func TestManifestRevalidationByContent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "latest.json")
	modtime := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	write := func(version string, stamp time.Time) string {
		t.Helper()
		body := `{"version":"` + version + `","file":"vanillapay-monitor-` + version +
			`.apk","sha256":"` + strings.Repeat("a", 64) + `","size":6,"published_at":"2026-11-09T00:00:00Z"}`
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(file, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		return body
	}
	get := func(etag, modifiedSince, body string, status int) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/downloads/latest.json", nil)
		req.Header.Set("If-None-Match", etag)
		req.Header.Set("If-Modified-Since", modifiedSince)
		res := httptest.NewRecorder()
		downloadHandler(dir, "")(res, req)
		if res.Code != status || res.Body.String() != body || res.Header().Get("Last-Modified") != "" {
			t.Fatalf("status=%d body=%q headers=%v", res.Code, res.Body, res.Header())
		}
		tag := res.Header().Get("ETag")
		if !strings.HasPrefix(tag, `"`) || !strings.HasSuffix(tag, `"`) || len(tag) < 34 {
			t.Fatalf("expected a strong content ETag, got %q", tag)
		}
		return tag
	}
	first := write("1.0.0", modtime)
	get("", modtime.Add(time.Hour).Format(http.TimeFormat), first, http.StatusOK)
	firstTag := get("", "", first, http.StatusOK)
	get(firstTag, "", "", http.StatusNotModified)
	second := write("1.0.1", modtime)
	secondTag := get(firstTag, "", second, http.StatusOK)
	if firstTag == secondTag {
		t.Fatal("different manifests with the same mtime share an ETag")
	}
	write("1.0.0", modtime.Add(-time.Hour))
	if tag := get(secondTag, "", first, http.StatusOK); tag != firstTag {
		t.Fatal("rollback did not restore the original content ETag")
	}
	for _, date := range []string{modtime.Format(http.TimeFormat), "invalid date", modtime.Add(-2 * time.Hour).Format(http.TimeFormat)} {
		get("", date, first, http.StatusOK)
	}
}
