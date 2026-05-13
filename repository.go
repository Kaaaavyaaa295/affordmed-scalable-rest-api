package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/affordmed-api/internal/models"
)

// ─── User Repository ──────────────────────────────────────────────────────────

type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindByID(ctx context.Context, id string) (*models.User, error)
}

type userRepository struct{ db *sql.DB }

func NewUserRepository(db *sql.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, u *models.User) error {
	q := `INSERT INTO users (email, password_hash, role)
	      VALUES ($1, $2, $3)
	      RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, q, u.Email, u.PasswordHash, u.Role).
		Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	u := &models.User{}
	q := `SELECT id, email, password_hash, role, created_at, updated_at
	      FROM users WHERE email = $1`
	err := r.db.QueryRowContext(ctx, q, email).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (r *userRepository) FindByID(ctx context.Context, id string) (*models.User, error) {
	u := &models.User{}
	q := `SELECT id, email, password_hash, role, created_at, updated_at
	      FROM users WHERE id = $1`
	err := r.db.QueryRowContext(ctx, q, id).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

// ─── Product Repository ───────────────────────────────────────────────────────

type ProductRepository interface {
	Create(ctx context.Context, p *models.Product) error
	FindByID(ctx context.Context, id string) (*models.Product, error)
	List(ctx context.Context, params models.ProductListParams) ([]*models.Product, int64, error)
	Update(ctx context.Context, id string, req *models.UpdateProductRequest) (*models.Product, error)
	Delete(ctx context.Context, id string) error
}

type productRepository struct{ db *sql.DB }

func NewProductRepository(db *sql.DB) ProductRepository {
	return &productRepository{db: db}
}

func (r *productRepository) Create(ctx context.Context, p *models.Product) error {
	q := `INSERT INTO products (name, description, price, category, image_url, is_active)
	      VALUES ($1, $2, $3, $4, $5, $6)
	      RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, q,
		p.Name, p.Description, p.Price, p.Category, p.ImageURL, p.IsActive,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *productRepository) FindByID(ctx context.Context, id string) (*models.Product, error) {
	p := &models.Product{}
	q := `SELECT id, name, description, price, category, image_url, is_active, created_at, updated_at
	      FROM products WHERE id = $1 AND is_active = TRUE`
	err := r.db.QueryRowContext(ctx, q, id).
		Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Category,
			&p.ImageURL, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (r *productRepository) List(ctx context.Context, params models.ProductListParams) ([]*models.Product, int64, error) {
	where := []string{"is_active = TRUE"}
	args := []interface{}{}
	i := 1

	if params.Category != "" {
		where = append(where, fmt.Sprintf("category = $%d", i))
		args = append(args, params.Category)
		i++
	}
	if params.Search != "" {
		where = append(where, fmt.Sprintf("name ILIKE $%d", i))
		args = append(args, "%"+params.Search+"%")
		i++
	}

	whereClause := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM products "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	args = append(args, params.Limit, offset)
	q := fmt.Sprintf(`SELECT id, name, description, price, category, image_url, is_active, created_at, updated_at
	      FROM products %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		whereClause, i, i+1)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var products []*models.Product
	for rows.Next() {
		p := &models.Product{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Category,
			&p.ImageURL, &p.IsActive, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, err
		}
		products = append(products, p)
	}
	return products, total, rows.Err()
}

func (r *productRepository) Update(ctx context.Context, id string, req *models.UpdateProductRequest) (*models.Product, error) {
	sets := []string{}
	args := []interface{}{}
	i := 1

	if req.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", i)); args = append(args, *req.Name); i++
	}
	if req.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", i)); args = append(args, *req.Description); i++
	}
	if req.Price != nil {
		sets = append(sets, fmt.Sprintf("price = $%d", i)); args = append(args, *req.Price); i++
	}
	if req.Category != nil {
		sets = append(sets, fmt.Sprintf("category = $%d", i)); args = append(args, *req.Category); i++
	}
	if req.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", i)); args = append(args, *req.IsActive); i++
	}
	if len(sets) == 0 {
		return r.FindByID(ctx, id)
	}
	sets = append(sets, fmt.Sprintf("updated_at = $%d", i)); args = append(args, time.Now()); i++
	args = append(args, id)

	q := fmt.Sprintf(`UPDATE products SET %s WHERE id = $%d
	      RETURNING id, name, description, price, category, image_url, is_active, created_at, updated_at`,
		strings.Join(sets, ", "), i)

	p := &models.Product{}
	err := r.db.QueryRowContext(ctx, q, args...).
		Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Category,
			&p.ImageURL, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (r *productRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE products SET is_active=FALSE WHERE id=$1", id)
	return err
}

// ─── Order Repository ─────────────────────────────────────────────────────────

type OrderRepository interface {
	CreateWithItems(ctx context.Context, order *models.Order, items []*models.OrderItem) error
	FindByID(ctx context.Context, id string) (*models.Order, error)
	ListByUser(ctx context.Context, userID string, page, limit int) ([]*models.Order, int64, error)
}

type orderRepository struct{ db *sql.DB }

func NewOrderRepository(db *sql.DB) OrderRepository {
	return &orderRepository{db: db}
}

func (r *orderRepository) CreateWithItems(ctx context.Context, order *models.Order, items []*models.OrderItem) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Create order
	err = tx.QueryRowContext(ctx,
		`INSERT INTO orders (user_id, status, total_amount) VALUES ($1,$2,$3)
		 RETURNING id, created_at, updated_at`,
		order.UserID, order.Status, order.TotalAmount,
	).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		return err
	}

	// Deduct inventory & insert items
	for _, item := range items {
		// Check and deduct stock atomically
		res, err := tx.ExecContext(ctx,
			`UPDATE inventory SET quantity = quantity - $1, updated_at = NOW()
			 WHERE product_id = $2 AND quantity >= $1`,
			item.Quantity, item.ProductID,
		)
		if err != nil {
			return err
		}
		rows, _ := res.RowsAffected()
		if rows == 0 {
			return fmt.Errorf("insufficient stock for product %s", item.ProductID)
		}

		// Insert order item
		err = tx.QueryRowContext(ctx,
			`INSERT INTO order_items (order_id, product_id, quantity, unit_price)
			 VALUES ($1,$2,$3,$4) RETURNING id`,
			order.ID, item.ProductID, item.Quantity, item.UnitPrice,
		).Scan(&item.ID)
		if err != nil {
			return err
		}
		item.OrderID = order.ID
	}

	return tx.Commit()
}

func (r *orderRepository) FindByID(ctx context.Context, id string) (*models.Order, error) {
	o := &models.Order{}
	q := `SELECT id, user_id, status, total_amount, created_at, updated_at
	      FROM orders WHERE id = $1`
	err := r.db.QueryRowContext(ctx, q, id).
		Scan(&o.ID, &o.UserID, &o.Status, &o.TotalAmount, &o.CreatedAt, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Load items
	rows, err := r.db.QueryContext(ctx,
		`SELECT oi.id, oi.product_id, oi.quantity, oi.unit_price,
		        p.name, p.description, p.category
		 FROM order_items oi
		 JOIN products p ON p.id = oi.product_id
		 WHERE oi.order_id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		item := &models.OrderItem{OrderID: id}
		item.Product = &models.Product{}
		if err := rows.Scan(&item.ID, &item.ProductID, &item.Quantity, &item.UnitPrice,
			&item.Product.Name, &item.Product.Description, &item.Product.Category); err != nil {
			return nil, err
		}
		item.Product.ID = item.ProductID
		o.Items = append(o.Items, item)
	}
	return o, rows.Err()
}

func (r *orderRepository) ListByUser(ctx context.Context, userID string, page, limit int) ([]*models.Order, int64, error) {
	var total int64
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM orders WHERE user_id=$1", userID).Scan(&total)

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, status, total_amount, created_at, updated_at
		 FROM orders WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var orders []*models.Order
	for rows.Next() {
		o := &models.Order{}
		if err := rows.Scan(&o.ID, &o.UserID, &o.Status, &o.TotalAmount, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, 0, err
		}
		orders = append(orders, o)
	}
	return orders, total, rows.Err()
}

// ─── Inventory Repository ─────────────────────────────────────────────────────

type InventoryRepository interface {
	Create(ctx context.Context, inv *models.Inventory) error
	FindByProductID(ctx context.Context, productID string) (*models.Inventory, error)
	Update(ctx context.Context, productID string, qty int) error
	List(ctx context.Context, page, limit int) ([]*models.Inventory, error)
}

type inventoryRepository struct{ db *sql.DB }

func NewInventoryRepository(db *sql.DB) InventoryRepository {
	return &inventoryRepository{db: db}
}

func (r *inventoryRepository) Create(ctx context.Context, inv *models.Inventory) error {
	q := `INSERT INTO inventory (product_id, quantity) VALUES ($1,$2)
	      RETURNING id, updated_at`
	return r.db.QueryRowContext(ctx, q, inv.ProductID, inv.Quantity).
		Scan(&inv.ID, &inv.UpdatedAt)
}

func (r *inventoryRepository) FindByProductID(ctx context.Context, productID string) (*models.Inventory, error) {
	inv := &models.Inventory{}
	q := `SELECT id, product_id, quantity, updated_at FROM inventory WHERE product_id=$1`
	err := r.db.QueryRowContext(ctx, q, productID).
		Scan(&inv.ID, &inv.ProductID, &inv.Quantity, &inv.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return inv, err
}

func (r *inventoryRepository) Update(ctx context.Context, productID string, qty int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE inventory SET quantity=$1, updated_at=NOW() WHERE product_id=$2`, qty, productID)
	return err
}

func (r *inventoryRepository) List(ctx context.Context, page, limit int) ([]*models.Inventory, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, product_id, quantity, updated_at FROM inventory
		 ORDER BY updated_at DESC LIMIT $1 OFFSET $2`,
		limit, (page-1)*limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*models.Inventory
	for rows.Next() {
		inv := &models.Inventory{}
		rows.Scan(&inv.ID, &inv.ProductID, &inv.Quantity, &inv.UpdatedAt)
		list = append(list, inv)
	}
	return list, rows.Err()
}
