package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"files/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type UsersHandler struct {
	db        *sql.DB
	usersRoot string
}

func NewUsersHandler(db *sql.DB, usersRoot string) *UsersHandler {
	return &UsersHandler{db: db, usersRoot: usersRoot}
}

// GET /api/v1/users
func (h *UsersHandler) List(c *gin.Context) {
	rows, err := h.db.Query(`SELECT id, username, root_dir, is_admin FROM users ORDER BY username`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	users := []models.UserResponse{}
	for rows.Next() {
		var u models.UserResponse
		if err := rows.Scan(&u.ID, &u.Username, &u.RootDir, &u.IsAdmin); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		users = append(users, u)
	}
	c.JSON(http.StatusOK, users)
}

// POST /api/v1/users
func (h *UsersHandler) Create(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required,min=4"`
		RootDir  string `json:"root_dir"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}

	rootDir := req.RootDir
	if rootDir == "" {
		rootDir = h.usersRoot + "/" + req.Username
	}

	result, err := h.db.Exec(
		`INSERT INTO users (username, password, root_dir, is_admin) VALUES (?, ?, ?, ?)`,
		req.Username, string(hash), rootDir, req.IsAdmin,
	)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}

	id, _ := result.LastInsertId()
	c.JSON(http.StatusCreated, models.UserResponse{
		ID:       id,
		Username: req.Username,
		RootDir:  rootDir,
		IsAdmin:  req.IsAdmin,
	})
}

// PUT /api/v1/users/:id
func (h *UsersHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Password string `json:"password"`
		RootDir  string `json:"root_dir"`
		IsAdmin  *bool  `json:"is_admin"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Fetch current record
	row := h.db.QueryRow(`SELECT id, username, password, root_dir, is_admin FROM users WHERE id = ?`, id)
	var u models.User
	if err := row.Scan(&u.ID, &u.Username, &u.Password, &u.RootDir, &u.IsAdmin); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	newHash := u.Password
	if req.Password != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
			return
		}
		newHash = string(h)
	}

	if req.RootDir != "" {
		u.RootDir = req.RootDir
	}
	if req.IsAdmin != nil {
		u.IsAdmin = *req.IsAdmin
	}

	_, err = h.db.Exec(
		`UPDATE users SET password = ?, root_dir = ?, is_admin = ? WHERE id = ?`,
		newHash, u.RootDir, u.IsAdmin, id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.UserResponse{
		ID:       u.ID,
		Username: u.Username,
		RootDir:  u.RootDir,
		IsAdmin:  u.IsAdmin,
	})
}

// DELETE /api/v1/users/:id
func (h *UsersHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// Prevent self-deletion
	if id == c.GetInt64("userID") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete yourself"})
		return
	}

	res, err := h.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
