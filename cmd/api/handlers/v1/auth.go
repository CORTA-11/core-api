// Package v1 contains the versioned HTTP handlers that remain dark until D06.
package v1

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/ratelimit"
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
	loginGuard    *ratelimit.LoginGuard
	registerGuard *ratelimit.RegistrationGuard
}

func NewAuthRouter(
	manager *session.Manager,
	verifier identity.CredentialVerifier,
	hasher identity.PasswordHasher,
	environment string,
	allowedOrigins []string,
) http.Handler {
	return NewRateLimitedAuthRouter(manager, verifier, hasher, environment, allowedOrigins, nil)
}

func NewRateLimitedAuthRouter(
	manager *session.Manager,
	verifier identity.CredentialVerifier,
	hasher identity.PasswordHasher,
	environment string,
	allowedOrigins []string,
	loginGuard *ratelimit.LoginGuard,
) http.Handler {
	handler := &AuthHandler{
		manager: manager, verifier: verifier, hasher: hasher,
		cookie:        session.CookiePolicy(environment),
		allowedOrigin: make(map[string]struct{}, len(allowedOrigins)),
		loginGuard:    loginGuard,
	}
	for _, origin := range allowedOrigins {
		handler.allowedOrigin[origin] = struct{}{}
	}
	router := chi.NewRouter()
	router.Use(httpx.RequestID)
	router.Use(httpx.Recover)
	router.NotFound(func(writer http.ResponseWriter, request *http.Request) {
		_ = httpx.WriteProblem(writer, request, httpx.NewError(httpx.ProblemNotFound, nil))
	})
	router.MethodNotAllowed(func(writer http.ResponseWriter, request *http.Request) {
		_ = httpx.WriteProblem(writer, request, httpx.NewError(httpx.ProblemNotFound, nil))
	})
	router.Post("/api/v1/auth/login", handler.login)
	router.Post("/api/v1/auth/register", handler.register)
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

type registerRequest struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
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

func (handler *AuthHandler) register(writer http.ResponseWriter, request *http.Request) {
	if handler.registerGuard != nil {
		client, ok := httpx.ClientFromContext(request.Context())
		if !ok {
			handler.rateProblem(writer, request, ratelimit.Decision{}, errors.New("trusted client unavailable"))
			return
		}
		decision, err := handler.registerGuard.Admit(request.Context(), client.Address)
		if err != nil || !decision.Allowed {
			handler.rateProblem(writer, request, decision, err)
			return
		}
	}
	var input registerRequest
	if err := httpx.DecodeJSON(request, &input, maximumAuthBodyBytes); err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	canonical, emailErr := (identity.EmailCanonicalizer{}).Canonicalize(input.Email)
	displayName, displayErr := identity.NormalizeDisplayName(input.DisplayName)
	password, passwordErr := (identity.PasswordPolicy{}).Normalize(input.Password)
	violations := make([]httpx.Violation, 0, 3)
	if emailErr != nil {
		violations = append(violations, httpx.Violation{Field: "email", Code: "invalid", Message: "Enter a valid email address."})
	}
	if displayErr != nil {
		violations = append(violations, httpx.Violation{Field: "display_name", Code: "invalid", Message: "Display name must contain 1 to 100 characters."})
	}
	if passwordErr != nil {
		violations = append(violations, httpx.Violation{Field: "password", Code: "invalid", Message: "Password must contain 15 to 128 characters."})
	}
	if len(violations) != 0 {
		_ = httpx.WriteProblem(writer, request, httpx.NewError(httpx.ProblemInvalidRequest, nil, violations...))
		return
	}
	if handler.hasher == nil || handler.manager == nil {
		handler.problem(writer, request, httpx.ProblemDependencyUnavailable, errors.New("registration dependency unavailable"))
		return
	}
	passwordHash, err := handler.hasher.Hash(request.Context(), password)
	if err != nil {
		handler.problem(writer, request, httpx.ProblemDependencyUnavailable, err)
		return
	}
	oldToken := ""
	if cookie, cookieErr := request.Cookie(handler.cookie.Name); cookieErr == nil {
		oldToken = cookie.Value
	}
	issued, err := handler.manager.Register(request.Context(), canonical.Display, displayName, passwordHash,
		identity.PasswordNormalizationNFCV1, oldToken, request.UserAgent())
	if err != nil {
		if errors.Is(err, session.ErrEmailAlreadyExists) {
			_ = httpx.WriteProblem(writer, request, httpx.NewError(httpx.ProblemConflict, err,
				httpx.Violation{Field: "email", Code: "already_exists", Message: "An account with this email already exists."}))
			return
		}
		handler.problem(writer, request, httpx.ProblemDependencyUnavailable, err)
		return
	}
	handler.setCookie(writer, issued.RawToken)
	_ = httpx.WriteJSON(writer, http.StatusCreated, responseFor(issued.Authentication, issued.CSRFToken))
}

func (handler *AuthHandler) login(writer http.ResponseWriter, request *http.Request) {
	if handler.loginGuard != nil {
		client, ok := httpx.ClientFromContext(request.Context())
		if !ok {
			handler.rateProblem(writer, request, ratelimit.Decision{}, errors.New("trusted client unavailable"))
			return
		}
		decision, err := handler.loginGuard.AdmitIP(request.Context(), client.Address)
		if err != nil || !decision.Allowed {
			handler.rateProblem(writer, request, decision, err)
			return
		}
	}
	var input loginRequest
	if err := httpx.DecodeJSON(request, &input, maximumAuthBodyBytes); err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	var account ratelimit.AccountIdentity
	var err error
	if handler.loginGuard != nil {
		var decision ratelimit.Decision
		account, decision, err = handler.loginGuard.AdmitAccount(request.Context(), input.Email)
		if err != nil || !decision.Allowed {
			handler.rateProblem(writer, request, decision, err)
			return
		}
	}
	principal, err := handler.verifier.Verify(request.Context(), input.Email, input.Password)
	if err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) {
			if handler.loginGuard != nil {
				decision, limitErr := handler.loginGuard.RecordFailure(request.Context(), account)
				if limitErr != nil || !decision.Allowed {
					handler.rateProblem(writer, request, decision, limitErr)
					return
				}
			}
			handler.problem(writer, request, httpx.ProblemUnauthenticated, err)
			return
		}
		handler.problem(writer, request, httpx.ProblemDependencyUnavailable, err)
		return
	}
	if handler.loginGuard != nil {
		if err := handler.loginGuard.ClearSuccess(request.Context(), account); err != nil {
			handler.rateProblem(writer, request, ratelimit.Decision{}, err)
			return
		}
	}
	oldToken := ""
	if cookie, cookieErr := request.Cookie(handler.cookie.Name); cookieErr == nil {
		oldToken = cookie.Value
	}
	issued, err := handler.manager.Rotate(request.Context(), principal.UserPublicID, oldToken, request.UserAgent())
	if err != nil {
		handler.problem(writer, request, httpx.ProblemDependencyUnavailable, err)
		return
	}
	handler.setCookie(writer, issued.RawToken)
	_ = httpx.WriteJSON(writer, http.StatusOK, responseFor(issued.Authentication, issued.CSRFToken))
}

func (handler *AuthHandler) rateProblem(writer http.ResponseWriter, request *http.Request, decision ratelimit.Decision, err error) {
	if err != nil {
		handler.problem(writer, request, httpx.ProblemDependencyUnavailable, err)
		return
	}
	retrySeconds := max(int64((decision.RetryAfter+time.Second-1)/time.Second), 1)
	writer.Header().Set("Retry-After", strconv.FormatInt(retrySeconds, 10))
	handler.problem(writer, request, httpx.ProblemRateLimited, nil)
}

type authenticatedHandler func(http.ResponseWriter, *http.Request, session.Authentication, string)

func (handler *AuthHandler) authenticated(unsafe bool, next authenticatedHandler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(handler.cookie.Name)
		if err != nil {
			handler.problem(writer, request, httpx.ProblemUnauthenticated, err)
			return
		}
		authentication, err := handler.manager.Authenticate(request.Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, session.ErrSessionDependency) {
				handler.problem(writer, request, httpx.ProblemDependencyUnavailable, err)
			} else {
				handler.problem(writer, request, httpx.ProblemUnauthenticated, err)
			}
			return
		}
		if unsafe && !handler.validUnsafe(request, authentication) {
			handler.problem(writer, request, httpx.ProblemForbidden, nil)
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
		handler.problem(writer, request, httpx.ProblemForbidden, nil)
		return
	}
	if err := handler.manager.RevokeCurrent(request.Context(), cookie.Value); err != nil {
		handler.problem(writer, request, httpx.ProblemDependencyUnavailable, err)
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
		handler.problem(writer, request, httpx.ProblemDependencyUnavailable, err)
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
		handler.problem(writer, request, httpx.ProblemDependencyUnavailable, err)
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
		handler.problem(writer, request, httpx.ProblemNotFound, err)
		return
	}
	if err := handler.manager.Revoke(request.Context(), authentication.Principal, sessionID); err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			handler.problem(writer, request, httpx.ProblemNotFound, err)
			return
		}
		handler.problem(writer, request, httpx.ProblemDependencyUnavailable, err)
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
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	issued, err := handler.manager.ChangePassword(request.Context(), authentication,
		input.CurrentPassword, input.NewPassword, request.UserAgent(), handler.hasher)
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrInvalidCredentials):
			handler.problem(writer, request, httpx.ProblemUnauthenticated, err)
		case errors.Is(err, identity.ErrPasswordPolicy):
			handler.problem(writer, request, httpx.ProblemInvalidRequest, err)
		case errors.Is(err, session.ErrCredentialChanged):
			handler.problem(writer, request, httpx.ProblemConflict, err)
		default:
			handler.problem(writer, request, httpx.ProblemDependencyUnavailable, err)
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
	// #nosec G124 -- handler.cookie is the complete environment-derived policy.
	cookie := handler.cookie
	cookie.Value = token
	http.SetCookie(writer, &cookie)
}

func (handler *AuthHandler) clearCookie(writer http.ResponseWriter) {
	// #nosec G124 -- clearing reuses the exact complete issuance policy.
	cookie := handler.cookie
	cookie.Value = ""
	cookie.MaxAge = -1
	cookie.Expires = time.Unix(1, 0).UTC()
	http.SetCookie(writer, &cookie)
}

func (handler *AuthHandler) problem(writer http.ResponseWriter, request *http.Request, kind httpx.ProblemKind, cause error) {
	_ = httpx.WriteProblem(writer, request, httpx.NewError(kind, cause))
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
