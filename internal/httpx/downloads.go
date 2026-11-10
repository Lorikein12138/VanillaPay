package httpx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"vanillapay/internal/platform"
)

var apkName = regexp.MustCompile(`^vanillapay-monitor-(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.apk$`)
var apkDigest = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

type downloadManifest struct {
	Version     string `json:"version"`
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	PublishedAt string `json:"published_at"`
}

func downloadHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/downloads/")
		if dir == "" || (name != "latest.json" && !apkName.MatchString(name)) {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = root.Close() }()
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		file, err := root.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = file.Close() }()
		actual, err := file.Stat()
		if err != nil || !os.SameFile(info, actual) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if name == "latest.json" {
			const limit = 4096
			payload, err := io.ReadAll(io.LimitReader(file, limit+1))
			if err != nil || len(payload) > limit || !validDownloadManifest(payload) {
				http.NotFound(w, r)
				return
			}
			digest := sha256.Sum256(payload)
			w.Header().Set("ETag", `"`+hex.EncodeToString(digest[:])+`"`)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-cache")
			// Release mtimes can go backwards on rollback. Only validate by content.
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(payload))
			return
		}
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		// A new image can contain a rebuilt APK with the same version and URL.
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, name, time.Time{}, file)
	}
}

func validDownloadManifest(payload []byte) bool {
	var manifest downloadManifest
	if err := platform.DecodeStrictJSON(bytes.NewReader(payload), &manifest); err != nil {
		return false
	}
	_, err := time.Parse(time.RFC3339, manifest.PublishedAt)
	return err == nil && manifest.File == "vanillapay-monitor-"+manifest.Version+".apk" &&
		apkName.MatchString(manifest.File) && apkDigest.MatchString(manifest.SHA256) && manifest.Size > 0
}
