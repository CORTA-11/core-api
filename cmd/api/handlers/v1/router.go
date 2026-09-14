package v1

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/CORTA-11/core-api/internal/apicontract"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/ratelimit"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/go-chi/chi/v5"
)

type ReadinessCheck func(context.Context) error

type RouterConfig struct {
	Manager                    *session.Manager
	Verifier                   identity.CredentialVerifier
	Hasher                     identity.PasswordHasher
	Organizations              service.OrganizationService
	OrganizationMembers        service.OrganizationMemberService
	TeamTasks                  service.TeamTaskService
	Documents                  service.DocumentService
	Invitations                service.InvitationService
	ResourceBookings           service.ResourceBookingService
	Keys                       service.KeyService
	KeyAccess                  service.KeyAccessService
	Files                      service.FileService
	Chat                       service.ChatService
	Environment                string
	Origins                    httpx.OriginPolicy
	TrustedProxies             httpx.TrustedProxies
	Logger                     *slog.Logger
	LoginGuard                 *ratelimit.LoginGuard
	RegistrationGuard          *ratelimit.RegistrationGuard
	Administrative             func(http.Handler) http.Handler
	ReadinessChecks            map[string]ReadinessCheck
	ReadinessTimeout           time.Duration
	CollaborationServiceSecret []byte
}

type Router struct {
	mux       *chi.Mux
	config    RouterConfig
	auth      *AuthHandler
	resources *ResourceHandler
}

func NewRouter(config RouterConfig) *Router {
	if config.ReadinessTimeout <= 0 {
		config.ReadinessTimeout = 3 * time.Second
	}
	auth := &AuthHandler{
		manager: config.Manager, verifier: config.Verifier, hasher: config.Hasher,
		cookie: session.CookiePolicy(config.Environment), allowedOrigin: map[string]struct{}{},
		loginGuard:    config.LoginGuard,
		registerGuard: config.RegistrationGuard,
		keys:          config.Keys,
	}
	for _, origin := range config.Origins.Values() {
		auth.allowedOrigin[origin] = struct{}{}
	}
	router := &Router{mux: chi.NewRouter(), config: config, auth: auth}
	router.resources = &ResourceHandler{
		organizations:              config.Organizations,
		organizationMembers:        config.OrganizationMembers,
		teamTasks:                  config.TeamTasks,
		documents:                  config.Documents,
		invitations:                config.Invitations,
		resourceBookings:           config.ResourceBookings,
		keys:                       config.Keys,
		keyAccess:                  config.KeyAccess,
		files:                      config.Files,
		chat:                       config.Chat,
		collaborationServiceSecret: append([]byte(nil), config.CollaborationServiceSecret...),
	}
	router.compose()
	return router
}

func (router *Router) Handler() http.Handler { return router.mux }

func (router *Router) compose() {
	router.mux.Use(httpx.RequestID)
	router.mux.Use(router.config.TrustedProxies.Middleware)
	router.mux.Use(httpx.Recover)
	router.mux.Use(func(next http.Handler) http.Handler { return httpx.BoundaryLog(router.config.Logger, next) })
	router.mux.Use(func(next http.Handler) http.Handler {
		return httpx.SecurityHeaders(router.config.Environment, true, next)
	})
	router.mux.Use(func(next http.Handler) http.Handler { return httpx.CORS(router.config.Origins, next) })
	router.mux.NotFound(problemHandler(httpx.ProblemNotFound))
	router.mux.MethodNotAllowed(problemHandler(httpx.ProblemNotFound))
	router.mux.Get("/health/live", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	router.mux.Get("/health/ready", router.ready)
	for _, route := range apicontract.Routes {
		handler := router.operation(route.OperationID)
		handler = httpx.LimitBody(route.BodyLimit, handler)
		if route.RateLimit == apicontract.RateAdministrative {
			if router.config.Administrative == nil {
				handler = problemHandler(httpx.ProblemDependencyUnavailable)
			} else {
				handler = router.config.Administrative(handler)
			}
		}
		if isResourceOperation(route.OperationID) {
			handler = router.authenticate(route.CSRF == apicontract.CSRFRequired, handler)
		}
		router.mux.Method(route.Method, route.Pattern, handler)
	}
}

func (router *Router) operation(operationID string) http.Handler {
	// It is of utmost importance that the 's' in 'operations' must be lowercase
	operationsLUT := map[string]http.HandlerFunc{
		"register": router.auth.register,
		"login":    router.auth.login,

		"getCurrentSession": router.auth.authenticated(false, router.auth.current),
		"logout":            router.auth.logout,
		"listSessions":      router.auth.authenticated(false, router.auth.list),
		"revokeAllSessions": router.auth.authenticated(true, router.auth.revokeAll),
		"revokeSession":     router.auth.authenticated(true, router.auth.revokeSpecific),
		"changePassword":    router.auth.authenticated(true, router.auth.changePassword),

		"listOrganizations":   router.resources.listOrganizations,
		"createOrganization":  router.resources.createOrganization,
		"getOrganization":     router.resources.getOrganization,
		"updateOrganization":  router.resources.updateOrganization,
		"deleteOrganization":  router.resources.deleteOrganization,
		"restoreOrganization": router.resources.restoreOrganization,

		"listTeams":  router.resources.listTeams,
		"createTeam": router.resources.createTeam,
		"listTasks":  router.resources.listTasks,
		"createTask": router.resources.createTask,
		"updateTask": router.resources.updateTask,
		"deleteTask": router.resources.deleteTask,

		"listOrganizationInvitations":          router.resources.listInvitations,
		"listOrganizationMembers":              router.resources.listOrganizationMembers,
		"createOrganizationInvitation":         router.resources.createInvitation,
		"revokeOrganizationInvitation":         router.resources.revokeInvitation,
		"getCurrentOrganizationInvitation":     router.resources.previewInvitation,
		"acceptCurrentOrganizationInvitation":  router.resources.acceptInvitation,
		"declineCurrentOrganizationInvitation": router.resources.declineInvitation,

		"listTeamMembers": router.resources.listTeamMembers,
		"addTeamMember":   router.resources.addTeamMember,

		"listResources":         router.resources.listResources,
		"createResource":        router.resources.createResource,
		"updateResource":        router.resources.updateResource,
		"deleteResource":        router.resources.deleteResource,
		"listBookings":          router.resources.listBookings,
		"createResourceRequest": router.resources.createResourceRequest,
		"listResourceRequests":  router.resources.listResourceRequests,
		"decideResourceRequest": router.resources.decideResourceRequest,

		"upsertUserKeys":       router.auth.upsertUserKeys,
		"getUserKeys":          router.auth.getUserKeys,
		"getPublicKeysForTeam": router.resources.getPublicKeysForTeam,

		"createTeamKey":        router.resources.createTeamKey,
		"listTeamKeys":         router.resources.listTeamKeys,
		"addTeamKeyMemberWrap": router.resources.addTeamKeyMemberWrap,

		"createKeyAccessRequest":  router.resources.createKeyAccessRequest,
		"listKeyAccessRequests":   router.resources.listKeyAccessRequests,
		"approveKeyAccessRequest": router.resources.approveKeyAccessRequest,
		"denyKeyAccessRequest":    router.resources.denyKeyAccessRequest,

		"uploadFile":   router.resources.uploadFile,
		"listFiles":    router.resources.listFiles,
		"downloadFile": router.resources.downloadFile,
		"deleteFile":   router.resources.deleteFile,

		"listChatMessages":      router.resources.listChatMessages,
		"createChatMessage":     router.resources.createChatMessage,
		"deleteChatMessage":     router.resources.deleteChatMessage,
		"issueChatSocketTicket": router.resources.issueChatSocketTicket,

		"listDocuments":             router.resources.listDocuments,
		"createDocument":            router.resources.createDocument,
		"getDocument":               router.resources.getDocument,
		"updateDocument":            router.resources.updateDocument,
		"deleteDocument":            router.resources.deleteDocument,
		"issueDocumentSocketTicket": router.resources.issueDocumentSocketTicket,
		"loadDocumentState":         router.resources.loadDocumentState,
		"storeDocumentState":        router.resources.storeDocumentState,
	}

	if handler, ok := operationsLUT[operationID]; ok {
		return handler
	}

	return problemHandler(httpx.ProblemInternalFailure)
}

type authenticationContextKey struct{}

func (router *Router) authenticate(unsafe bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if router.config.Manager == nil {
			writeProblem(writer, request, httpx.ProblemDependencyUnavailable, nil)
			return
		}
		cookie, err := request.Cookie(session.CookiePolicy(router.config.Environment).Name)
		if err != nil {
			writeProblem(writer, request, httpx.ProblemUnauthenticated, err)
			return
		}
		authentication, err := router.config.Manager.Authenticate(request.Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, session.ErrSessionDependency) {
				writeProblem(writer, request, httpx.ProblemDependencyUnavailable, err)
			} else {
				writeProblem(writer, request, httpx.ProblemUnauthenticated, err)
			}
			return
		}
		if unsafe && (!router.config.Origins.Allows(request.Header.Get("Origin")) ||
			!router.config.Manager.ValidCSRF(authentication, request.Header.Get("X-CSRF-Token"))) {
			writeProblem(writer, request, httpx.ProblemForbidden, nil)
			return
		}
		ctx := session.ContextWithPrincipal(request.Context(), authentication.Principal)
		ctx = context.WithValue(ctx, authenticationContextKey{}, authentication)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func authenticationFrom(request *http.Request) (session.Authentication, bool) {
	authentication, ok := request.Context().Value(authenticationContextKey{}).(session.Authentication)
	return authentication, ok
}

var resourceOperationIDs = map[string]struct{}{
	"listOrganizations":                    {},
	"createOrganization":                   {},
	"getOrganization":                      {},
	"updateOrganization":                   {},
	"deleteOrganization":                   {},
	"restoreOrganization":                  {},
	"listTeams":                            {},
	"createTeam":                           {},
	"listTasks":                            {},
	"createTask":                           {},
	"updateTask":                           {},
	"deleteTask":                           {},
	"listOrganizationInvitations":          {},
	"createOrganizationInvitation":         {},
	"listOrganizationMembers":              {},
	"revokeOrganizationInvitation":         {},
	"acceptCurrentOrganizationInvitation":  {},
	"declineCurrentOrganizationInvitation": {},
	"listTeamMembers":                      {},
	"addTeamMember":                        {},
	"listResources":                        {},
	"createResource":                       {},
	"updateResource":                       {},
	"deleteResource":                       {},
	"listBookings":                         {},
	"createResourceRequest":                {},
	"listResourceRequests":                 {},
	"decideResourceRequest":                {},
	"upsertUserKeys":                       {},
	"getUserKeys":                          {},
	"getPublicKeysForTeam":                 {},
	"createTeamKey":                        {},
	"listTeamKeys":                         {},
	"addTeamKeyMemberWrap":                 {},
	"createKeyAccessRequest":               {},
	"listKeyAccessRequests":                {},
	"approveKeyAccessRequest":              {},
	"denyKeyAccessRequest":                 {},
	"uploadFile":                           {},
	"listFiles":                            {},
	"downloadFile":                         {},
	"deleteFile":                           {},
	"listChatMessages":                     {},
	"createChatMessage":                    {},
	"deleteChatMessage":                    {},
	"issueChatSocketTicket":                {},
	"listDocuments":                        {},
	"createDocument":                       {},
	"getDocument":                          {},
	"updateDocument":                       {},
	"deleteDocument":                       {},
	"issueDocumentSocketTicket":            {},
}

func isResourceOperation(operationID string) bool {
	_, exists := resourceOperationIDs[operationID]
	return exists
}

func (router *Router) ready(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), router.config.ReadinessTimeout)
	defer cancel()
	failed := make([]string, 0)
	var mutex sync.Mutex
	var group sync.WaitGroup
	for name, check := range router.config.ReadinessChecks {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := check(ctx); err != nil {
				mutex.Lock()
				failed = append(failed, name)
				mutex.Unlock()
			}
		}()
	}
	group.Wait()
	if len(failed) == 0 {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	sort.Strings(failed)
	_ = httpx.WriteJSON(writer, http.StatusServiceUnavailable, struct {
		Failed []string `json:"failed"`
	}{Failed: failed})
}

func problemHandler(kind httpx.ProblemKind) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) { writeProblem(writer, request, kind, nil) }
}

func writeProblem(writer http.ResponseWriter, request *http.Request, kind httpx.ProblemKind, cause error) {
	_ = httpx.WriteProblem(writer, request, httpx.NewError(kind, cause))
}
