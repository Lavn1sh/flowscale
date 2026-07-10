package config

import (
	"os"
)

type Config struct {
	DatabaseURL string
	Environment string
	Port        string
}

func Load() *Config {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://flowscale:password@localhost:5432/flowscale?sslmode=disable"
	}

	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "development"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return &Config{
		DatabaseURL: dbURL,
		Environment: env,
		Port:        port,
	}
}
