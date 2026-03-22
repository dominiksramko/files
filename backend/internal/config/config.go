package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
)

type Config struct {
	Port          string
	JWTSecret     string
	DBPath        string
	UsersRoot     string
	GinMode       string
	ThumbCacheDir string
	HLSCacheDir   string
	ConvCacheDir  string
}

func Load() *Config {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = randomHex(32)
		log.Println("WARNING: JWT_SECRET not set — generated a random secret (all sessions will be invalidated on restart)")
	}

	return &Config{
		Port:          getEnv("PORT", "8080"),
		JWTSecret:     jwtSecret,
		DBPath:        getEnv("DB_PATH", "./data/files.db"),
		UsersRoot:     getEnv("USERS_ROOT", "./data/users"),
		GinMode:       getEnv("GIN_MODE", "debug"),
		ThumbCacheDir: getEnv("THUMB_CACHE_DIR", "./data/thumbs"),
		HLSCacheDir:   getEnv("HLS_CACHE_DIR", "./data/hls"),
		ConvCacheDir:  getEnv("CONV_CACHE_DIR", "./data/conv"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func randomHex(byteLen int) string {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		panic("config: failed to generate random bytes: " + err.Error())
	}
	return hex.EncodeToString(b)
}
