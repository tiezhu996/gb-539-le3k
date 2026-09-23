package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port      string
	DBDriver  string
	DBDSN     string
	JWTSecret string
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	driver := os.Getenv("DB_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "file:kilncurve?cache=shared"
	}
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-only-change-me"
	}
	return Config{Port: port, DBDriver: driver, DBDSN: dsn, JWTSecret: secret}
}

func IntEnv(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}
