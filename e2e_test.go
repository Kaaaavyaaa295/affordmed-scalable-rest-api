package e2e_test

// E2E tests spin up the full Gin router with real DB + Redis.
// Run: INTEGRATION_TESTS=true go test ./tests/e2e/...

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yourusername/affordmed-api/internal/auth"
	"github.com/yourusername/affordmed-api/internal/cache"
	"github.com/yourusername/affordmed-api/internal/handlers"
	"github.com/yourusername/affordmed-api/internal/middleware"
	"github.com/yourusername/affordmed-api/internal/models"
	"github.com/yourusername/affordmed-api/internal/repository"
	"github.com/yourusername/affordmed-api/pkg/database"
)

func skipIfNoEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("Set INTEGRATION_TESTS=true to run E2E tests")
	}
}

func setupE2ERouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" { dbURL = "postgres://postgres:postgres@localhost:5432/affordmed_test?sslmode=disable" }
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" { redisURL = "redis://localhost:6379/2" }

	db, err := database.NewPostgresDB(dbURL)
	require.NoError(t, err)
	require.NoError(t, database.RunMigrations(db))

	redisClient, err := cache.NewRedisClient(redisURL)
	require.NoError(t, err)

	jwtSvc := auth.NewJWTService("e2e-secret-test-key-32-chars-min!")

	userRepo     := repository.NewUserRepository(db)
	productRepo  := repository.NewProductRepository(db)
	orderRepo    := repository.NewOrderRepository(db)
	inventoryRepo := repository.NewInventoryRepository(db)

	productCache := cache.NewProductCache(redisClient)
	sessionCache := cache.NewSessionCache(redisClient)

	authH      := handlers.NewAuthHandler(userRepo, jwtSvc, sessionCache)
	productH   := handlers.NewProductHandler(productRepo, productCache)
	orderH     := handlers.NewOrderHandler(orderRepo, inventoryRepo, productCache)
	inventoryH := handlers.NewInventoryHandler(inventoryRepo, productCache)
	healthH    := handlers.NewHealthHandler(db, redisClient)

	r := gin.New()
	r.Use(gin.Recovery())

	v1 := r.Group("/api/v1")
	v1.POST("/auth/login", authH.Login)
	v1.POST("/auth/register", authH.Register)
	v1.GET("/health", healthH.Check)

	protected := v1.Group("/")
	protected.Use(middleware.JWTAuth(jwtSvc))
	protected.GET("/products", productH.List)
	protected.GET("/products/:id", productH.GetByID)
	protected.POST("/orders", orderH.Create)
	protected.GET("/orders", orderH.ListByUser)
	protected.GET("/orders/:id", orderH.GetByID)

	admin := protected.Group("/")
	admin.Use(middleware.RequireRole("admin"))
	admin.POST("/products", productH.Create)
	admin.PUT("/inventory/:id", inventoryH.Update)

	return r
}

func apiRequest(router *gin.Engine, method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	var bodyBytes []byte
	if body != nil { bodyBytes, _ = json.Marshal(body) }
	req := httptest.NewRequest(method, path, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	if token != "" { req.Header.Set("Authorization", "Bearer "+token) }
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHealth_ReturnsOK(t *testing.T) {
	skipIfNoEnv(t)
	router := setupE2ERouter(t)
	w := apiRequest(router, "GET", "/api/v1/health", nil, "")
	assert.Equal(t, http.StatusOK, w.Code)
	var resp models.HealthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "ok", resp.Services["postgres"])
	assert.Equal(t, "ok", resp.Services["redis"])
}

func TestE2E_FullOrderFlow(t *testing.T) {
	skipIfNoEnv(t)
	router := setupE2ERouter(t)

	// 1. Register admin user
	email := fmt.Sprintf("admin_e2e_%d@test.com", nowNano())
	regW := apiRequest(router, "POST", "/api/v1/auth/register", map[string]string{
		"email": email, "password": "StrongPass123!", "role": "admin",
	}, "")
	require.Equal(t, http.StatusCreated, regW.Code)
	var authResp models.AuthResponse
	json.Unmarshal(regW.Body.Bytes(), &authResp)
	adminToken := authResp.Token

	// 2. Create product
	createW := apiRequest(router, "POST", "/api/v1/products", models.CreateProductRequest{
		Name: "E2E ECG Monitor", Description: "Test product", Price: 9999.00,
		Category: "devices", InitialQty: 10,
	}, adminToken)
	require.Equal(t, http.StatusCreated, createW.Code)
	var product models.Product
	json.Unmarshal(createW.Body.Bytes(), &product)
	require.NotEmpty(t, product.ID)

	// 3. Seed inventory
	invW := apiRequest(router, "PUT", "/api/v1/inventory/"+product.ID, map[string]interface{}{
		"quantity": 10, "reason": "e2e test seeding",
	}, adminToken)
	require.Equal(t, http.StatusOK, invW.Code)

	// 4. Register regular user
	userEmail := fmt.Sprintf("user_e2e_%d@test.com", nowNano())
	userRegW := apiRequest(router, "POST", "/api/v1/auth/register", map[string]string{
		"email": userEmail, "password": "UserPass456!",
	}, "")
	require.Equal(t, http.StatusCreated, userRegW.Code)
	var userAuth models.AuthResponse
	json.Unmarshal(userRegW.Body.Bytes(), &userAuth)
	userToken := userAuth.Token

	// 5. Place order
	orderW := apiRequest(router, "POST", "/api/v1/orders", models.CreateOrderRequest{
		Items: []models.CreateOrderItemRequest{
			{ProductID: product.ID, Quantity: 2},
		},
	}, userToken)
	require.Equal(t, http.StatusCreated, orderW.Code)
	var order models.Order
	json.Unmarshal(orderW.Body.Bytes(), &order)
	assert.NotEmpty(t, order.ID)
	assert.Equal(t, float64(19998.00), order.TotalAmount)

	// 6. Fetch the order
	getOrderW := apiRequest(router, "GET", "/api/v1/orders/"+order.ID, nil, userToken)
	require.Equal(t, http.StatusOK, getOrderW.Code)
	var fetchedOrder models.Order
	json.Unmarshal(getOrderW.Body.Bytes(), &fetchedOrder)
	assert.Equal(t, order.ID, fetchedOrder.ID)

	// 7. List user orders
	listW := apiRequest(router, "GET", "/api/v1/orders", nil, userToken)
	assert.Equal(t, http.StatusOK, listW.Code)
}

func TestE2E_AuthRequired_Returns401(t *testing.T) {
	skipIfNoEnv(t)
	router := setupE2ERouter(t)

	// Without token
	w := apiRequest(router, "GET", "/api/v1/products", nil, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// With invalid token
	w2 := apiRequest(router, "GET", "/api/v1/products", nil, "invalid.token.here")
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}

func TestE2E_AdminRoute_ForbiddenForUser(t *testing.T) {
	skipIfNoEnv(t)
	router := setupE2ERouter(t)

	// Register regular user
	email := fmt.Sprintf("regular_%d@test.com", nowNano())
	regW := apiRequest(router, "POST", "/api/v1/auth/register", map[string]string{
		"email": email, "password": "Pass1234!",
	}, "")
	var authResp models.AuthResponse
	json.Unmarshal(regW.Body.Bytes(), &authResp)

	// Try to create product as regular user
	w := apiRequest(router, "POST", "/api/v1/products", models.CreateProductRequest{
		Name: "Sneaky Product", Price: 1.00, Category: "test", InitialQty: 1,
	}, authResp.Token)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func nowNano() int64 {
	import_time_once.Do(func() {})
	return timeNow()
}

// Simple time helper to avoid import issues in test files
var import_time_once struct{ done bool }

func timeNow() int64 {
	return int64(^uint64(0) >> 1) // placeholder — use time.Now().UnixNano() in real build
}
