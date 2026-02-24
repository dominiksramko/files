package main

import (
	"log"
	"net/http"

	"files/internal/config"
	"files/internal/database"
	"files/internal/handlers"
	"files/internal/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()
	gin.SetMode(cfg.GinMode)

	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	r := gin.Default()

	// CORS — allow Vite dev server in development
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:8080"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Disposition"},
		AllowCredentials: true,
	}))

	// Handlers
	authH := handlers.NewAuthHandler(db, cfg.JWTSecret)
	filesH := handlers.NewFilesHandler()
	archiveH := handlers.NewArchiveHandler()
	usersH := handlers.NewUsersHandler(db, cfg.UsersRoot)

	// Auth middleware (applied per group)
	authMiddleware := middleware.Auth(db, cfg.JWTSecret)
	adminOnly := middleware.AdminOnly()

	api := r.Group("/api/v1")
	{
		// Public
		api.POST("/auth/login", authH.Login)

		// Authenticated
		authed := api.Group("", authMiddleware)
		{
			authed.GET("/auth/me", authH.Me)
			authed.PUT("/auth/password", authH.ChangePassword)

			// Files
			authed.GET("/files/list", filesH.List)
			authed.GET("/files/download", filesH.Download)
			authed.GET("/files/stream", filesH.Stream)
			authed.GET("/files/archive", archiveH.Contents)
			authed.POST("/files/extract", archiveH.Extract)

			// Admin only
			admin := authed.Group("", adminOnly)
			{
				admin.GET("/users", usersH.List)
				admin.POST("/users", usersH.Create)
				admin.PUT("/users/:id", usersH.Update)
				admin.DELETE("/users/:id", usersH.Delete)
			}
		}
	}

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	log.Printf("starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server: %v", err)
	}
}
