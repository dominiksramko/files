package config

import (
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
	return &Config{
		Port:          getEnv("PORT", "8080"),
		JWTSecret:     getEnv("JWT_SECRET", "change-me-in-production"),
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
