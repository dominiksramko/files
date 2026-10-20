package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"files/internal/userfs"

	"github.com/gin-gonic/gin"
)

type OfficeHandler struct {
	jwtSecret string
}

func NewOfficeHandler(jwtSecret string) *OfficeHandler {
	return &OfficeHandler{jwtSecret: jwtSecret}
}

type officeTokenPayload struct {
	Path string `json:"path"`
	Root string `json:"root"`
	Exp  int64  `json:"exp"`
}

func (h *OfficeHandler) makeToken(path, root string) (string, error) {
	payload := officeTokenPayload{
		Path: path,
		Root: root,
		Exp:  time.Now().Add(5 * time.Minute).Unix(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, []byte(h.jwtSecret))
	mac.Write([]byte(encoded))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + sig, nil
}

func (h *OfficeHandler) parseToken(token string) (*officeTokenPayload, bool) {
	dot := strings.LastIndex(token, ".")
	if dot < 0 {
		return nil, false
	}
	encoded, sig := token[:dot], token[dot+1:]

	mac := hmac.New(sha256.New, []byte(h.jwtSecret))
	mac.Write([]byte(encoded))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return nil, false
	}

	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, false
	}
	var payload officeTokenPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, false
	}
	if time.Now().Unix() > payload.Exp {
		return nil, false
	}
	return &payload, true
}

// POST /api/v1/files/office-token
func (h *OfficeHandler) GenerateToken(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	root := userRoot(c)
	if _, _, ok := requestFolderName(c, req.Path); !ok {
		return
	}

	token, err := h.makeToken(req.Path, root)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

// GET /public/office/:token
func (h *OfficeHandler) ServeFile(c *gin.Context) {
	payload, ok := h.parseToken(c.Param("token"))
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}

	name, err := userfs.Clean(payload.Path)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	fsys, err := userfs.Open(payload.Root)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer fsys.Close()
	serveUserFile(c, fsys, name, false)
}
