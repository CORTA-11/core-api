package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	appMiddleware "github.com/CORTA-11/core-api/cmd/api/middleware"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Router struct {
	mux              *chi.Mux
	db               *pgxpool.Pool
	queries          *repository.Queries
	orgService       service.OrgService
	teamService      service.TeamService
	taskService      service.TaskService
	fileService      service.FileService
	userService      service.UserService
	tokenService     service.TokenService
	readinessChecks  map[string]ReadinessCheck
	readinessTimeout time.Duration
	pprofEnabled     bool
}

type RouterConf struct {
	DB               *pgxpool.Pool
	Queries          *repository.Queries
	OrgService       *service.OrgService
	TeamService      *service.TeamService
	TaskService      *service.TaskService
	FileService      *service.FileService
	UserService      *service.UserService
	TokenService     *service.TokenService
	ReadinessChecks  map[string]ReadinessCheck
	ReadinessTimeout time.Duration
	PprofEnabled     bool
}

func NewRouter(conf RouterConf) *Router {
	return &Router{
		mux:              chi.NewRouter(),
		db:               conf.DB,
		queries:          conf.Queries,
		orgService:       *conf.OrgService,
		teamService:      *conf.TeamService,
		taskService:      *conf.TaskService,
		fileService:      *conf.FileService,
		userService:      *conf.UserService,
		tokenService:     *conf.TokenService,
		readinessChecks:  conf.ReadinessChecks,
		readinessTimeout: conf.ReadinessTimeout,
		pprofEnabled:     conf.PprofEnabled,
	}
}

func (router *Router) SetupRoutes() {
	r := router.mux
	r.Use(httpx.RequestID)
	r.Use(middleware.Logger)
	r.Use(appMiddleware.Recoverer)
	r.Use(appMiddleware.CorsMiddleware)

	if router.pprofEnabled {
		r.Mount("/debug", middleware.Profiler())
	}

	r.Get("/health/live", healthLive())
	r.Get("/health/ready", healthReady(router.readinessChecks, router.readinessTimeout))

	r.Get("/", router.handleRoot())

	// Organization routes
	r.Mount("/orgs", orgRouter(router))

	// Team routes
	r.Mount("/teams", teamRouter(router))

	// Task routes
	r.Mount("/{team}/tasks", taskRouter(router))

	// File routes
	r.Mount("/{team}/files", fileRouter(router))

	r.Mount("/users", userRouter(router))

}

func orgRouter(router *Router) chi.Router {
	r := chi.NewRouter()

	r.Get("/", router.getOrgs())
	r.Post("/", router.createOrg())
	r.Put("/{orgID}", router.updateOrg())
	r.Delete("/{orgID}", router.deleteOrg())
	r.Post("/restore", router.restoreOrg())

	return r
}

func teamRouter(router *Router) chi.Router {
	r := chi.NewRouter()
	r.Use(appMiddleware.OrgMiddleware)

	r.Get("/", router.getTeams())
	r.Post("/", router.createTeam())

	return r
}

func taskRouter(router *Router) chi.Router {
	r := chi.NewRouter()
	r.Use(appMiddleware.OrgMiddleware)
	r.Use(appMiddleware.TeamMiddleware(router.teamService))

	r.Get("/", router.getTasks())
	r.Post("/", router.createTask())

	return r
}

func fileRouter(router *Router) chi.Router {
	r := chi.NewRouter()
	r.Use(appMiddleware.OrgMiddleware)
	r.Use(appMiddleware.TeamMiddleware(router.teamService))

	r.Get("/", router.getFiles())
	r.Get("/download/{filename}", router.downloadFile())
	r.Post("/upload", router.uploadFile())

	return r
}

func userRouter(router *Router) chi.Router {
	r := chi.NewRouter()

	r.Get("/", router.getUsers())
	r.Post("/", router.createUser())
	r.Post("/login", router.loginUser())
	r.Put("/{userID}", router.updateUser())
	r.Delete("/{userID}", router.deleteUser())

	return r
}

func (router *Router) Handler() http.Handler {
	return router.mux
}

func (router *Router) handleRoot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var response struct {
			Message string `json:"message"`
		}
		response.Message = "Hello World"

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			slog.ErrorContext(r.Context(), "failed to encode response", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
