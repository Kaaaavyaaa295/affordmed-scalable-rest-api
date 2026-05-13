package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourusername/affordmed-api/internal/models"
)

// ─── Redis client ─────────────────────────────────────────────────────────────

func NewRedisClient(url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}

// ─── Product cache ────────────────────────────────────────────────────────────

const (
	productListTTL   = 5 * time.Minute
	productDetailTTL = 10 * time.Minute
)

type ProductCache interface {
	GetList(ctx context.Context, key string) ([]*models.Product, error)
	SetList(ctx context.Context, key string, products []*models.Product) error
	GetOne(ctx context.Context, id string) (*models.Product, error)
	SetOne(ctx context.Context, product *models.Product) error
	InvalidateProduct(ctx context.Context, id string) error
	InvalidateAllLists(ctx context.Context) error
}

type productCache struct {
	client *redis.Client
}

func NewProductCache(client *redis.Client) ProductCache {
	return &productCache{client: client}
}

func (c *productCache) GetList(ctx context.Context, key string) ([]*models.Product, error) {
	data, err := c.client.Get(ctx, "product:list:"+key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var products []*models.Product
	if err := json.Unmarshal(data, &products); err != nil {
		return nil, err
	}
	return products, nil
}

func (c *productCache) SetList(ctx context.Context, key string, products []*models.Product) error {
	data, err := json.Marshal(products)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, "product:list:"+key, data, productListTTL).Err()
}

func (c *productCache) GetOne(ctx context.Context, id string) (*models.Product, error) {
	data, err := c.client.Get(ctx, "product:"+id).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var product models.Product
	if err := json.Unmarshal(data, &product); err != nil {
		return nil, err
	}
	return &product, nil
}

func (c *productCache) SetOne(ctx context.Context, product *models.Product) error {
	data, err := json.Marshal(product)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, "product:"+product.ID, data, productDetailTTL).Err()
}

func (c *productCache) InvalidateProduct(ctx context.Context, id string) error {
	return c.client.Del(ctx, "product:"+id).Err()
}

func (c *productCache) InvalidateAllLists(ctx context.Context) error {
	iter := c.client.Scan(ctx, 0, "product:list:*", 0).Iterator()
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

// ─── Session cache ────────────────────────────────────────────────────────────

const sessionTTL = 7 * 24 * time.Hour

type SessionCache interface {
	SetRefreshToken(ctx context.Context, userID, token string) error
	GetRefreshToken(ctx context.Context, userID string) (string, error)
	DeleteRefreshToken(ctx context.Context, userID string) error
	BlacklistToken(ctx context.Context, token string, ttl time.Duration) error
	IsBlacklisted(ctx context.Context, token string) (bool, error)
}

type sessionCache struct {
	client *redis.Client
}

func NewSessionCache(client *redis.Client) SessionCache {
	return &sessionCache{client: client}
}

func (s *sessionCache) SetRefreshToken(ctx context.Context, userID, token string) error {
	return s.client.Set(ctx, "refresh:"+userID, token, sessionTTL).Err()
}

func (s *sessionCache) GetRefreshToken(ctx context.Context, userID string) (string, error) {
	token, err := s.client.Get(ctx, "refresh:"+userID).Result()
	if err == redis.Nil {
		return "", nil
	}
	return token, err
}

func (s *sessionCache) DeleteRefreshToken(ctx context.Context, userID string) error {
	return s.client.Del(ctx, "refresh:"+userID).Err()
}

func (s *sessionCache) BlacklistToken(ctx context.Context, token string, ttl time.Duration) error {
	return s.client.Set(ctx, "blacklist:"+token, "1", ttl).Err()
}

func (s *sessionCache) IsBlacklisted(ctx context.Context, token string) (bool, error) {
	exists, err := s.client.Exists(ctx, "blacklist:"+token).Result()
	return exists > 0, err
}
