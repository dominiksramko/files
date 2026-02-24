package config

import (
	"os"
)

type Config struct {
	Port      string
	JWTSecret string
	DBPath    string
	UsersRoot string
	GinMode   string
}

func Load() *Config {
	return &Config{
		Port:      getEnv("PORT", "8080"),
		JWTSecret: getEnv("JWT_SECRET", "change-me-in-production"),
		DBPath:    getEnv("DB_PATH", "./data/files.db"),
		UsersRoot: getEnv("USERS_ROOT", "./data/users"),
		GinMode:   getEnv("GIN_MODE", "debug"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
