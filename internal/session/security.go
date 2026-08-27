package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	minimumCSRFSecretBytes = 32
	maximumUserAgentBytes  = 256
	csrfDomain             = "csrf-v1\x00"
)

var ErrInvalidCSRFSecret = errors.New("CSRF secret must contain at least 32 bytes")

type CSRFProtector struct {
	secret []byte
}

func NewCSRFProtector(secret []byte) (CSRFProtector, error) {
	if len(secret) < minimumCSRFSecretBytes {
		return CSRFProtector{}, ErrInvalidCSRFSecret
	}
	return CSRFProtector{secret: append([]byte(nil), secret...)}, nil
}

func (protector CSRFProtector) Derive(rawToken []byte) string {
	mac := hmac.New(sha256.New, protector.secret)
	_, _ = mac.Write([]byte(csrfDomain))
	_, _ = mac.Write(rawToken)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (protector CSRFProtector) Valid(rawToken []byte, candidate string) bool {
	if len(rawToken) != tokenBytes || len(candidate) != encodedTokenBytes {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(candidate)
	if err != nil || len(decoded) != sha256.Size || base64.RawURLEncoding.EncodeToString(decoded) != candidate {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(protector.Derive(rawToken))
	return err == nil && subtle.ConstantTimeCompare(decoded, expected) == 1
}

func CookiePolicy(environment string) http.Cookie {
	// #nosec G124 -- development/test intentionally use HTTP on loopback; the
	// production branch below always enables Secure.
	cookie := http.Cookie{
		Name:     "synodus_dev_session",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false,
	}
	if environment == "production" {
		cookie.Name = "__Host-synodus_session"
		cookie.Secure = true
	}
	return cookie
}

func NormalizeUserAgent(userAgent string) string {
	if !utf8.ValidString(userAgent) {
		userAgent = strings.ToValidUTF8(userAgent, "")
	}
	normalized := norm.NFC.String(userAgent)
	if len(normalized) <= maximumUserAgentBytes {
		return normalized
	}
	end := maximumUserAgentBytes
	for end > 0 && !utf8.RuneStart(normalized[end]) {
		end--
	}
	return normalized[:end]
}
