package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"files/internal/config"
	"files/internal/database"
	"files/internal/handlers"
	"files/internal/middleware"
	"files/internal/sessions"
	"files/internal/thumbs"

	"github.com/gin-gonic/gin"
)

const version = "1.0.0"

func main() {
	cfg := config.Load()
	gin.SetMode(cfg.GinMode)

	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	r := gin.Default()
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		log.Fatalf("TRUSTED_PROXIES: %v", err)
	}
	sessionStore := sessions.NewStore(db)

	// Thumbnail service (non-fatal if it fails — server runs without thumbnails)
	thumbSvc, err := thumbs.New(db, cfg.ThumbCacheDir)
	if err != nil {
		log.Printf("thumbnails: disabled (%v)", err)
		thumbSvc = nil
	}

	// Handlers
	authH := handlers.NewAuthHandler(db, sessionStore)
	filesH := handlers.NewFilesHandler(thumbSvc, cfg.ConvCacheDir)
	archiveH := handlers.NewArchiveHandler()
	foldersH := handlers.NewFoldersHandler(db)
	opsH := handlers.NewFileOpsHandler()
	subsH := handlers.NewSubtitlesHandler()
	thumbH := handlers.NewThumbnailHandler(thumbSvc)
	cacheH := handlers.NewCacheHandler(thumbSvc, cfg.HLSCacheDir, cfg.ConvCacheDir)
	usersH := handlers.NewUsersHandler(db, cfg.UsersRoot, sessionStore)
	hlsH := handlers.NewHLSHandler(cfg.HLSCacheDir)
	infoH := handlers.NewInfoHandler()
	progressH := handlers.NewProgressHandler(db)
	systemH := handlers.NewSystemHandler(version)
	officeH := handlers.NewOfficeHandler(cfg.JWTSecret)
	screenshotH := handlers.NewScreenshotHandler()
	advancedRenameH := handlers.NewAdvancedRenamerHandler(db)

	// Auth middleware (applied per group)
	authMiddleware := middleware.Auth(sessionStore)
	adminOnly := middleware.AdminOnly()

	api := r.Group("/api/v1", middleware.SameOrigin())
	{
		// Public
		api.POST("/auth/login", authH.Login)
		api.POST("/auth/logout", authH.Logout)

		// Authenticated
		authed := api.Group("", authMiddleware, handlers.CloseUserFS())
		{
			authed.GET("/auth/me", authH.Me)
			authed.PUT("/auth/password", authH.ChangePassword)
			authed.PUT("/auth/profile", authH.UpdateProfile)
			authed.PUT("/auth/subtitle-prefs", authH.UpdateSubtitlePrefs)

			// Files
			authed.GET("/files/list", filesH.List)
			authed.GET("/files/stat", filesH.Stat)
			authed.GET("/files/watch", filesH.Watch)
			authed.GET("/files/search", filesH.Search)
			authed.GET("/files/download", filesH.Download)
			authed.POST("/files/download-zip", filesH.DownloadZip)
			authed.GET("/files/stream", filesH.Stream)
			authed.GET("/files/content", filesH.ReadContent)
			authed.PUT("/files/content", filesH.WriteContent)
			authed.GET("/files/hls/playlist", hlsH.Playlist)
			authed.GET("/files/hls/segment", hlsH.Segment)
			authed.POST("/files/archive", archiveH.Contents)
			authed.POST("/files/extract", archiveH.Extract)
			authed.POST("/files/compress", archiveH.Compress)
			authed.DELETE("/files", opsH.Delete)
			authed.POST("/files/copy", opsH.Copy)
			authed.POST("/files/move", opsH.Move)
			authed.POST("/files/upload", opsH.Upload)
			authed.POST("/files/mkdir", opsH.Mkdir)
			authed.POST("/files/create", opsH.Create)
			authed.POST("/files/rename", opsH.Rename)
			authed.POST("/files/screenshot", screenshotH.Take)

			// Advanced renamer
			authed.POST("/rename/preview", advancedRenameH.Preview)
			authed.POST("/rename/apply", advancedRenameH.Apply)
			authed.GET("/rename/presets", advancedRenameH.ListPresets)
			authed.POST("/rename/presets", advancedRenameH.SavePreset)
			authed.DELETE("/rename/presets/:id", advancedRenameH.DeletePreset)
			authed.GET("/rename/folder-template", advancedRenameH.GetFolderTemplate)
			authed.GET("/files/subtitles", subsH.List)
			authed.GET("/files/subtitle", subsH.Stream)
			authed.GET("/files/size", infoH.SizeStream)
			authed.GET("/files/info", infoH.Meta)
			authed.POST("/files/office-token", officeH.GenerateToken)

			// System info
			authed.GET("/system/disk", systemH.DiskSpace)
			authed.GET("/system/version", systemH.Version)

			// Watch progress
			authed.GET("/progress", progressH.Get)
			authed.PUT("/progress", progressH.Save)
			authed.DELETE("/progress", progressH.Delete)

			// Thumbnails
			authed.GET("/thumbnails/get", thumbH.Get)

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
	r.GET("/health", healthHandler(db))

	r.NoRoute(frontendHandler("./public"))

	server := newHTTPServer(":"+cfg.Port, r)
	server.RegisterOnShutdown(filesH.EndStreams)
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	log.Printf("starting on :%s", cfg.Port)
	if err := serveUntilStopped(stop, server, listener, shutdownTimeout); err != nil {
		log.Fatalf("server: %v", err)
	}
	log.Printf("stopped")
}

// shutdownTimeout is how long requests in progress (uploads, extractions)
// get to finish on shutdown. docker-compose.yml's stop_grace_period allows
// a little more before the container is killed.
const shutdownTimeout = 100 * time.Second

// serveUntilStopped serves on listener until stop is done (a stop signal),
// then shuts the server down: no new connections, and requests in progress
// get up to timeout to finish.
func serveUntilStopped(stop context.Context, server *http.Server, listener net.Listener, timeout time.Duration) error {
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	select {
	case err := <-served:
		return err
	case <-stop.Done():
	}
	log.Printf("stopping: waiting up to %v for requests in progress", timeout)
	shutdown, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return server.Shutdown(shutdown)
}

// healthHandler answers /health: 200 while the database answers, 503 otherwise.
func healthHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := db.PingContext(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "database unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// newHTTPServer serves handler on addr. Only receiving a request's headers,
// and idle kept-alive connections, are time-limited — without that, clients
// sending headers a byte at a time can tie up the server. Requests as a whole
// aren't: uploads, downloads, video and the folder watch stream can rightly
// take hours.
func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
}

// frontendHandler serves the built frontend from publicDir: its files, and
// index.html for any other page the browser asks for (client-side routes).
// index.html is never cached, so a deploy's new asset names are picked up at
// once; the files under assets/ have content hashes in their names and are
// cached for good.
func frontendHandler(publicDir string) gin.HandlerFunc {
	indexPath := filepath.Join(publicDir, "index.html")
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}

		cleanPath := strings.TrimPrefix(filepath.Clean("/"+c.Request.URL.Path), "/")
		if cleanPath != "" && cleanPath != "index.html" {
			requested := filepath.Join(publicDir, cleanPath)
			if fi, err := os.Stat(requested); err == nil && !fi.IsDir() {
				if strings.HasPrefix(cleanPath, "assets/") {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				}
				c.File(requested)
				return
			}
		}

		if cleanPath == "" || cleanPath == "index.html" || strings.Contains(c.GetHeader("Accept"), "text/html") {
			c.Header("Cache-Control", "no-cache")
			c.File(indexPath)
			return
		}

		c.Status(http.StatusNotFound)
	}
}
