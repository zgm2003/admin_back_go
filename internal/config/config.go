package config

import "os"

type Config struct {
	AppEnv   string
	HTTPAddr string
}

func Load() Config {
	return Config{
		AppEnv:   env("APP_ENV", "local"),
		HTTPAddr: env("HTTP_ADDR", ":8080"),
	}
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
