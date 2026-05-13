package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

// NewPostgresDB creates a new PostgreSQL connection pool.
func NewPostgresDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	log.Println("Connected to PostgreSQL")
	return db, nil
}

// RunMigrations applies all schema migrations in order.
func RunMigrations(db *sql.DB) error {
	migrations := []string{
		createUsersTable,
		createProductsTable,
		createInventoryTable,
		createOrdersTable,
		createOrderItemsTable,
		createIndexes,
	}
	for i, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}
	log.Println("Database migrations applied")
	return nil
}

const createUsersTable = `
CREATE TABLE IF NOT EXISTS users (
	id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	email         TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
	created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`

const createProductsTable = `
CREATE TABLE IF NOT EXISTS products (
	id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	price       NUMERIC(12,2) NOT NULL CHECK (price > 0),
	category    TEXT NOT NULL,
	image_url   TEXT NOT NULL DEFAULT '',
	is_active   BOOLEAN NOT NULL DEFAULT TRUE,
	created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`

const createInventoryTable = `
CREATE TABLE IF NOT EXISTS inventory (
	id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
	quantity   INT NOT NULL DEFAULT 0 CHECK (quantity >= 0),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE(product_id)
);`

const createOrdersTable = `
CREATE TABLE IF NOT EXISTS orders (
	id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id      UUID NOT NULL REFERENCES users(id),
	status       TEXT NOT NULL DEFAULT 'pending'
	             CHECK (status IN ('pending','confirmed','shipped','delivered','cancelled')),
	total_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
	created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`

const createOrderItemsTable = `
CREATE TABLE IF NOT EXISTS order_items (
	id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	order_id   UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
	product_id UUID NOT NULL REFERENCES products(id),
	quantity   INT NOT NULL CHECK (quantity > 0),
	unit_price NUMERIC(12,2) NOT NULL
);`

const createIndexes = `
CREATE INDEX IF NOT EXISTS idx_products_category ON products(category);
CREATE INDEX IF NOT EXISTS idx_products_is_active ON products(is_active);
CREATE INDEX IF NOT EXISTS idx_orders_user_id    ON orders(user_id);
CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items(order_id);
`
