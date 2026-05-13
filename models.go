package models

import (
	"time"
)

// ─── User ────────────────────────────────────────────────────────────────────

type User struct {
	ID           string    `json:"id" db:"id"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Role         string    `json:"role" db:"role"` // "user" | "admin"
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	User         *User  `json:"user"`
}

// ─── Product ─────────────────────────────────────────────────────────────────

type Product struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Price       float64   `json:"price" db:"price"`
	Category    string    `json:"category" db:"category"`
	ImageURL    string    `json:"image_url" db:"image_url"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type CreateProductRequest struct {
	Name        string  `json:"name" binding:"required,min=2"`
	Description string  `json:"description"`
	Price       float64 `json:"price" binding:"required,gt=0"`
	Category    string  `json:"category" binding:"required"`
	ImageURL    string  `json:"image_url"`
	InitialQty  int     `json:"initial_qty" binding:"required,gte=0"`
}

type UpdateProductRequest struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Price       *float64 `json:"price,omitempty" binding:"omitempty,gt=0"`
	Category    *string  `json:"category,omitempty"`
	ImageURL    *string  `json:"image_url,omitempty"`
	IsActive    *bool    `json:"is_active,omitempty"`
}

type ProductListParams struct {
	Page     int    `form:"page,default=1" binding:"min=1"`
	Limit    int    `form:"limit,default=20" binding:"min=1,max=100"`
	Category string `form:"category"`
	Search   string `form:"search"`
}

type PaginatedProducts struct {
	Data  []*Product `json:"data"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Limit int        `json:"limit"`
}

// ─── Order ───────────────────────────────────────────────────────────────────

type OrderStatus string

const (
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusConfirmed  OrderStatus = "confirmed"
	OrderStatusShipped    OrderStatus = "shipped"
	OrderStatusDelivered  OrderStatus = "delivered"
	OrderStatusCancelled  OrderStatus = "cancelled"
)

type OrderItem struct {
	ID        string  `json:"id" db:"id"`
	OrderID   string  `json:"order_id" db:"order_id"`
	ProductID string  `json:"product_id" db:"product_id"`
	Product   *Product `json:"product,omitempty"`
	Quantity  int     `json:"quantity" db:"quantity"`
	UnitPrice float64 `json:"unit_price" db:"unit_price"`
}

type Order struct {
	ID          string      `json:"id" db:"id"`
	UserID      string      `json:"user_id" db:"user_id"`
	Status      OrderStatus `json:"status" db:"status"`
	TotalAmount float64     `json:"total_amount" db:"total_amount"`
	Items       []*OrderItem `json:"items,omitempty"`
	CreatedAt   time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at" db:"updated_at"`
}

type CreateOrderRequest struct {
	Items []CreateOrderItemRequest `json:"items" binding:"required,min=1,dive"`
}

type CreateOrderItemRequest struct {
	ProductID string `json:"product_id" binding:"required,uuid"`
	Quantity  int    `json:"quantity" binding:"required,min=1"`
}

// ─── Inventory ────────────────────────────────────────────────────────────────

type Inventory struct {
	ID        string    `json:"id" db:"id"`
	ProductID string    `json:"product_id" db:"product_id"`
	Quantity  int       `json:"quantity" db:"quantity"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type UpdateInventoryRequest struct {
	Quantity int    `json:"quantity" binding:"required,gte=0"`
	Reason   string `json:"reason" binding:"required"`
}

// ─── Common ───────────────────────────────────────────────────────────────────

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code,omitempty"`
}

type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type HealthResponse struct {
	Status   string            `json:"status"`
	Services map[string]string `json:"services"`
	UptimeS  int64             `json:"uptime_s"`
}
