package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubUserService struct {
	getUserssFn      func(context.Context) ([]service.User, error)
	getUserByIDFn    func(context.Context, string) (*service.User, error)
	getUserByEmailFn func(context.Context, string) (*service.User, error)
	createUserFn     func(context.Context, string, string, string) (*service.User, error)
	updateUserFn     func(context.Context, string, string, string, string) (*service.User, error)
	softDeleteUserFn func(context.Context, string) (*service.User, error)
	restoreUserFn    func(context.Context, string) (*service.User, error)
	loginFn          func(context.Context, string, string) (string, *service.User, error)
}

func (s *stubUserService) GetUsers(ctx context.Context) ([]service.User, error) {
	if s.getUserssFn == nil {
		panic("unexpected GetUsers call")
	}
	return s.getUserssFn(ctx)
}

func (s *stubUserService) GetUserByID(ctx context.Context, publicID string) (*service.User, error) {
	if s.getUserByIDFn == nil {
		panic("unexpected GetUserByID call")
	}
	return s.getUserByIDFn(ctx, publicID)
}

func (s *stubUserService) GetUserByEmail(ctx context.Context, email string) (*service.User, error) {
	if s.getUserByEmailFn == nil {
		panic("unexpected GetUserByEmail call")
	}
	return s.getUserByEmailFn(ctx, email)
}

func (s *stubUserService) CreateUser(ctx context.Context, name, email, password string) (*service.User, error) {
	if s.createUserFn == nil {
		panic("unexpected CreateUser call")
	}
	return s.createUserFn(ctx, name, email, password)
}

func (s *stubUserService) UpdateUser(ctx context.Context, publicID, name, email, password string) (*service.User, error) {
	if s.updateUserFn == nil {
		panic("unexpected UpdateUser call")
	}
	return s.updateUserFn(ctx, publicID, name, email, password)
}

func (s *stubUserService) SoftDeleteUser(ctx context.Context, publicID string) (*service.User, error) {
	if s.softDeleteUserFn == nil {
		panic("unexpected SoftDeleteUser call")
	}
	return s.softDeleteUserFn(ctx, publicID)
}

func (s *stubUserService) RestoreUser(ctx context.Context, publicID string) (*service.User, error) {
	if s.restoreUserFn == nil {
		panic("unexpected RestoreUser call")
	}
	return s.restoreUserFn(ctx, publicID)
}

func (s *stubUserService) Login(ctx context.Context, email, password string) (string, *service.User, error) {
	if s.loginFn == nil {
		panic("unexpected Login call")
	}
	return s.loginFn(ctx, email, password)
}

func performUserRequest(t *testing.T, userService service.UserService, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	r := chi.NewRouter()
	routerInstance := &Router{
		mux:         r,
		userService: userService,
	}

	r.Get("/users", routerInstance.getUsers())
	r.Post("/users", routerInstance.createUser())
	r.Post("/users/login", routerInstance.loginUser())
	r.Put("/users/{id}", routerInstance.updateUser())
	r.Delete("/users/{id}", routerInstance.deleteUser())

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if method == http.MethodPost || method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}

	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, req)
	return recorder
}

func testUser() service.User {
	return service.User{
		PublicID:  uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		Name:      "John Doe",
		Email:     "john@example.com",
		CreatedAt: time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, time.August, 15, 11, 0, 0, 0, time.UTC),
	}
}

func TestGetUsers(t *testing.T) {
	t.Run("returns all users", func(t *testing.T) {
		want := []service.User{testUser()}
		userService := &stubUserService{
			getUserssFn: func(ctx context.Context) ([]service.User, error) {
				require.NotNil(t, ctx)
				return want, nil
			},
		}

		response := performUserRequest(t, userService, http.MethodGet, "/users", "")

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		var got []service.User
		require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
		assert.Equal(t, want, got)
	})

	t.Run("returns service error", func(t *testing.T) {
		userService := &stubUserService{
			getUserssFn: func(context.Context) ([]service.User, error) {
				return nil, errors.New("database unavailable")
			},
		}

		response := performUserRequest(t, userService, http.MethodGet, "/users", "")

		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}

func TestCreateUser(t *testing.T) {
	t.Run("creates a user successfully", func(t *testing.T) {
		want := testUser()
		userService := &stubUserService{
			createUserFn: func(ctx context.Context, name, email, password string) (*service.User, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, want.Name, name)
				assert.Equal(t, want.Email, email)
				assert.Equal(t, "secret123", password)
				return &want, nil
			},
		}

		body := `{"name":"John Doe","email":"john@example.com","password":"secret123"}`
		response := performUserRequest(t, userService, http.MethodPost, "/users", body)

		assert.Equal(t, http.StatusCreated, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		var got service.User
		require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
		assert.Equal(t, want, got)
	})

	t.Run("rejects invalid JSON body", func(t *testing.T) {
		response := performUserRequest(t, &stubUserService{}, http.MethodPost, "/users", `{`)

		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("returns conflict when email already in use", func(t *testing.T) {
		userService := &stubUserService{
			createUserFn: func(ctx context.Context, name, email, password string) (*service.User, error) {
				return nil, service.ErrEmailAlreadyInUse
			},
		}

		body := `{"name":"John Doe","email":"john@example.com","password":"secret123"}`
		response := performUserRequest(t, userService, http.MethodPost, "/users", body)

		assert.Equal(t, http.StatusConflict, response.Code)
		assert.Contains(t, response.Body.String(), "Email already in use")
	})
}

func TestUpdateUser(t *testing.T) {
	testID := "11111111-2222-3333-4444-555555555555"

	t.Run("updates a user successfully", func(t *testing.T) {
		want := testUser()
		want.Name = "Jane Doe"
		want.Email = "jane@example.com"

		userService := &stubUserService{
			updateUserFn: func(ctx context.Context, publicID, name, email, password string) (*service.User, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, testID, publicID)
				assert.Equal(t, "Jane Doe", name)
				assert.Equal(t, "jane@example.com", email)
				return &want, nil
			},
		}

		// Ensure body matches the expected request struct in your handler
		body := `{"name":"Jane Doe","email":"jane@example.com","password":"newsecret"}`
		response := performUserRequest(t, userService, http.MethodPut, "/users/"+testID, body)

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		var got service.User
		require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
		assert.Equal(t, want, got)
	})
}

func TestDeleteUser(t *testing.T) {
	testID := "11111111-2222-3333-4444-555555555555"

	t.Run("soft deletes a user successfully", func(t *testing.T) {
		want := testUser()
		want.DeletedAt = time.Now()

		userService := &stubUserService{
			softDeleteUserFn: func(ctx context.Context, publicID string) (*service.User, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, testID, publicID)
				return &want, nil
			},
		}

		response := performUserRequest(t, userService, http.MethodDelete, "/users/"+testID, "")

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
	})
}

func TestLoginUser(t *testing.T) {
	t.Run("logins a user successfully", func(t *testing.T) {
		wantUser := testUser()
		wantToken := "mock-jwt-token"

		userService := &stubUserService{
			loginFn: func(ctx context.Context, email, password string) (string, *service.User, error) {
				require.NotNil(t, ctx)
				assert.Equal(t, "john@example.com", email)
				assert.Equal(t, "secret123", password)
				return wantToken, &wantUser, nil
			},
		}

		body := `{"email":"john@example.com","password":"secret123"}`
		response := performUserRequest(t, userService, http.MethodPost, "/users/login", body)

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))

		var got struct {
			Token string        `json:"token"`
			User  *service.User `json:"user"`
		}
		require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
		assert.Equal(t, wantToken, got.Token)
		assert.Equal(t, wantUser.Email, got.User.Email)
	})

	t.Run("returns unauthorized on invalid credentials", func(t *testing.T) {
		userService := &stubUserService{
			loginFn: func(ctx context.Context, email, password string) (string, *service.User, error) {
				return "", nil, service.ErrInvalidCredentials
			},
		}

		body := `{"email":"john@example.com","password":"wrong"}`
		response := performUserRequest(t, userService, http.MethodPost, "/users/login", body)

		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})
}
