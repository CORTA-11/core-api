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
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/ratelimit"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ReadinessCheck func(context.Context) error

type OrganizationService interface {
	List(context.Context, session.Principal, pagination.Parameters) (service.OrganizationPage, error)
	Create(context.Context, session.Principal, string) (service.OrganizationView, error)
	Get(context.Context, session.Principal, uuid.UUID) (service.OrganizationView, error)
	Update(context.Context, session.Principal, uuid.UUID, string) (service.OrganizationView, error)
	Delete(context.Context, session.Principal, uuid.UUID) error
	Restore(context.Context, session.Principal, uuid.UUID) (service.OrganizationView, error)
}

type OrganizationMemberService interface {
	ListMembers(context.Context, session.Principal, uuid.UUID) ([]service.OrganizationMemberView, error)
}

type TeamTaskService interface {
	ListTeams(context.Context, session.Principal, uuid.UUID, pagination.Parameters) (service.TeamPage, error)
	CreateTeam(context.Context, session.Principal, uuid.UUID, string) (service.TeamView, error)
	ListTasks(context.Context, session.Principal, uuid.UUID, uuid.UUID, pagination.Parameters) (service.TaskPage, error)
	CreateTask(context.Context, session.Principal, uuid.UUID, uuid.UUID, string, string) (service.TaskView, error)
	UpdateTask(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID, string, string) (service.TaskView, error)
	DeleteTask(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID) error
	ListTeamMembers(context.Context, session.Principal, uuid.UUID, uuid.UUID) ([]service.TeamMemberView, error)
	AddTeamMember(context.Context, session.Principal, uuid.UUID, uuid.UUID, string) (service.TeamMemberView, error)
}

type InvitationService interface {
	List(context.Context, session.Principal, uuid.UUID) ([]service.InvitationView, error)
	Create(context.Context, session.Principal, uuid.UUID, string) (service.InvitationCreatedView, error)
	Revoke(context.Context, session.Principal, uuid.UUID, uuid.UUID) error
	Preview(context.Context, string) (service.InvitationPreview, error)
	Consume(context.Context, session.Principal, string, bool) error
}

type RouterConfig struct {
	Manager             *session.Manager
	Verifier            identity.CredentialVerifier
	Hasher              identity.PasswordHasher
	Organizations       OrganizationService
	OrganizationMembers OrganizationMemberService
	TeamTasks           TeamTaskService
	Invitations         InvitationService
	Environment         string
	Origins             httpx.OriginPolicy
	TrustedProxies      httpx.TrustedProxies
	Logger              *slog.Logger
	LoginGuard          *ratelimit.LoginGuard
	RegistrationGuard   *ratelimit.RegistrationGuard
	Administrative      func(http.Handler) http.Handler
	ReadinessChecks     map[string]ReadinessCheck
	ReadinessTimeout    time.Duration
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
	}
	for _, origin := range config.Origins.Values() {
		auth.allowedOrigin[origin] = struct{}{}
	}
	router := &Router{mux: chi.NewRouter(), config: config, auth: auth}
	router.resources = &ResourceHandler{organizations: config.Organizations, organizationMembers: config.OrganizationMembers, teamTasks: config.TeamTasks, invitations: config.Invitations}
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
	switch operationID {
	case "register":
		return http.HandlerFunc(router.auth.register)
	case "login":
		return http.HandlerFunc(router.auth.login)
	case "getCurrentSession":
		return http.HandlerFunc(router.auth.authenticated(false, router.auth.current))
	case "logout":
		return http.HandlerFunc(router.auth.logout)
	case "listSessions":
		return http.HandlerFunc(router.auth.authenticated(false, router.auth.list))
	case "revokeAllSessions":
		return http.HandlerFunc(router.auth.authenticated(true, router.auth.revokeAll))
	case "revokeSession":
		return http.HandlerFunc(router.auth.authenticated(true, router.auth.revokeSpecific))
	case "changePassword":
		return http.HandlerFunc(router.auth.authenticated(true, router.auth.changePassword))
	case "listOrganizations":
		return http.HandlerFunc(router.resources.listOrganizations)
	case "createOrganization":
		return http.HandlerFunc(router.resources.createOrganization)
	case "getOrganization":
		return http.HandlerFunc(router.resources.getOrganization)
	case "updateOrganization":
		return http.HandlerFunc(router.resources.updateOrganization)
	case "deleteOrganization":
		return http.HandlerFunc(router.resources.deleteOrganization)
	case "restoreOrganization":
		return http.HandlerFunc(router.resources.restoreOrganization)
	case "listTeams":
		return http.HandlerFunc(router.resources.listTeams)
	case "createTeam":
		return http.HandlerFunc(router.resources.createTeam)
	case "listTasks":
		return http.HandlerFunc(router.resources.listTasks)
	case "createTask":
		return http.HandlerFunc(router.resources.createTask)
	case "updateTask":
		return http.HandlerFunc(router.resources.updateTask)
	case "deleteTask":
		return http.HandlerFunc(router.resources.deleteTask)
	case "listOrganizationInvitations":
		return http.HandlerFunc(router.resources.listInvitations)
	case "listOrganizationMembers":
		return http.HandlerFunc(router.resources.listOrganizationMembers)
	case "createOrganizationInvitation":
		return http.HandlerFunc(router.resources.createInvitation)
	case "revokeOrganizationInvitation":
		return http.HandlerFunc(router.resources.revokeInvitation)
	case "getCurrentOrganizationInvitation":
		return http.HandlerFunc(router.resources.previewInvitation)
	case "acceptCurrentOrganizationInvitation":
		return http.HandlerFunc(router.resources.acceptInvitation)
	case "declineCurrentOrganizationInvitation":
		return http.HandlerFunc(router.resources.declineInvitation)
	case "listTeamMembers":
		return http.HandlerFunc(router.resources.listTeamMembers)
	case "addTeamMember":
		return http.HandlerFunc(router.resources.addTeamMember)
	default:
		return problemHandler(httpx.ProblemInternalFailure)
	}
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

func isResourceOperation(operationID string) bool {
	return operationID == "listOrganizations" || operationID == "createOrganization" ||
		operationID == "getOrganization" || operationID == "updateOrganization" ||
		operationID == "deleteOrganization" || operationID == "restoreOrganization" ||
		operationID == "listTeams" || operationID == "createTeam" || operationID == "listTasks" ||
		operationID == "createTask" || operationID == "updateTask" || operationID == "deleteTask" ||
		operationID == "listOrganizationInvitations" || operationID == "createOrganizationInvitation" ||
		operationID == "listOrganizationMembers" ||
		operationID == "revokeOrganizationInvitation" || operationID == "acceptCurrentOrganizationInvitation" ||
		operationID == "declineCurrentOrganizationInvitation" ||
		operationID == "listTeamMembers" || operationID == "addTeamMember"
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
