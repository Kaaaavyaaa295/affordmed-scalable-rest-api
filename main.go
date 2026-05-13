package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/yourusername/affordmed-api/internal/auth"
	"github.com/yourusername/affordmed-api/internal/handlers"
	"github.com/yourusername/affordmed-api/internal/middleware"
	"github.com/yourusername/affordmed-api/internal/repository"
	"github.com/yourusername/affordmed-api/internal/cache"
	"github.com/yourusername/affordmed-api/pkg/database"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load .env file (ignored in production)
	_ = godotenv.Load()

	// Connect to PostgreSQL
	db, err := database.NewPostgresDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Connect to Redis
	redisClient, err := cache.NewRedisClient(os.Getenv("REDIS_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	// Run DB migrations
	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	// Init JWT service
	jwtService := auth.NewJWTService(os.Getenv("JWT_SECRET"))

	// Init repositories
	userRepo     := repository.NewUserRepository(db)
	productRepo  := repository.NewProductRepository(db)
	orderRepo    := repository.NewOrderRepository(db)
	inventoryRepo := repository.NewInventoryRepository(db)

	// Init cache layer
	productCache := cache.NewProductCache(redisClient)
	sessionCache := cache.NewSessionCache(redisClient)

	// Init handlers
	authHandler      := handlers.NewAuthHandler(userRepo, jwtService, sessionCache)
	productHandler   := handlers.NewProductHandler(productRepo, productCache)
	orderHandler     := handlers.NewOrderHandler(orderRepo, inventoryRepo, productCache)
	inventoryHandler := handlers.NewInventoryHandler(inventoryRepo, productCache)
	healthHandler    := handlers.NewHealthHandler(db, redisClient)

	// Setup Gin router
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.Use(middleware.CORS())
	router.Use(middleware.RateLimiter(redisClient))

	// Routes
	v1 := router.Group("/api/v1")
	{
		// Public
		v1.POST("/auth/login", authHandler.Login)
		v1.POST("/auth/register", authHandler.Register)
		v1.POST("/auth/refresh", authHandler.RefreshToken)
		v1.GET("/health", healthHandler.Check)

		// Protected
		protected := v1.Group("/")
		protected.Use(middleware.JWTAuth(jwtService))
		{
			// Products
			protected.GET("/products", productHandler.List)
			protected.GET("/products/:id", productHandler.GetByID)

			// Orders
			protected.POST("/orders", orderHandler.Create)
			protected.GET("/orders", orderHandler.ListByUser)
			protected.GET("/orders/:id", orderHandler.GetByID)

			// Admin only
			admin := protected.Group("/")
			admin.Use(middleware.RequireRole("admin"))
			{
				admin.POST("/products", productHandler.Create)
				admin.PUT("/products/:id", productHandler.Update)
				admin.DELETE("/products/:id", productHandler.Delete)
				admin.PUT("/inventory/:id", inventoryHandler.Update)
				admin.GET("/inventory", inventoryHandler.List)
			}
		}
	}

	// Start server with graceful shutdown
	srv := &http.Server{
		Addr:         ":" + getEnv("PORT", "8080"),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Server starting on port %s", getEnv("PORT", "8080"))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Forced shutdown: %v", err)
	}
	log.Println("Server exited cleanly")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
