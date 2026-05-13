package unit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/yourusername/affordmed-api/internal/handlers"
	"github.com/yourusername/affordmed-api/internal/models"
)

// ─── Mock Product Repo ────────────────────────────────────────────────────────

type MockProductRepo struct{ mock.Mock }

func (m *MockProductRepo) Create(ctx context.Context, p *models.Product) error {
	args := m.Called(ctx, p); p.ID = "prod-uuid-1"; return args.Error(0)
}
func (m *MockProductRepo) FindByID(ctx context.Context, id string) (*models.Product, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil { return nil, args.Error(1) }
	return args.Get(0).(*models.Product), args.Error(1)
}
func (m *MockProductRepo) List(ctx context.Context, params models.ProductListParams) ([]*models.Product, int64, error) {
	args := m.Called(ctx, params)
	return args.Get(0).([]*models.Product), args.Get(1).(int64), args.Error(2)
}
func (m *MockProductRepo) Update(ctx context.Context, id string, req *models.UpdateProductRequest) (*models.Product, error) {
	args := m.Called(ctx, id, req)
	if args.Get(0) == nil { return nil, args.Error(1) }
	return args.Get(0).(*models.Product), args.Error(1)
}
func (m *MockProductRepo) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

// ─── Mock Product Cache ───────────────────────────────────────────────────────

type MockProductCache struct{ mock.Mock }

func (m *MockProductCache) GetList(ctx context.Context, key string) ([]*models.Product, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil { return nil, args.Error(1) }
	return args.Get(0).([]*models.Product), args.Error(1)
}
func (m *MockProductCache) SetList(ctx context.Context, key string, products []*models.Product) error {
	return m.Called(ctx, key, products).Error(0)
}
func (m *MockProductCache) GetOne(ctx context.Context, id string) (*models.Product, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil { return nil, args.Error(1) }
	return args.Get(0).(*models.Product), args.Error(1)
}
func (m *MockProductCache) SetOne(ctx context.Context, p *models.Product) error {
	return m.Called(ctx, p).Error(0)
}
func (m *MockProductCache) InvalidateProduct(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *MockProductCache) InvalidateAllLists(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func setupProductRouter(h *handlers.ProductHandler) *gin.Engine {
	r := gin.New()
	r.GET("/products", h.List)
	r.GET("/products/:id", h.GetByID)
	r.POST("/products", h.Create)
	r.PUT("/products/:id", h.Update)
	r.DELETE("/products/:id", h.Delete)
	return r
}

func getRequest(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func sampleProducts() []*models.Product {
	return []*models.Product{
		{ID: "p1", Name: "ECG Monitor", Price: 4999.99, Category: "devices", IsActive: true, CreatedAt: time.Now()},
		{ID: "p2", Name: "Pulse Oximeter", Price: 1299.00, Category: "devices", IsActive: true, CreatedAt: time.Now()},
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestProductList_PaginationDefaults(t *testing.T) {
	repo := new(MockProductRepo)
	cacheM := new(MockProductCache)
	products := sampleProducts()

	cacheM.On("GetList", mock.Anything, mock.Anything).Return(nil, nil)
	repo.On("List", mock.Anything, mock.MatchedBy(func(p models.ProductListParams) bool {
		return p.Page == 1 && p.Limit == 20
	})).Return(products, int64(2), nil)
	cacheM.On("SetList", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	h := handlers.NewProductHandler(repo, cacheM)
	router := setupProductRouter(h)

	w := getRequest(router, "/products")

	assert.Equal(t, http.StatusOK, w.Code)
	var resp models.PaginatedProducts
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 2)
	assert.Equal(t, int64(2), resp.Total)
	assert.Equal(t, 1, resp.Page)
	repo.AssertExpectations(t)
}

func TestProductList_CategoryFilter(t *testing.T) {
	repo := new(MockProductRepo)
	cacheM := new(MockProductCache)

	cacheM.On("GetList", mock.Anything, mock.Anything).Return(nil, nil)
	repo.On("List", mock.Anything, mock.MatchedBy(func(p models.ProductListParams) bool {
		return p.Category == "devices"
	})).Return(sampleProducts(), int64(2), nil)
	cacheM.On("SetList", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	h := handlers.NewProductHandler(repo, cacheM)
	router := setupProductRouter(h)

	w := getRequest(router, "/products?category=devices")
	assert.Equal(t, http.StatusOK, w.Code)
	repo.AssertExpectations(t)
}

func TestProductList_CacheHit(t *testing.T) {
	repo := new(MockProductRepo)
	cacheM := new(MockProductCache)

	cacheM.On("GetList", mock.Anything, mock.Anything).Return(sampleProducts(), nil)
	// repo.List should NOT be called when cache hits

	h := handlers.NewProductHandler(repo, cacheM)
	router := setupProductRouter(h)

	w := getRequest(router, "/products")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "HIT", w.Header().Get("X-Cache"))
	repo.AssertNotCalled(t, "List", mock.Anything, mock.Anything)
}

func TestProductGetByID_Found(t *testing.T) {
	repo := new(MockProductRepo)
	cacheM := new(MockProductCache)
	p := &models.Product{ID: "p1", Name: "ECG Monitor", Price: 4999.99, Category: "devices"}

	cacheM.On("GetOne", mock.Anything, "p1").Return(nil, nil)
	repo.On("FindByID", mock.Anything, "p1").Return(p, nil)
	cacheM.On("SetOne", mock.Anything, p).Return(nil)

	h := handlers.NewProductHandler(repo, cacheM)
	router := setupProductRouter(h)

	w := getRequest(router, "/products/p1")
	assert.Equal(t, http.StatusOK, w.Code)
	var result models.Product
	json.Unmarshal(w.Body.Bytes(), &result)
	assert.Equal(t, "ECG Monitor", result.Name)
}

func TestProductGetByID_NotFound(t *testing.T) {
	repo := new(MockProductRepo)
	cacheM := new(MockProductCache)

	cacheM.On("GetOne", mock.Anything, "nonexistent").Return(nil, nil)
	repo.On("FindByID", mock.Anything, "nonexistent").Return(nil, nil)

	h := handlers.NewProductHandler(repo, cacheM)
	router := setupProductRouter(h)

	w := getRequest(router, "/products/nonexistent")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProductGetByID_CacheHit(t *testing.T) {
	repo := new(MockProductRepo)
	cacheM := new(MockProductCache)
	p := &models.Product{ID: "p1", Name: "ECG Monitor", Price: 4999.99}

	cacheM.On("GetOne", mock.Anything, "p1").Return(p, nil)

	h := handlers.NewProductHandler(repo, cacheM)
	router := setupProductRouter(h)

	w := getRequest(router, "/products/p1")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "HIT", w.Header().Get("X-Cache"))
	repo.AssertNotCalled(t, "FindByID")
}
