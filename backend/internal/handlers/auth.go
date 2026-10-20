package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"files/internal/loginlimit"
	"files/internal/middleware"
	"files/internal/models"
	"files/internal/sessions"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	db       *sql.DB
	sessions *sessions.Store
	limiter  *loginlimit.Limiter
}

func NewAuthHandler(db *sql.DB, sessionStore *sessions.Store) *AuthHandler {
	return &AuthHandler{db: db, sessions: sessionStore, limiter: loginlimit.New()}
}

// POST /api/v1/auth/login
// Starts a session: sets the session cookie and responds with the user. Too
// many failed attempts from one address, or for one username, are refused
// with 429 for a while (see loginlimit). Failed and refused attempts are
// logged with the client's address, e.g. for fail2ban.
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
		return
	}

	address := c.ClientIP()
	if wait, ok := h.limiter.Begin(address, req.Username); !ok {
		log.Printf("login refused, too many attempts: user %q from %s", req.Username, address)
		c.Header("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		c.JSON(http.StatusTooManyRequests, gin.H{"error": tooManyAttemptsMessage(wait)})
		return
	}

	user, err := h.fetchUser(req.Username)
	if err == nil {
		err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	} else {
		bcrypt.CompareHashAndPassword(unknownUserHash(), []byte(req.Password))
	}
	if err != nil {
		log.Printf("login failed: user %q from %s", req.Username, address)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	h.limiter.Succeeded(address, req.Username)

	token, expires, err := h.sessions.Create(user.ID)
	if err != nil {
		log.Printf("login: starting a session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not start session"})
		return
	}
	middleware.SetSessionCookie(c, token, expires)
	c.JSON(http.StatusOK, gin.H{"user": user.ToResponse()})
}

// unknownUserHash is checked against when the username doesn't exist, so that
// answer takes as long as a wrong password and doesn't reveal which usernames
// exist. Made once, at the cost real passwords are hashed with.
var unknownUserHash = sync.OnceValue(func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte(rand.Text()), bcrypt.DefaultCost)
	if err != nil {
		panic("hashing the unknown-user password: " + err.Error())
	}
	return hash
})

// MinPasswordLength applies to passwords set from now on; existing ones keep working.
const MinPasswordLength = 8

// validateNewPassword checks a password being set (new user, password change
// or reset). Length counts characters, not bytes.
func validateNewPassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}
	return nil
}

// tooManyAttemptsMessage tells the user how long to wait, in whole minutes.
func tooManyAttemptsMessage(wait time.Duration) string {
	minutes := int(math.Ceil(wait.Minutes()))
	if minutes <= 1 {
		return "Too many login attempts. Try again in a minute."
	}
	return fmt.Sprintf("Too many login attempts. Try again in %d minutes.", minutes)
}

// POST /api/v1/auth/logout
// Ends this session on the server and drops the cookie. Works without a live
// session too, so a stale cookie can always be cleared.
func (h *AuthHandler) Logout(c *gin.Context) {
	if token, err := c.Cookie(middleware.SessionCookie); err == nil && token != "" {
		if err := h.sessions.Delete(token); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not end session"})
			return
		}
	}
	middleware.ClearSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

// GET /api/v1/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	userID := c.GetInt64("userID")
	user, err := h.fetchUserByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user.ToResponse())
}

// PUT /api/v1/auth/password
// Also ends the user's other sessions, e.g. one on a lost or shared device.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateNewPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.fetchUserByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.CurrentPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "current password is incorrect"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}

	if _, err := h.db.Exec(`UPDATE users SET password = ? WHERE id = ?`, string(hash), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update password"})
		return
	}
	if err := h.sessions.DeleteForUser(userID, c.GetString("sessionToken")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password updated, but other sessions could not be ended"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password updated"})
}

// PUT /api/v1/auth/profile
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, err := h.db.Exec(`UPDATE users SET display_name = ? WHERE id = ?`, req.DisplayName, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update profile"})
		return
	}

	user, err := h.fetchUserByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch user"})
		return
	}
	c.JSON(http.StatusOK, user.ToResponse())
}

func (h *AuthHandler) fetchUser(username string) (*models.User, error) {
	row := h.db.QueryRow(
		`SELECT id, username, password, root_dir, is_admin, display_name, subtitle_prefs FROM users WHERE username = ?`,
		username,
	)
	return scanUser(row)
}

func (h *AuthHandler) fetchUserByID(id int64) (*models.User, error) {
	row := h.db.QueryRow(
		`SELECT id, username, password, root_dir, is_admin, display_name, subtitle_prefs FROM users WHERE id = ?`,
		id,
	)
	return scanUser(row)
}

func scanUser(row *sql.Row) (*models.User, error) {
	u := &models.User{}
	err := row.Scan(&u.ID, &u.Username, &u.Password, &u.RootDir, &u.IsAdmin, &u.DisplayName, &u.SubtitlePrefsRaw)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// PUT /api/v1/auth/subtitle-prefs
func (h *AuthHandler) UpdateSubtitlePrefs(c *gin.Context) {
	userID := c.GetInt64("userID")

	var prefs models.SubtitlePrefs
	if err := c.ShouldBindJSON(&prefs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if prefs.Color == "" {
		prefs.Color = "#ffffff"
	}
	if prefs.FontSize < 50 || prefs.FontSize > 300 {
		prefs.FontSize = 100
	}
	if prefs.BgOpacity < 0 || prefs.BgOpacity > 1 {
		prefs.BgOpacity = 0.5
	}
	validStyles := map[string]bool{"none": true, "raised": true, "depressed": true, "uniform": true, "drop-shadow": true}
	if !validStyles[prefs.TextStyle] {
		prefs.TextStyle = "drop-shadow"
	}
	if prefs.FontFamily == "" {
		prefs.FontFamily = "sans-serif"
	}

	raw, err := json.Marshal(prefs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not encode prefs"})
		return
	}

	if _, err := h.db.Exec(`UPDATE users SET subtitle_prefs = ? WHERE id = ?`, string(raw), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update subtitle prefs"})
		return
	}

	c.JSON(http.StatusOK, prefs)
}
