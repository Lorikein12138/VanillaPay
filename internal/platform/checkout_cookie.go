package platform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func CheckoutCookieName(trade string) string {
	digest := sha256.Sum256([]byte(trade))
	return "vp_checkout_" + hex.EncodeToString(digest[:12])
}

func CheckoutCookieValue(key []byte, trade string, expiry int64) string {
	stamp := strconv.FormatInt(expiry, 10)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("checkout-verification\n" + trade + "\n" + stamp))
	return stamp + "." + hex.EncodeToString(mac.Sum(nil))
}

func CheckoutVerified(key []byte, trade, token string, now, expires time.Time) bool {
	stamp, _, found := strings.Cut(token, ".")
	expiry, err := strconv.ParseInt(stamp, 10, 64)
	return found && err == nil && expiry > now.Unix() && expiry <= expires.Unix() &&
		hmac.Equal([]byte(token), []byte(CheckoutCookieValue(key, trade, expiry)))
}

func CheckoutCookieToken(r *http.Request, trade string) string {
	cookie, err := r.Cookie(CheckoutCookieName(trade))
	if err != nil {
		return ""
	}
	return cookie.Value
}
