package settings

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"vanillapay/internal/platform"
)

//go:embed default-icon.png
var defaultIcon []byte

type iconRepresentation struct {
	data []byte
	mime string
	etag string
}

type siteIcons struct {
	mu       sync.Mutex
	source   string
	variants map[string]iconRepresentation
}

func faviconURL(site SiteGroupRequest) string {
	version := "default"
	for _, id := range []*string{site.FaviconAssetID, site.LogoAssetID} {
		if id != nil && *id != "" {
			version = fmt.Sprintf("%x", sha256.Sum256([]byte(*id)))
			break
		}
	}
	return "/favicon.ico?v=" + version
}

func validateFaviconAsset(ctx context.Context, tx pgx.Tx, clean json.RawMessage) error {
	var site SiteGroupRequest
	if err := json.Unmarshal(clean, &site); err != nil {
		return err
	}
	if site.FaviconAssetID == nil {
		return nil
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_assets
		WHERE id=$1 AND kind='favicon' AND active AND owner_merchant_id IS NULL)`, *site.FaviconAssetID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	return nil
}

func (h *Handler) Favicon(w http.ResponseWriter, r *http.Request) {
	icon, err := h.Service.siteIcon(r.Context(), r.URL.Path)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", icon.mime)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", icon.etag)
	http.ServeContent(w, r, r.URL.Path, time.Time{}, bytes.NewReader(icon.data))
}

func (s *Service) siteIcon(ctx context.Context, path string) (iconRepresentation, error) {
	var favicon, logo string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(f.file_path,''), COALESCE(l.file_path,'')
		FROM site_settings s
		LEFT JOIN media_assets f ON f.id=s.site->>'favicon_asset_id' AND f.kind='favicon' AND f.active AND f.owner_merchant_id IS NULL
		LEFT JOIN media_assets l ON l.id=s.site->>'logo_asset_id' AND l.kind='logo' AND l.active AND l.owner_merchant_id IS NULL
		WHERE s.singleton`).Scan(&favicon, &logo)
	if err != nil {
		return iconRepresentation{}, err
	}
	s.icons.mu.Lock()
	defer s.icons.mu.Unlock()
	for _, source := range []string{favicon, logo} {
		if source == "" {
			continue
		}
		if source == s.icons.source && s.icons.variants != nil {
			return s.icons.variants[path], nil
		}
		file, err := s.openMediaPath(source)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return iconRepresentation{}, err
		}
		img, _, err := image.Decode(file)
		_ = file.Close() // Preserve the decoding result.
		if err != nil {
			// A damaged or unsupported file must not take the icon down: try the
			// next source and finally the built-in icon.
			slog.WarnContext(ctx, "site_icon_undecodable", "source", source, "error", err)
			continue
		}
		return s.cacheIcons(source, path, img)
	}
	if s.icons.source == "" && s.icons.variants != nil {
		return s.icons.variants[path], nil
	}
	img, err := png.Decode(bytes.NewReader(defaultIcon))
	if err != nil {
		return iconRepresentation{}, err
	}
	return s.cacheIcons("", path, img)
}

func (s *Service) cacheIcons(source, path string, img image.Image) (iconRepresentation, error) {
	var sizes = [...]int{16, 32, 48, 180}
	var pngs [len(sizes)][]byte
	for i, size := range sizes {
		data, err := iconPNG(img, size)
		if err != nil {
			return iconRepresentation{}, err
		}
		pngs[i] = data
	}
	s.icons.source = source
	s.icons.variants = map[string]iconRepresentation{
		"/favicon.ico":          iconResponse(iconICO(sizes[:3], pngs[:3]), "image/x-icon"),
		"/favicon-32.png":       iconResponse(pngs[1], "image/png"),
		"/apple-touch-icon.png": iconResponse(pngs[3], "image/png"),
	}
	return s.icons.variants[path], nil
}

func iconResponse(data []byte, mime string) iconRepresentation {
	return iconRepresentation{data: data, mime: mime, etag: fmt.Sprintf("\"%x\"", sha256.Sum256(data))}
}

func iconPNG(src image.Image, size int) ([]byte, error) {
	bounds := src.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	left := bounds.Min.X + (bounds.Dx()-side)/2
	top := bounds.Min.Y + (bounds.Dy()-side)/2
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.Set(x, y, src.At(left+(2*x+1)*side/(2*size), top+(2*y+1)*side/(2*size)))
		}
	}
	var data bytes.Buffer
	err := png.Encode(&data, img)
	return data.Bytes(), err
}

// ICO stores a directory of offsets followed by complete PNG images.
func iconICO(sizes []int, pngs [][]byte) []byte {
	header := make([]byte, 6+16*len(sizes))
	binary.LittleEndian.PutUint16(header[2:4], 1)
	binary.LittleEndian.PutUint16(header[4:6], uint16(len(sizes)))
	offset := len(header)
	for i, size := range sizes {
		entry := header[6+i*16 : 6+(i+1)*16]
		entry[0], entry[1] = byte(size), byte(size)
		binary.LittleEndian.PutUint16(entry[4:6], 1)
		binary.LittleEndian.PutUint16(entry[6:8], 32)
		binary.LittleEndian.PutUint32(entry[8:12], uint32(len(pngs[i])))
		binary.LittleEndian.PutUint32(entry[12:16], uint32(offset))
		offset += len(pngs[i])
	}
	for _, data := range pngs {
		header = append(header, data...)
	}
	return header
}
