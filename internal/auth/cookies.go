package auth

import (
	"net/http"
	"strings"
	"time"
)

const RefreshCookieName = "corta_refresh"

// SetRefreshCookie stores the refresh token in an httpOnly cookie.
// Path is "/" so it works through the Next.js /api proxy in local development.
// Secure is always true (browsers allow Secure cookies on http://localhost).
func SetRefreshCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
		Expires:  time.Now().Add(ttl),
	})
}

// ClearRefreshCookie expires the refresh cookie.
func ClearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

// RefreshTokenFromRequest reads the refresh token from cookie, falling back to JSON body field.
func RefreshTokenFromRequest(r *http.Request, bodyToken string) string {
	if cookie, err := r.Cookie(RefreshCookieName); err == nil {
		if token := strings.TrimSpace(cookie.Value); token != "" {
			return token
		}
	}
	return strings.TrimSpace(bodyToken)
}
