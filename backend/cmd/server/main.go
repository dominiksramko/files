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
	"files/internal/thumbs"

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

	// Thumbnail service (non-fatal if it fails — server runs without thumbnails)
	thumbSvc, err := thumbs.New(db, cfg.ThumbCacheDir)
	if err != nil {
		log.Printf("thumbnails: disabled (%v)", err)
		thumbSvc = nil
	}

	// Handlers
	authH := handlers.NewAuthHandler(db, cfg.JWTSecret)
	filesH := handlers.NewFilesHandler(thumbSvc, cfg.ConvCacheDir)
	archiveH := handlers.NewArchiveHandler()
	foldersH := handlers.NewFoldersHandler(db)
	opsH := handlers.NewFileOpsHandler()
	subsH := handlers.NewSubtitlesHandler()
	thumbH := handlers.NewThumbnailHandler(thumbSvc)
	cacheH := handlers.NewCacheHandler(thumbSvc, cfg.HLSCacheDir, cfg.ConvCacheDir)
	usersH := handlers.NewUsersHandler(db, cfg.UsersRoot)
	hlsH := handlers.NewHLSHandler(cfg.HLSCacheDir)
	infoH := handlers.NewInfoHandler()
	progressH := handlers.NewProgressHandler(db)
	systemH := handlers.NewSystemHandler(cfg.UsersRoot)
	officeH := handlers.NewOfficeHandler(cfg.JWTSecret)

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
			authed.PUT("/auth/profile", authH.UpdateProfile)
			authed.PUT("/auth/subtitle-prefs", authH.UpdateSubtitlePrefs)

			// Files
			authed.GET("/files/list", filesH.List)
			authed.GET("/files/watch", filesH.Watch)
			authed.GET("/files/search", filesH.Search)
			authed.GET("/files/download", filesH.Download)
			authed.POST("/files/download-zip", filesH.DownloadZip)
			authed.GET("/files/stream", filesH.Stream)
			authed.PUT("/files/content", filesH.WriteContent)
			authed.GET("/files/hls/playlist", hlsH.Playlist)
			authed.GET("/files/hls/segment", hlsH.Segment)
			authed.GET("/files/archive", archiveH.Contents)
			authed.POST("/files/extract", archiveH.Extract)
			authed.POST("/files/compress", archiveH.Compress)
			authed.DELETE("/files", opsH.Delete)
			authed.POST("/files/copy", opsH.Copy)
			authed.POST("/files/move", opsH.Move)
			authed.POST("/files/upload", opsH.Upload)
			authed.POST("/files/mkdir", opsH.Mkdir)
			authed.POST("/files/rename", opsH.Rename)
			authed.GET("/files/subtitles", subsH.List)
			authed.GET("/files/subtitle", subsH.Stream)
			authed.GET("/files/size", infoH.SizeStream)
			authed.GET("/files/info", infoH.Meta)
			authed.POST("/files/office-token", officeH.GenerateToken)

			// System info
			authed.GET("/system/disk", systemH.DiskSpace)

			// Watch progress
			authed.GET("/progress", progressH.Get)
			authed.PUT("/progress", progressH.Save)
			authed.DELETE("/progress", progressH.Delete)

			// Thumbnails
			authed.GET("/thumbnails/get", thumbH.Get)
			authed.GET("/thumbnails/stream", thumbH.Stream)

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
				admin.DELETE("/cache", cacheH.Clear)
			}
		}
	}

	// Public office preview (short-lived HMAC token, no auth middleware)
	r.GET("/public/office/:token", officeH.ServeFile)

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
