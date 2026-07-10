package config

import (
	"os"
)

type Config struct {
	Environment string
	Port        string
	DatabaseURL string
	RabbitMQURL string
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func Load() *Config {
	return &Config{
		Environment: getEnv("ENV", "development"),
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://flowscale:password@localhost:5432/flowscale?sslmode=disable"),
		RabbitMQURL: getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
	}
}
