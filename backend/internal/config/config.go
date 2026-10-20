package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strings"
)

// DefaultTrustedProxies are the addresses whose X-Forwarded-For / X-Real-IP
// headers are believed for a client's address: loopback and private networks,
// where a reverse proxy (on the host or in another container) normally sits.
// From anyone else those headers are ignored, as any client could send them.
var DefaultTrustedProxies = []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"}

type Config struct {
	Port          string
	JWTSecret     string
	DBPath        string
	UsersRoot     string
	GinMode       string
	ThumbCacheDir string
	HLSCacheDir   string
	ConvCacheDir  string
	// TrustedProxies: addresses or CIDR ranges; empty trusts no proxy.
	TrustedProxies []string
}

func Load() *Config {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = randomHex(32)
		log.Println("WARNING: JWT_SECRET not set — generated a random secret (Office preview links stop working on restart)")
	}

	return &Config{
		Port:           getEnv("PORT", "8080"),
		JWTSecret:      jwtSecret,
		DBPath:         getEnv("DB_PATH", "./data/files.db"),
		UsersRoot:      getEnv("USERS_ROOT", "./data/users"),
		GinMode:        getEnv("GIN_MODE", "debug"),
		ThumbCacheDir:  getEnv("THUMB_CACHE_DIR", "./data/thumbs"),
		HLSCacheDir:    getEnv("HLS_CACHE_DIR", "./data/hls"),
		ConvCacheDir:   getEnv("CONV_CACHE_DIR", "./data/conv"),
		TrustedProxies: getListEnv("TRUSTED_PROXIES", DefaultTrustedProxies),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getListEnv reads a comma-separated list; "none" means an empty one.
func getListEnv(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	switch value {
	case "":
		return fallback
	case "none":
		return nil
	}
	var items []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

func randomHex(byteLen int) string {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		panic("config: failed to generate random bytes: " + err.Error())
	}
	return hex.EncodeToString(b)
}
