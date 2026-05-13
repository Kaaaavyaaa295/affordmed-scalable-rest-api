package integration_test

// Integration tests require a real PostgreSQL + Redis instance.
// Run with: TEST_DB_URL=... TEST_REDIS_URL=... go test ./tests/integration/...
//
// For CI, docker-compose.test.yml spins up these services automatically.

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yourusername/affordmed-api/internal/cache"
	"github.com/yourusername/affordmed-api/internal/models"
	"github.com/yourusername/affordmed-api/internal/repository"
	"github.com/yourusername/affordmed-api/pkg/database"
)

func dbURL() string {
	if v := os.Getenv("TEST_DB_URL"); v != "" { return v }
	return "postgres://postgres:postgres@localhost:5432/affordmed_test?sslmode=disable"
}

func redisURL() string {
	if v := os.Getenv("TEST_REDIS_URL"); v != "" { return v }
	return "redis://localhost:6379/1"
}

func skipIfNoEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("Set INTEGRATION_TESTS=true to run integration tests")
	}
}

// ─── Order Transaction Tests ──────────────────────────────────────────────────

func TestOrderTx_StockDeductedCorrectly(t *testing.T) {
	skipIfNoEnv(t)
	db, err := database.NewPostgresDB(dbURL())
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, database.RunMigrations(db))

	ctx := context.Background()
	userRepo := repository.NewUserRepository(db)
	productRepo := repository.NewProductRepository(db)
	inventoryRepo := repository.NewInventoryRepository(db)
	orderRepo := repository.NewOrderRepository(db)

	// Create test user
	user := &models.User{Email: "order_test@test.com", PasswordHash: "hash", Role: "user"}
	require.NoError(t, userRepo.Create(ctx, user))

	// Create test product
	product := &models.Product{Name: "Test Oximeter", Price: 1299.00, Category: "devices", IsActive: true}
	require.NoError(t, productRepo.Create(ctx, product))

	// Seed inventory with 10 units
	inv := &models.Inventory{ProductID: product.ID, Quantity: 10}
	require.NoError(t, inventoryRepo.Create(ctx, inv))

	// Create order for 3 units
	order := &models.Order{UserID: user.ID, Status: models.OrderStatusPending, TotalAmount: 3897.00}
	items := []*models.OrderItem{{ProductID: product.ID, Quantity: 3, UnitPrice: 1299.00}}
	require.NoError(t, orderRepo.CreateWithItems(ctx, order, items))

	// Verify stock was deducted
	updatedInv, err := inventoryRepo.FindByProductID(ctx, product.ID)
	require.NoError(t, err)
	assert.Equal(t, 7, updatedInv.Quantity, "stock should be 10 - 3 = 7")

	// Verify order was created
	assert.NotEmpty(t, order.ID)
	assert.NotEmpty(t, items[0].ID)
}

func TestOrderTx_RollbackOnInsufficientStock(t *testing.T) {
	skipIfNoEnv(t)
	db, err := database.NewPostgresDB(dbURL())
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, database.RunMigrations(db))

	ctx := context.Background()
	userRepo := repository.NewUserRepository(db)
	productRepo := repository.NewProductRepository(db)
	inventoryRepo := repository.NewInventoryRepository(db)
	orderRepo := repository.NewOrderRepository(db)

	user := &models.User{Email: "rollback_test@test.com", PasswordHash: "hash", Role: "user"}
	require.NoError(t, userRepo.Create(ctx, user))

	product := &models.Product{Name: "Low Stock Item", Price: 500.00, Category: "devices", IsActive: true}
	require.NoError(t, productRepo.Create(ctx, product))

	inv := &models.Inventory{ProductID: product.ID, Quantity: 2}
	require.NoError(t, inventoryRepo.Create(ctx, inv))

	// Try to order 5 units when only 2 available — should fail
	order := &models.Order{UserID: user.ID, Status: models.OrderStatusPending, TotalAmount: 2500.00}
	items := []*models.OrderItem{{ProductID: product.ID, Quantity: 5, UnitPrice: 500.00}}
	err = orderRepo.CreateWithItems(ctx, order, items)
	assert.Error(t, err, "should fail: insufficient stock")
	assert.Contains(t, err.Error(), "insufficient stock")

	// Verify stock was NOT modified (transaction rolled back)
	updatedInv, _ := inventoryRepo.FindByProductID(ctx, product.ID)
	assert.Equal(t, 2, updatedInv.Quantity, "stock should remain at 2 after rollback")
}

// ─── Redis Cache Tests ────────────────────────────────────────────────────────

func TestRedisCache_SetAndGetProduct(t *testing.T) {
	skipIfNoEnv(t)
	client, err := cache.NewRedisClient(redisURL())
	require.NoError(t, err)

	ctx := context.Background()
	productCache := cache.NewProductCache(client)

	p := &models.Product{
		ID: "cache-test-uuid", Name: "ECG Monitor",
		Price: 4999.99, Category: "devices", IsActive: true,
	}
	require.NoError(t, productCache.SetOne(ctx, p))

	got, err := productCache.GetOne(ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, p.Name, got.Name)
	assert.Equal(t, p.Price, got.Price)
}

func TestRedisCache_MissReturnsNil(t *testing.T) {
	skipIfNoEnv(t)
	client, err := cache.NewRedisClient(redisURL())
	require.NoError(t, err)

	productCache := cache.NewProductCache(client)
	got, err := productCache.GetOne(context.Background(), "nonexistent-id-zzz")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestRedisCache_SetAndGetList(t *testing.T) {
	skipIfNoEnv(t)
	client, err := cache.NewRedisClient(redisURL())
	require.NoError(t, err)

	ctx := context.Background()
	productCache := cache.NewProductCache(client)

	products := []*models.Product{
		{ID: "list-p1", Name: "Device A", Price: 999.00, Category: "devices"},
		{ID: "list-p2", Name: "Device B", Price: 1499.00, Category: "devices"},
	}

	const key = "p1:l20:cdevices:s"
	require.NoError(t, productCache.SetList(ctx, key, products))

	got, err := productCache.GetList(ctx, key)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "Device A", got[0].Name)
}

func TestRedisCache_InvalidationRemovesKey(t *testing.T) {
	skipIfNoEnv(t)
	client, err := cache.NewRedisClient(redisURL())
	require.NoError(t, err)

	ctx := context.Background()
	productCache := cache.NewProductCache(client)

	p := &models.Product{ID: "invalidate-test", Name: "Temp Product", Price: 100.00}
	require.NoError(t, productCache.SetOne(ctx, p))

	// Confirm it's cached
	got, _ := productCache.GetOne(ctx, p.ID)
	require.NotNil(t, got)

	// Invalidate
	require.NoError(t, productCache.InvalidateProduct(ctx, p.ID))

	// Now it should be gone
	got, err := productCache.GetOne(ctx, p.ID)
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestSessionCache_RefreshTokenFlow(t *testing.T) {
	skipIfNoEnv(t)
	client, err := cache.NewRedisClient(redisURL())
	require.NoError(t, err)

	ctx := context.Background()
	sessionCache := cache.NewSessionCache(client)

	userID := "session-test-user"
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test"

	require.NoError(t, sessionCache.SetRefreshToken(ctx, userID, token))

	got, err := sessionCache.GetRefreshToken(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, token, got)

	require.NoError(t, sessionCache.DeleteRefreshToken(ctx, userID))
	got, _ = sessionCache.GetRefreshToken(ctx, userID)
	assert.Empty(t, got)
}
