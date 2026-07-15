package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type lookupEnv func(string) (string, bool)

func osLookup(key string) (string, bool) { return os.LookupEnv(key) }

func envText(lookup lookupEnv, key, fallback string) string {
	if key == "APP_ENV" {
		return envAppEnvironment(lookup, fallback)
	}
	value, ok := lookup(key)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func envAppEnvironment(lookup lookupEnv, fallback string) string {
	value, ok := lookup("APP_ENV")
	if !ok {
		return fallback
	}
	return strings.TrimSpace(value)
}

func envOpaque(lookup lookupEnv, key, fallback string) string {
	value, ok := lookup(key)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func envInteger(lookup lookupEnv, key string, fallback int, positive bool) (int, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s: parse integer: %w", key, err)
	}
	if positive && value <= 0 {
		return 0, fmt.Errorf("%s: must be greater than zero", key)
	}
	if !positive && value < 0 {
		return 0, fmt.Errorf("%s: must not be negative", key)
	}
	return value, nil
}

func envBoolean(lookup lookupEnv, key string, fallback bool) (bool, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("%s: parse boolean: %w", key, err)
	}
	return value, nil
}

func envPeriod(lookup lookupEnv, key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s: parse duration: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s: must be greater than zero", key)
	}
	return value, nil
}

func envList(lookup lookupEnv, key string, fallback []string) []string {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return append([]string(nil), fallback...)
	}
	out := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
