package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type ProgressHandler struct {
	db *sql.DB
}

func NewProgressHandler(db *sql.DB) *ProgressHandler {
	return &ProgressHandler{db: db}
}

// GET /api/v1/progress?path=
func (h *ProgressHandler) Get(c *gin.Context) {
	userID := c.GetInt64("userID")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	var position, duration float64
	err := h.db.QueryRow(
		`SELECT position, duration FROM watch_progress WHERE user_id = ? AND file_path = ?`,
		userID, path,
	).Scan(&position, &duration)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"position": position, "duration": duration})
}

// PUT /api/v1/progress
func (h *ProgressHandler) Save(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req struct {
		Path     string  `json:"path" binding:"required"`
		Position float64 `json:"position"`
		Duration float64 `json:"duration"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := h.db.Exec(`
		INSERT INTO watch_progress (user_id, file_path, position, duration, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, file_path) DO UPDATE SET
			position   = excluded.position,
			duration   = excluded.duration,
			updated_at = excluded.updated_at
	`, userID, req.Path, req.Position, req.Duration, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "saved"})
}

// DELETE /api/v1/progress?path=
func (h *ProgressHandler) Delete(c *gin.Context) {
	userID := c.GetInt64("userID")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	h.db.Exec(`DELETE FROM watch_progress WHERE user_id = ? AND file_path = ?`, userID, path)
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
