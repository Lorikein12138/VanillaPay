package platform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignInlineQR scopes the capability to an inline media path and a single order.
func SignInlineQR(key []byte, trade, path, expiry string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("inline-qr\n" + trade + "\n" + path + "\n" + expiry))
	return hex.EncodeToString(mac.Sum(nil))
}
