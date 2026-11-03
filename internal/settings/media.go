package settings

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/makiuchi-d/gozxing"
	multiqr "github.com/makiuchi-d/gozxing/multi/qrcode"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/platform"
)

const maxImageBytes = 5 << 20

type MediaFile struct {
	Reader io.ReadCloser
	MIME   string
}

func (s *Service) UploadQR(ctx context.Context, actor platform.Actor, channel string, src io.Reader) (QRDTO, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return QRDTO{}, &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	}
	if channel != "alipay" && channel != "wxpay" && channel != "qqpay" {
		return QRDTO{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	data, format, img, err := readCleanImage(src)
	if err != nil {
		return QRDTO{}, err
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return QRDTO{}, &platform.Error{Code: "INVALID_QR", HTTPStatus: 400}
	}
	decoded, err := multiqr.NewQRCodeMultiReader().DecodeMultipleWithoutHint(bitmap)
	if err != nil || len(decoded) != 1 {
		return QRDTO{}, &platform.Error{Code: "INVALID_QR", HTTPStatus: 400}
	}
	payload := decoded[0].GetText()
	if !validQRPayload(channel, payload) {
		return QRDTO{}, &platform.Error{Code: "INVALID_QR", HTTPStatus: 400}
	}
	payloadCipher, err := platform.Encrypt(s.key[:], []byte(payload), s.random)
	if err != nil {
		return QRDTO{}, err
	}
	return s.saveAsset(ctx, actor, channel, "qr", format, data, payloadCipher)
}

func (s *Service) UploadLogo(ctx context.Context, actor platform.Actor, src io.Reader) (map[string]string, error) {
	if actor.Role != "admin" {
		return nil, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	data, format, _, err := readCleanImage(src)
	if err != nil {
		return nil, err
	}
	dto, err := s.saveAsset(ctx, actor, "", "logo", format, data, nil)
	if err != nil {
		return nil, err
	}
	if dto.AssetID == nil {
		return nil, errors.New("logo asset was not persisted")
	}
	return map[string]string{"asset_id": *dto.AssetID, "url": "/media/" + *dto.AssetID}, nil
}

func readCleanImage(src io.Reader) ([]byte, string, image.Image, error) {
	if src == nil {
		return nil, "", nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	original, err := io.ReadAll(io.LimitReader(src, maxImageBytes+1))
	if err != nil {
		return nil, "", nil, err
	}
	if len(original) > maxImageBytes {
		return nil, "", nil, &platform.Error{Code: "UPLOAD_TOO_LARGE", HTTPStatus: 413}
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(original))
	if err != nil || (format != "png" && format != "jpeg") {
		return nil, "", nil, &platform.Error{Code: "UNSUPPORTED_MEDIA_TYPE", HTTPStatus: 415}
	}
	if config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 {
		return nil, "", nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	img, decodedFormat, err := image.Decode(bytes.NewReader(original))
	if err != nil || decodedFormat != format {
		return nil, "", nil, &platform.Error{Code: "UNSUPPORTED_MEDIA_TYPE", HTTPStatus: 415}
	}
	var cleaned bytes.Buffer
	if format == "png" {
		err = png.Encode(&cleaned, img)
	} else {
		err = jpeg.Encode(&cleaned, img, &jpeg.Options{Quality: 90})
	}
	if err != nil {
		return nil, "", nil, err
	}
	return cleaned.Bytes(), format, img, nil
}

func validQRPayload(channel, payload string) bool {
	u, err := url.Parse(payload)
	if err != nil || u.User != nil || u.Fragment != "" {
		return false
	}
	switch channel {
	case "alipay":
		return u.Scheme == "https" && strings.EqualFold(u.Hostname(), "qr.alipay.com")
	case "wxpay":
		return (u.Scheme == "wxp" && strings.HasPrefix(payload, "wxp://") && (u.Host != "" || u.Path != "")) || (u.Scheme == "https" && strings.EqualFold(u.Hostname(), "wx.tenpay.com"))
	case "qqpay":
		return u.Scheme == "https" && (strings.EqualFold(u.Hostname(), "i.qianbao.qq.com") || strings.EqualFold(u.Hostname(), "qianbao.qq.com"))
	default:
		return false
	}
}

func (s *Service) saveAsset(ctx context.Context, actor platform.Actor, channel, kind, format string, data, payloadCipher []byte) (QRDTO, error) {
	if s.dataDir == "" {
		return QRDTO{}, errors.New("DATA_DIR is required for media storage")
	}
	id, err := platform.RandomToken(s.random, 20)
	if err != nil {
		return QRDTO{}, err
	}
	ext := ".jpg"
	mime := "image/jpeg"
	if format == "png" {
		ext = ".png"
		mime = "image/png"
	}
	assetsDir := filepath.Join(s.dataDir, "assets")
	if err := os.MkdirAll(assetsDir, 0700); err != nil {
		return QRDTO{}, err
	}
	finalPath := filepath.Join(assetsDir, id+ext)
	tmp, err := os.CreateTemp(assetsDir, ".upload-*")
	if err != nil {
		return QRDTO{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return QRDTO{}, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return QRDTO{}, err
	}
	if err := tmp.Close(); err != nil {
		return QRDTO{}, err
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		return QRDTO{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(finalPath)
		}
	}()
	checksum := sha256.Sum256(data)
	fileRel, err := filepath.Rel(s.dataDir, finalPath)
	if err != nil {
		return QRDTO{}, err
	}
	fileRel = filepath.ToSlash(fileRel)
	var owner any
	var ownerID *int64
	if kind == "qr" {
		owner = actor.MerchantID
		ownerID = &actor.MerchantID
	}
	var result QRDTO
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if kind == "qr" {
			if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE media_assets SET active=false WHERE owner_merchant_id=$1 AND kind='qr' AND channel=$2 AND active=true`, actor.MerchantID, channel); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO media_assets(id,owner_merchant_id,kind,channel,file_path,sha256,mime,qr_payload_cipher,active) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,true)`, id, owner, kind, channel, fileRel, checksum[:], mime, payloadCipher)
		if err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]any{"kind": kind, "channel": channel})
		if err := platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, MerchantID: ownerID, Action: "media.upload", TargetType: "media_asset", TargetID: id, Result: "success", SafeMetadata: metadata}); err != nil {
			return err
		}
		if kind == "qr" {
			result = QRDTO{Channel: channel, AssetID: &id, Enabled: true}
		} else {
			result = QRDTO{AssetID: &id, Enabled: true}
		}
		return nil
	})
	if err != nil {
		return QRDTO{}, err
	}
	committed = true
	return result, nil
}

func (s *Service) SignCheckoutMedia(tradeNo, assetID string, expires time.Time) string {
	message := tradeNo + "\n" + assetID + "\n" + strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) OpenAsset(ctx context.Context, assetID string, merchantID int64, tradeNo, expiry, signature string) (MediaFile, error) {
	var path, mime, kind string
	var owner *int64
	var active bool
	if err := s.pool.QueryRow(ctx, `SELECT file_path,mime,kind,owner_merchant_id,active FROM media_assets WHERE id=$1`, assetID).Scan(&path, &mime, &kind, &owner, &active); err != nil {
		return MediaFile{}, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}
	}
	if !active && kind != "qr" {
		return MediaFile{}, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}
	}
	allowed := kind == "logo" && owner == nil
	if owner != nil && merchantID == *owner {
		allowed = true
	}
	if !allowed && tradeNo != "" && expiry != "" && signature != "" {
		unix, err := strconv.ParseInt(expiry, 10, 64)
		if err == nil {
			validUntil := time.Unix(unix, 0)
			if validUntil.After(s.clock.Now()) && hmac.Equal([]byte(strings.ToLower(signature)), []byte(s.SignCheckoutMedia(tradeNo, assetID, validUntil))) {
				var orderOK bool
				if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM orders WHERE trade_no=$1 AND qr_asset_id=$2 AND status='pending' AND expires_at>=$3 AND expires_at>$4)`, tradeNo, assetID, validUntil, s.clock.Now()).Scan(&orderOK); err == nil && orderOK {
					allowed = true
				}
			}
		}
	}
	if !allowed {
		return MediaFile{}, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}
	}
	full := filepath.Join(s.dataDir, filepath.FromSlash(path))
	root, err := filepath.Abs(s.dataDir)
	if err != nil {
		return MediaFile{}, err
	}
	full, err = filepath.Abs(full)
	if err != nil || !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return MediaFile{}, fmt.Errorf("media path is outside data directory")
	}
	file, err := os.Open(full)
	if err != nil {
		return MediaFile{}, err
	}
	return MediaFile{Reader: file, MIME: mime}, nil
}
