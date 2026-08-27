// Package v1 contains the versioned HTTP handlers that remain dark until D06.
package v1

import (
	"errors"
	"net/http"
	"time"

	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const maximumAuthBodyBytes = 4 * 1024

type AuthHandler struct {
	manager       *session.Manager
	verifier      identity.CredentialVerifier
	hasher        identity.PasswordHasher
	cookie        http.Cookie
	allowedOrigin map[string]struct{}
}

func NewAuthRouter(
	manager *session.Manager,
	verifier identity.CredentialVerifier,
	hasher identity.PasswordHasher,
	environment string,
	allowedOrigins []string,
) http.Handler {
	handler := &AuthHandler{
		manager: manager, verifier: verifier, hasher: hasher,
		cookie:        session.CookiePolicy(environment),
		allowedOrigin: make(map[string]struct{}, len(allowedOrigins)),
	}
	for _, origin := range allowedOrigins {
		handler.allowedOrigin[origin] = struct{}{}
	}
	router := chi.NewRouter()
	router.Post("/api/v1/auth/login", handler.login)
	router.Get("/api/v1/auth/session", handler.authenticated(false, handler.current))
	router.Delete("/api/v1/auth/session", handler.logout)
	router.Get("/api/v1/auth/sessions", handler.authenticated(false, handler.list))
	router.Delete("/api/v1/auth/sessions", handler.authenticated(true, handler.revokeAll))
	router.Delete("/api/v1/auth/sessions/{session_id}", handler.authenticated(true, handler.revokeSpecific))
	router.Put("/api/v1/auth/password", handler.authenticated(true, handler.changePassword))
	return router
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type passwordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type sessionView struct {
	ID                uuid.UUID `json:"id"`
	CreatedAt         time.Time `json:"created_at"`
	LastSeenAt        time.Time `json:"last_seen_at"`
	IdleExpiresAt     time.Time `json:"idle_expires_at"`
	AbsoluteExpiresAt time.Time `json:"absolute_expires_at"`
	Current           bool      `json:"current"`
}

type authResponse struct {
	User      session.User `json:"user"`
	Session   sessionView  `json:"session"`
	CSRFToken string       `json:"csrf_token"`
}

func (handler *AuthHandler) login(writer http.ResponseWriter, request *http.Request) {
	var input loginRequest
	if err := httpx.DecodeJSON(request, &input, maximumAuthBodyBytes); err != nil {
		handler.problem(writer, request, http.StatusBadRequest, "Invalid request.")
		return
	}
	principal, err := handler.verifier.Verify(request.Context(), input.Email, input.Password)
	if err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) {
			handler.problem(writer, request, http.StatusUnauthorized, "Invalid credentials.")
			return
		}
		handler.problem(writer, request, http.StatusServiceUnavailable, "Authentication is unavailable.")
		return
	}
	oldToken := ""
	if cookie, cookieErr := request.Cookie(handler.cookie.Name); cookieErr == nil {
		oldToken = cookie.Value
	}
	issued, err := handler.manager.Rotate(request.Context(), principal.UserPublicID, oldToken, request.UserAgent())
	if err != nil {
		handler.problem(writer, request, http.StatusServiceUnavailable, "Authentication is unavailable.")
		return
	}
	handler.setCookie(writer, issued.RawToken)
	_ = httpx.WriteJSON(writer, http.StatusOK, responseFor(issued.Authentication, issued.CSRFToken))
}

type authenticatedHandler func(http.ResponseWriter, *http.Request, session.Authentication, string)

func (handler *AuthHandler) authenticated(unsafe bool, next authenticatedHandler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(handler.cookie.Name)
		if err != nil {
			handler.problem(writer, request, http.StatusUnauthorized, "Authentication required.")
			return
		}
		authentication, err := handler.manager.Authenticate(request.Context(), cookie.Value)
		if err != nil {
			handler.problem(writer, request, http.StatusUnauthorized, "Authentication required.")
			return
		}
		if unsafe && !handler.validUnsafe(request, authentication) {
			handler.problem(writer, request, http.StatusForbidden, "Request origin or CSRF token is invalid.")
			return
		}
		ctx := session.ContextWithPrincipal(request.Context(), authentication.Principal)
		next(writer, request.WithContext(ctx), authentication, cookie.Value)
	}
}

func (handler *AuthHandler) current(
	writer http.ResponseWriter,
	request *http.Request,
	authentication session.Authentication,
	_ string,
) {
	_ = httpx.WriteJSON(writer, http.StatusOK,
		responseFor(authentication, handler.manager.CSRFToken(authentication)))
}

func (handler *AuthHandler) logout(writer http.ResponseWriter, request *http.Request) {
	cookie, err := request.Cookie(handler.cookie.Name)
	if err != nil || !session.ParseableToken(cookie.Value) {
		handler.clearCookie(writer)
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if !handler.validOrigin(request) ||
		!handler.manager.ValidTokenCSRF(cookie.Value, request.Header.Get("X-CSRF-Token")) {
		handler.problem(writer, request, http.StatusForbidden, "Request origin or CSRF token is invalid.")
		return
	}
	if err := handler.manager.RevokeCurrent(request.Context(), cookie.Value); err != nil {
		handler.problem(writer, request, http.StatusServiceUnavailable, "Authentication is unavailable.")
		return
	}
	handler.clearCookie(writer)
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *AuthHandler) list(
	writer http.ResponseWriter,
	request *http.Request,
	authentication session.Authentication,
	_ string,
) {
	items, err := handler.manager.List(request.Context(), authentication.Principal)
	if err != nil {
		handler.problem(writer, request, http.StatusServiceUnavailable, "Session inspection is unavailable.")
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, struct {
		Sessions []session.Metadata `json:"sessions"`
	}{Sessions: items})
}

func (handler *AuthHandler) revokeAll(
	writer http.ResponseWriter,
	request *http.Request,
	authentication session.Authentication,
	_ string,
) {
	if err := handler.manager.RevokeAll(request.Context(), authentication.Principal); err != nil {
		handler.problem(writer, request, http.StatusServiceUnavailable, "Session revocation is unavailable.")
		return
	}
	handler.clearCookie(writer)
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *AuthHandler) revokeSpecific(
	writer http.ResponseWriter,
	request *http.Request,
	authentication session.Authentication,
	_ string,
) {
	sessionID, err := uuid.Parse(chi.URLParam(request, "session_id"))
	if err != nil {
		handler.problem(writer, request, http.StatusNotFound, "Session not found.")
		return
	}
	if err := handler.manager.Revoke(request.Context(), authentication.Principal, sessionID); err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			handler.problem(writer, request, http.StatusNotFound, "Session not found.")
			return
		}
		handler.problem(writer, request, http.StatusServiceUnavailable, "Session revocation is unavailable.")
		return
	}
	if sessionID == authentication.Principal.SessionID {
		handler.clearCookie(writer)
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *AuthHandler) changePassword(
	writer http.ResponseWriter,
	request *http.Request,
	authentication session.Authentication,
	_ string,
) {
	var input passwordRequest
	if err := httpx.DecodeJSON(request, &input, maximumAuthBodyBytes); err != nil {
		handler.problem(writer, request, http.StatusBadRequest, "Invalid request.")
		return
	}
	issued, err := handler.manager.ChangePassword(request.Context(), authentication,
		input.CurrentPassword, input.NewPassword, request.UserAgent(), handler.hasher)
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrInvalidCredentials):
			handler.problem(writer, request, http.StatusUnauthorized, "Invalid credentials.")
		case errors.Is(err, identity.ErrPasswordPolicy):
			handler.problem(writer, request, http.StatusBadRequest, "New password does not satisfy policy.")
		case errors.Is(err, session.ErrCredentialChanged):
			handler.problem(writer, request, http.StatusConflict, "Credentials changed concurrently.")
		default:
			handler.problem(writer, request, http.StatusServiceUnavailable, "Password change is unavailable.")
		}
		return
	}
	handler.setCookie(writer, issued.RawToken)
	_ = httpx.WriteJSON(writer, http.StatusOK, responseFor(issued.Authentication, issued.CSRFToken))
}

func (handler *AuthHandler) validUnsafe(request *http.Request, authentication session.Authentication) bool {
	return handler.validOrigin(request) &&
		handler.manager.ValidCSRF(authentication, request.Header.Get("X-CSRF-Token"))
}

func (handler *AuthHandler) validOrigin(request *http.Request) bool {
	_, ok := handler.allowedOrigin[request.Header.Get("Origin")]
	return ok && request.Header.Get("Origin") != ""
}

func (handler *AuthHandler) setCookie(writer http.ResponseWriter, token string) {
	cookie := handler.cookie
	cookie.Value = token
	http.SetCookie(writer, &cookie)
}

func (handler *AuthHandler) clearCookie(writer http.ResponseWriter) {
	cookie := handler.cookie
	cookie.Value = ""
	cookie.MaxAge = -1
	cookie.Expires = time.Unix(1, 0).UTC()
	http.SetCookie(writer, &cookie)
}

func (handler *AuthHandler) problem(writer http.ResponseWriter, request *http.Request, status int, detail string) {
	_ = httpx.WriteProblem(writer, request, &httpx.AppError{
		Status: status, Title: http.StatusText(status), Detail: detail,
	})
}

func responseFor(authentication session.Authentication, csrfToken string) authResponse {
	metadata := authentication.Session
	return authResponse{
		User: authentication.User,
		Session: sessionView{
			ID: metadata.ID, CreatedAt: metadata.CreatedAt, LastSeenAt: metadata.LastSeenAt,
			IdleExpiresAt: metadata.IdleExpiresAt, AbsoluteExpiresAt: metadata.AbsoluteExpiresAt,
			Current: metadata.Current,
		},
		CSRFToken: csrfToken,
	}
}
