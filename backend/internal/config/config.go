package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"strings"
)

// DefaultTrustedProxies are the addresses whose X-Forwarded-For / X-Real-IP
// headers are believed for a client's address: loopback and private networks,
// where a reverse proxy (on the host or in another container) normally sits.
// From anyone else those headers are ignored, as any client could send them.
var DefaultTrustedProxies = []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"}

// DefaultUsersRoot is USERS_ROOT when unset (the Docker image sets /data/users).
const DefaultUsersRoot = "./data/users"

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
	// Size limits of the video and conversion caches, in bytes; 0 = none.
	HLSCacheMaxBytes  int64
	ConvCacheMaxBytes int64
}

func Load() *Config {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = randomHex(32)
		log.Println("WARNING: JWT_SECRET not set — generated a random secret (Office preview links stop working on restart)")
	}

	return &Config{
		Port:              getEnv("PORT", "8080"),
		JWTSecret:         jwtSecret,
		DBPath:            getEnv("DB_PATH", "./data/files.db"),
		UsersRoot:         getEnv("USERS_ROOT", DefaultUsersRoot),
		GinMode:           getEnv("GIN_MODE", "debug"),
		ThumbCacheDir:     getEnv("THUMB_CACHE_DIR", "./data/thumbs"),
		HLSCacheDir:       getEnv("HLS_CACHE_DIR", "./data/hls"),
		ConvCacheDir:      getEnv("CONV_CACHE_DIR", "./data/conv"),
		TrustedProxies:    getListEnv("TRUSTED_PROXIES", DefaultTrustedProxies),
		HLSCacheMaxBytes:  getSizeEnv("HLS_CACHE_MAX_GB", 20),
		ConvCacheMaxBytes: getSizeEnv("CONV_CACHE_MAX_GB", 2),
	}
}

// getSizeEnv reads a size in gigabytes (GiB, decimals allowed; 0 = no limit)
// and returns it in bytes. An unreadable value falls back to fallbackGB.
func getSizeEnv(key string, fallbackGB float64) int64 {
	gigabytes := fallbackGB
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed < 0 {
			log.Printf("WARNING: %s=%q is not a size in GB — using %g", key, value, fallbackGB)
		} else {
			gigabytes = parsed
		}
	}
	return int64(gigabytes * (1 << 30))
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
