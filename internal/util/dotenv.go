package util

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// LoadDotEnv walks from the current working directory up to the filesystem
// root, collecting every `.env` file it finds, and loads them in
// nearest-wins order. Any errors are silently ignored — loading a .env file
// is always best-effort.
func LoadDotEnv() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	var paths []string
	dir := cwd
	for {
		candidate := filepath.Join(dir, ".env")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			paths = append(paths, candidate)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if len(paths) == 0 {
		return
	}

	// godotenv.Load does not override already-set variables, so the first
	// file to define a variable wins. `paths` is ordered nearest-first, which
	// gives nearest-wins semantics.
	_ = godotenv.Load(paths...)
}
