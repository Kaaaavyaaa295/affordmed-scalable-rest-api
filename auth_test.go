package unit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/yourusername/affordmed-api/internal/auth"
	"github.com/yourusername/affordmed-api/internal/handlers"
	"github.com/yourusername/affordmed-api/internal/models"
)

func init() { gin.SetMode(gin.TestMode) }

// ─── Mocks ────────────────────────────────────────────────────────────────────

type MockUserRepo struct{ mock.Mock }

func (m *MockUserRepo) Create(ctx context.Context, u *models.User) error {
	args := m.Called(ctx, u)
	u.ID = "test-uuid-1234"
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()
	return args.Error(0)
}
func (m *MockUserRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil { return nil, args.Error(1) }
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *MockUserRepo) FindByID(ctx context.Context, id string) (*models.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil { return nil, args.Error(1) }
	return args.Get(0).(*models.User), args.Error(1)
}

type MockSessionCache struct{ mock.Mock }

func (m *MockSessionCache) SetRefreshToken(ctx context.Context, userID, token string) error {
	return m.Called(ctx, userID, token).Error(0)
}
func (m *MockSessionCache) GetRefreshToken(ctx context.Context, userID string) (string, error) {
	args := m.Called(ctx, userID)
	return args.String(0), args.Error(1)
}
func (m *MockSessionCache) DeleteRefreshToken(ctx context.Context, userID string) error {
	return m.Called(ctx, userID).Error(0)
}
func (m *MockSessionCache) BlacklistToken(ctx context.Context, token string, ttl time.Duration) error {
	return m.Called(ctx, token, ttl).Error(0)
}
func (m *MockSessionCache) IsBlacklisted(ctx context.Context, token string) (bool, error) {
	args := m.Called(ctx, token)
	return args.Bool(0), args.Error(1)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func setupAuthRouter(h *handlers.AuthHandler) *gin.Engine {
	r := gin.New()
	r.POST("/auth/register", h.Register)
	r.POST("/auth/login", h.Login)
	r.POST("/auth/refresh", h.RefreshToken)
	return r
}

func postJSON(router *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestLoginHandler_ValidCredentials(t *testing.T) {
	// Arrange
	// Pre-hash for password "password123" – generated once, stored here for speed
	hash := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

	userRepo := new(MockUserRepo)
	sessionCache := new(MockSessionCache)
	jwtSvc := auth.NewJWTService("test-secret-key-32-bytes-minimum!")

	userRepo.On("FindByEmail", mock.Anything, "user@test.com").Return(&models.User{
		ID: "uuid-1", Email: "user@test.com", PasswordHash: hash, Role: "user",
	}, nil)
	sessionCache.On("SetRefreshToken", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	h := handlers.NewAuthHandler(userRepo, jwtSvc, sessionCache)
	router := setupAuthRouter(h)

	// Act
	w := postJSON(router, "/auth/login", map[string]string{
		"email": "user@test.com", "password": "password123",
	})

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotEmpty(t, resp["token"])
	assert.NotEmpty(t, resp["refresh_token"])
	assert.Equal(t, float64(3600), resp["expires_in"])
	userRepo.AssertExpectations(t)
}

func TestLoginHandler_InvalidPassword(t *testing.T) {
	hash := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

	userRepo := new(MockUserRepo)
	sessionCache := new(MockSessionCache)
	jwtSvc := auth.NewJWTService("test-secret-key-32-bytes-minimum!")

	userRepo.On("FindByEmail", mock.Anything, "user@test.com").Return(&models.User{
		ID: "uuid-1", Email: "user@test.com", PasswordHash: hash, Role: "user",
	}, nil)

	h := handlers.NewAuthHandler(userRepo, jwtSvc, sessionCache)
	router := setupAuthRouter(h)

	w := postJSON(router, "/auth/login", map[string]string{
		"email": "user@test.com", "password": "wrongpassword",
	})

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "invalid credentials", resp["error"])
}

func TestLoginHandler_UserNotFound(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionCache := new(MockSessionCache)
	jwtSvc := auth.NewJWTService("test-secret-key-32-bytes-minimum!")

	userRepo.On("FindByEmail", mock.Anything, "ghost@test.com").Return(nil, nil)

	h := handlers.NewAuthHandler(userRepo, jwtSvc, sessionCache)
	router := setupAuthRouter(h)

	w := postJSON(router, "/auth/login", map[string]string{
		"email": "ghost@test.com", "password": "anything",
	})

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLoginHandler_MissingFields(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionCache := new(MockSessionCache)
	jwtSvc := auth.NewJWTService("test-secret-key-32-bytes-minimum!")

	h := handlers.NewAuthHandler(userRepo, jwtSvc, sessionCache)
	router := setupAuthRouter(h)

	w := postJSON(router, "/auth/login", map[string]string{"email": "only@email.com"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRegisterHandler_EmailConflict(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionCache := new(MockSessionCache)
	jwtSvc := auth.NewJWTService("test-secret-key-32-bytes-minimum!")

	userRepo.On("FindByEmail", mock.Anything, "taken@test.com").Return(&models.User{
		ID: "existing-uuid", Email: "taken@test.com",
	}, nil)

	h := handlers.NewAuthHandler(userRepo, jwtSvc, sessionCache)
	router := setupAuthRouter(h)

	w := postJSON(router, "/auth/register", map[string]string{
		"email": "taken@test.com", "password": "password123",
	})

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestJWTService_GenerateAndValidate(t *testing.T) {
	svc := auth.NewJWTService("test-secret-key-32-bytes-minimum!!")

	token, err := svc.GenerateToken("user-123", "user@test.com", "user")
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := svc.ValidateToken(token)
	assert.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, "user@test.com", claims.Email)
	assert.Equal(t, "user", claims.Role)
}

func TestJWTService_InvalidToken(t *testing.T) {
	svc := auth.NewJWTService("test-secret-key-32-bytes-minimum!!")
	_, err := svc.ValidateToken("this.is.not.valid")
	assert.Error(t, err)
}
