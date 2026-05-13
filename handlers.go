package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"github.com/yourusername/affordmed-api/internal/auth"
	"github.com/yourusername/affordmed-api/internal/cache"
	"github.com/yourusername/affordmed-api/internal/models"
	"github.com/yourusername/affordmed-api/internal/repository"
)

var startTime = time.Now()

// ─── Auth Handler ─────────────────────────────────────────────────────────────

type AuthHandler struct {
	users    repository.UserRepository
	jwt      auth.JWTService
	sessions cache.SessionCache
}

func NewAuthHandler(users repository.UserRepository, jwt auth.JWTService, sessions cache.SessionCache) *AuthHandler {
	return &AuthHandler{users: users, jwt: jwt, sessions: sessions}
}

// Register godoc
// POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, _ := h.users.FindByEmail(c.Request.Context(), req.Email)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	role := "user"
	if req.Role == "admin" {
		role = "admin" // In real app, restrict this behind an admin check
	}

	user := &models.User{Email: req.Email, PasswordHash: string(hash), Role: role}
	if err := h.users.Create(c.Request.Context(), user); err != nil {
		log.Printf("register error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	token, _ := h.jwt.GenerateToken(user.ID, user.Email, user.Role)
	refreshToken, _ := h.jwt.GenerateRefreshToken(user.ID)
	h.sessions.SetRefreshToken(c.Request.Context(), user.ID, refreshToken)

	c.JSON(http.StatusCreated, models.AuthResponse{
		Token: token, RefreshToken: refreshToken,
		ExpiresIn: 3600, User: user,
	})
}

// Login godoc
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.users.FindByEmail(c.Request.Context(), req.Email)
	if err != nil || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, _ := h.jwt.GenerateToken(user.ID, user.Email, user.Role)
	refreshToken, _ := h.jwt.GenerateRefreshToken(user.ID)
	h.sessions.SetRefreshToken(c.Request.Context(), user.ID, refreshToken)

	c.JSON(http.StatusOK, models.AuthResponse{
		Token: token, RefreshToken: refreshToken,
		ExpiresIn: 3600, User: user,
	})
}

// RefreshToken godoc
// POST /api/v1/auth/refresh
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var body struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	claims, err := h.jwt.ValidateToken(body.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}

	stored, _ := h.sessions.GetRefreshToken(c.Request.Context(), claims.UserID)
	if stored != body.RefreshToken {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token revoked"})
		return
	}

	user, _ := h.users.FindByID(c.Request.Context(), claims.UserID)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	token, _ := h.jwt.GenerateToken(user.ID, user.Email, user.Role)
	newRefresh, _ := h.jwt.GenerateRefreshToken(user.ID)
	h.sessions.SetRefreshToken(c.Request.Context(), user.ID, newRefresh)

	c.JSON(http.StatusOK, gin.H{"token": token, "refresh_token": newRefresh, "expires_in": 3600})
}

// ─── Product Handler ──────────────────────────────────────────────────────────

type ProductHandler struct {
	repo  repository.ProductRepository
	cache cache.ProductCache
}

func NewProductHandler(repo repository.ProductRepository, cache cache.ProductCache) *ProductHandler {
	return &ProductHandler{repo: repo, cache: cache}
}

// List godoc
// GET /api/v1/products
func (h *ProductHandler) List(c *gin.Context) {
	var params models.ProductListParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if params.Page == 0 { params.Page = 1 }
	if params.Limit == 0 { params.Limit = 20 }

	cacheKey := fmt.Sprintf("p%d:l%d:c%s:s%s", params.Page, params.Limit, params.Category, params.Search)
	if cached, _ := h.cache.GetList(c.Request.Context(), cacheKey); cached != nil {
		c.Header("X-Cache", "HIT")
		c.JSON(http.StatusOK, models.PaginatedProducts{Data: cached, Page: params.Page, Limit: params.Limit})
		return
	}

	products, total, err := h.repo.List(c.Request.Context(), params)
	if err != nil {
		log.Printf("product list error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list products"})
		return
	}

	h.cache.SetList(c.Request.Context(), cacheKey, products)
	c.Header("X-Cache", "MISS")
	c.JSON(http.StatusOK, models.PaginatedProducts{
		Data: products, Total: total, Page: params.Page, Limit: params.Limit,
	})
}

// GetByID godoc
// GET /api/v1/products/:id
func (h *ProductHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	if p, _ := h.cache.GetOne(c.Request.Context(), id); p != nil {
		c.Header("X-Cache", "HIT")
		c.JSON(http.StatusOK, p)
		return
	}
	p, err := h.repo.FindByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
		return
	}
	h.cache.SetOne(c.Request.Context(), p)
	c.Header("X-Cache", "MISS")
	c.JSON(http.StatusOK, p)
}

// Create godoc
// POST /api/v1/products (admin)
func (h *ProductHandler) Create(c *gin.Context) {
	var req models.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p := &models.Product{
		Name: req.Name, Description: req.Description,
		Price: req.Price, Category: req.Category,
		ImageURL: req.ImageURL, IsActive: true,
	}
	if err := h.repo.Create(c.Request.Context(), p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create product"})
		return
	}
	h.cache.InvalidateAllLists(c.Request.Context())
	c.JSON(http.StatusCreated, p)
}

// Update godoc
// PUT /api/v1/products/:id (admin)
func (h *ProductHandler) Update(c *gin.Context) {
	var req models.UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id := c.Param("id")
	p, err := h.repo.Update(c.Request.Context(), id, &req)
	if err != nil || p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
		return
	}
	h.cache.InvalidateProduct(c.Request.Context(), id)
	h.cache.InvalidateAllLists(c.Request.Context())
	c.JSON(http.StatusOK, p)
}

// Delete godoc
// DELETE /api/v1/products/:id (admin) – soft delete
func (h *ProductHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete product"})
		return
	}
	h.cache.InvalidateProduct(c.Request.Context(), id)
	h.cache.InvalidateAllLists(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"message": "product deleted"})
}

// ─── Order Handler ────────────────────────────────────────────────────────────

type OrderHandler struct {
	orders    repository.OrderRepository
	inventory repository.InventoryRepository
	prodCache cache.ProductCache
}

func NewOrderHandler(orders repository.OrderRepository, inventory repository.InventoryRepository, prodCache cache.ProductCache) *OrderHandler {
	return &OrderHandler{orders: orders, inventory: inventory, prodCache: prodCache}
}

// Create godoc
// POST /api/v1/orders
func (h *OrderHandler) Create(c *gin.Context) {
	var req models.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")

	// Build order items with prices
	var items []*models.OrderItem
	total := 0.0
	for _, i := range req.Items {
		// Get product price (try cache first)
		p, _ := h.prodCache.GetOne(c.Request.Context(), i.ProductID)
		if p == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("product %s not found", i.ProductID)})
			return
		}
		lineTotal := p.Price * float64(i.Quantity)
		total += lineTotal
		items = append(items, &models.OrderItem{
			ProductID: i.ProductID,
			Quantity:  i.Quantity,
			UnitPrice: p.Price,
		})
	}

	order := &models.Order{
		UserID:      fmt.Sprintf("%v", userID),
		Status:      models.OrderStatusPending,
		TotalAmount: total,
	}

	if err := h.orders.CreateWithItems(c.Request.Context(), order, items); err != nil {
		log.Printf("order create error: %v", err)
		if err.Error() != "" && len(err.Error()) > 18 && err.Error()[:18] == "insufficient stock" {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create order"})
		return
	}

	order.Items = items
	c.JSON(http.StatusCreated, order)
}

// GetByID godoc
// GET /api/v1/orders/:id
func (h *OrderHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")

	order, err := h.orders.FindByID(c.Request.Context(), id)
	if err != nil || order == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
		return
	}
	if order.UserID != fmt.Sprintf("%v", userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your order"})
		return
	}
	c.JSON(http.StatusOK, order)
}

// ListByUser godoc
// GET /api/v1/orders
func (h *OrderHandler) ListByUser(c *gin.Context) {
	userID, _ := c.Get("user_id")
	page, limit := 1, 20
	c.ShouldBindQuery(&struct {
		Page  *int `form:"page"`
		Limit *int `form:"limit"`
	}{&page, &limit})

	orders, total, err := h.orders.ListByUser(c.Request.Context(), fmt.Sprintf("%v", userID), page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list orders"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": orders, "total": total, "page": page})
}

// ─── Inventory Handler ────────────────────────────────────────────────────────

type InventoryHandler struct {
	repo  repository.InventoryRepository
	cache cache.ProductCache
}

func NewInventoryHandler(repo repository.InventoryRepository, cache cache.ProductCache) *InventoryHandler {
	return &InventoryHandler{repo: repo, cache: cache}
}

// Update godoc
// PUT /api/v1/inventory/:id (admin)
func (h *InventoryHandler) Update(c *gin.Context) {
	var req models.UpdateInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	productID := c.Param("id")
	if err := h.repo.Update(c.Request.Context(), productID, req.Quantity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update inventory"})
		return
	}
	// Invalidate product cache so stock info refreshes
	h.cache.InvalidateProduct(c.Request.Context(), productID)
	h.cache.InvalidateAllLists(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"message": "inventory updated", "product_id": productID, "quantity": req.Quantity})
}

// List godoc
// GET /api/v1/inventory (admin)
func (h *InventoryHandler) List(c *gin.Context) {
	page, limit := 1, 50
	inv, err := h.repo.List(c.Request.Context(), page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list inventory"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": inv})
}

// ─── Health Handler ────────────────────────────────────────────────────────────

type HealthHandler struct {
	db    *sql.DB
	redis *redis.Client
}

func NewHealthHandler(db *sql.DB, redis *redis.Client) *HealthHandler {
	return &HealthHandler{db: db, redis: redis}
}

// Check godoc
// GET /api/v1/health
func (h *HealthHandler) Check(c *gin.Context) {
	services := map[string]string{}

	if err := h.db.PingContext(c.Request.Context()); err != nil {
		services["postgres"] = "unhealthy"
	} else {
		services["postgres"] = "ok"
	}

	if err := h.redis.Ping(c.Request.Context()).Err(); err != nil {
		services["redis"] = "unhealthy"
	} else {
		services["redis"] = "ok"
	}

	status := "ok"
	code := http.StatusOK
	for _, v := range services {
		if v != "ok" {
			status = "degraded"
			code = http.StatusServiceUnavailable
			break
		}
	}

	c.JSON(code, models.HealthResponse{
		Status:   status,
		Services: services,
		UptimeS:  int64(time.Since(startTime).Seconds()),
	})
}
