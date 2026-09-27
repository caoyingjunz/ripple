package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Addr       string
	DBPath     string
	UploadDir  string
	JWTSecret  string
	MaxRounds  int
	MaxUpload  int64
}

func Load() Config {
	cwd, _ := os.Getwd()
	root := filepath.Clean(filepath.Join(cwd, ".."))
	if filepath.Base(cwd) != "server" {
		root = cwd
	}
	dbPath := env("RIPPLE_DB", filepath.Join(cwd, "data", "ripple.db"))
	upload := env("RIPPLE_UPLOAD", filepath.Join(root, "uploads"))
	if filepath.Base(cwd) == "server" {
		upload = env("RIPPLE_UPLOAD", filepath.Join(cwd, "..", "uploads"))
	}
	return Config{
		Addr:      env("RIPPLE_ADDR", ":8080"),
		DBPath:    dbPath,
		UploadDir: upload,
		JWTSecret: env("RIPPLE_JWT_SECRET", "ripple-dev-secret-change-me"),
		MaxRounds: 6,
		MaxUpload: 2 << 20,
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
