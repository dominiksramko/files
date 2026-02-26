package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	foldersH := handlers.NewFoldersHandler(db)
	opsH := handlers.NewFileOpsHandler()
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
			authed.GET("/files/search", filesH.Search)
			authed.GET("/files/download", filesH.Download)
			authed.GET("/files/stream", filesH.Stream)
			authed.GET("/files/archive", archiveH.Contents)
			authed.POST("/files/extract", archiveH.Extract)
			authed.DELETE("/files", opsH.Delete)
			authed.POST("/files/copy", opsH.Copy)
			authed.POST("/files/move", opsH.Move)
			authed.POST("/files/upload", opsH.Upload)
			authed.POST("/files/mkdir", opsH.Mkdir)
			authed.POST("/files/rename", opsH.Rename)

			// Folder preferences
			authed.GET("/folders/pref", foldersH.GetPref)
			authed.PUT("/folders/pref", foldersH.SetPref)

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

	publicDir := "./public"
	indexPath := filepath.Join(publicDir, "index.html")
	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}

		path := c.Request.URL.Path
		if path == "" {
			path = "/"
		}

		cleanPath := filepath.Clean(path)
		if strings.HasPrefix(cleanPath, "..") {
			c.Status(http.StatusBadRequest)
			return
		}
		cleanPath = strings.TrimPrefix(cleanPath, "/")

		requested := filepath.Join(publicDir, cleanPath)
		if fi, err := os.Stat(requested); err == nil && !fi.IsDir() {
			c.File(requested)
			return
		}

		if strings.Contains(c.GetHeader("Accept"), "text/html") {
			c.File(indexPath)
			return
		}

		c.Status(http.StatusNotFound)
	})

	log.Printf("starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server: %v", err)
	}
}
