package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

func LoadDotEnv(filenames ...string) error {
	if len(filenames) == 0 {
		filenames = []string{".env"}
	}

	if err := godotenv.Load(filenames...); err != nil {
		if isOnlyMissingEnvFile(err) {
			return nil
		}
		return err
	}
	return nil
}

func isOnlyMissingEnvFile(err error) bool {
	if err == nil {
		return false
	}

	var pathErr *os.PathError
	return errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist)
}
