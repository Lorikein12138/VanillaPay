package webassets

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
)

//go:embed all:*
var embedded embed.FS

var hashedAssetName = regexp.MustCompile(`(?:^|[-.])[A-Fa-f0-9]{8,}(?:[.-]|$)`)

type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

// FS returns the built Vite distribution. It is empty in a source-only
// checkout until the frontend build creates internal/webassets/dist.
func FS() fs.FS {
	distribution, err := fs.Sub(embedded, "dist")
	if err != nil {
		return emptyFS{}
	}
	return distribution
}

// SPAHandler serves real embedded assets and uses index.html for ordinary
// client-side routes. API, health, media, PHP-like, and missing asset paths
// remain real 404 responses instead of being rewritten to the SPA.
func SPAHandler(assets fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		name, ok := safeAssetName(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if name == "" {
			serveIndex(w, r, assets)
			return
		}

		if body, err := fs.ReadFile(assets, name); err == nil {
			serveAsset(w, r, name, body)
			return
		}
		if isReservedPath(r.URL.Path) || path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		serveIndex(w, r, assets)
	})
}

func safeAssetName(requestPath string) (string, bool) {
	if strings.Contains(requestPath, "\\") {
		return "", false
	}
	for _, segment := range strings.Split(requestPath, "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}
	clean := path.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	name := strings.TrimPrefix(clean, "/")
	if name == "." {
		return "", true
	}
	if name != "" && !fs.ValidPath(name) {
		return "", false
	}
	return name, true
}

func isReservedPath(requestPath string) bool {
	lower := strings.ToLower(requestPath)
	for _, prefix := range []string{"/api", "/health", "/assets", "/media"} {
		if lower == prefix || strings.HasPrefix(lower, prefix+"/") {
			return true
		}
	}
	return strings.HasSuffix(lower, ".php")
}

func serveIndex(w http.ResponseWriter, r *http.Request, assets fs.FS) {
	body, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func serveAsset(w http.ResponseWriter, r *http.Request, name string, body []byte) {
	contentType := "application/octet-stream"
	if extension := path.Ext(name); extension != "" {
		if detected := mimeType(extension); detected != "" {
			contentType = detected
		}
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if hashedAssetName.MatchString(path.Base(name)) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func mimeType(extension string) string {
	return map[string]string{
		".css":   "text/css; charset=utf-8",
		".html":  "text/html; charset=utf-8",
		".ico":   "image/x-icon",
		".js":    "text/javascript; charset=utf-8",
		".json":  "application/json; charset=utf-8",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".png":   "image/png",
		".svg":   "image/svg+xml",
		".wasm":  "application/wasm",
		".webp":  "image/webp",
		".woff2": "font/woff2",
	}[strings.ToLower(extension)]
}
