package handlers

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"files/internal/thumbs"
)

// thumbnailThreshold is the minimum fraction of files that must be thumbnail-capable
// for a folder to default to grid view.
const thumbnailThreshold = 0.20

type FoldersHandler struct {
	db *sql.DB
}

func NewFoldersHandler(db *sql.DB) *FoldersHandler {
	return &FoldersHandler{db: db}
}

// GET /api/v1/folders/pref?path=
// Returns the stored view_mode for the folder. If none is stored, auto-detects
// based on thumbnail content (≥20% thumbnail files → grid, else list),
// saves that preference, and returns it.
func (h *FoldersHandler) GetPref(c *gin.Context) {
	userID := c.GetInt64("userID")
	rel := c.Query("path")

	// Return existing preference if stored
	var viewMode string
	err := h.db.QueryRow(
		`SELECT view_mode FROM folder_prefs WHERE user_id = ? AND path = ?`,
		userID, rel,
	).Scan(&viewMode)
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"view_mode": viewMode})
		return
	}
	if err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// No preference stored — auto-detect from directory contents
	root := userRoot(c)
	abs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var persist bool
	viewMode, persist = autoDetectViewMode(abs)

	// Only persist when the folder has files — empty folders should re-detect on next visit
	if persist {
		//nolint:errcheck
		h.db.Exec(
			`INSERT INTO folder_prefs (user_id, path, view_mode) VALUES (?, ?, ?)
			 ON CONFLICT(user_id, path) DO UPDATE SET view_mode = excluded.view_mode`,
			userID, rel, viewMode,
		)
	}

	c.JSON(http.StatusOK, gin.H{"view_mode": viewMode})
}

// PUT /api/v1/folders/pref
// Upserts the view_mode preference for a folder (explicit user choice).
func (h *FoldersHandler) SetPref(c *gin.Context) {
	userID := c.GetInt64("userID")

	var body struct {
		Path     string `json:"path"`
		ViewMode string `json:"view_mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if body.ViewMode != "grid" && body.ViewMode != "list" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "view_mode must be 'grid' or 'list'"})
		return
	}

	_, err := h.db.Exec(
		`INSERT INTO folder_prefs (user_id, path, view_mode) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, path) DO UPDATE SET view_mode = excluded.view_mode`,
		userID, body.Path, body.ViewMode,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"view_mode": body.ViewMode})
}

// autoDetectViewMode returns the detected view mode and whether it should be
// persisted. Empty folders return ("list", false) so they re-detect on next visit.
func autoDetectViewMode(dir string) (mode string, persist bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "list", false
	}

	var totalFiles, thumbFiles int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		totalFiles++
		if thumbs.IsThumbnailable(mimeByExt(filepath.Ext(e.Name()))) {
			thumbFiles++
		}
	}

	if totalFiles == 0 {
		return "list", false
	}
	if float64(thumbFiles)/float64(totalFiles) >= thumbnailThreshold {
		return "grid", true
	}
	return "list", true
}
