package config

import (
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	Addr      string
	MySQLDSN  string
	UploadDir string
	JWTSecret string
	MaxRounds int
	MaxUpload int64
}

// Load 读取配置；MySQL DSN 只走环境变量，缺失时启动即报错（决策②）
func Load() (Config, error) {
	cwd, _ := os.Getwd()
	root := filepath.Clean(filepath.Join(cwd, ".."))
	if filepath.Base(cwd) != "server" {
		root = cwd
	}
	dsn := os.Getenv("RIPPLE_MYSQL_DSN")
	if dsn == "" {
		return Config{}, errors.New("RIPPLE_MYSQL_DSN 未设置：请先 export RIPPLE_MYSQL_DSN='root:<密码>@tcp(peng:3306)/ripple?charset=utf8mb4&multiStatements=true'")
	}
	return Config{
		Addr:      env("RIPPLE_ADDR", ":8080"),
		MySQLDSN:  dsn,
		UploadDir: env("RIPPLE_UPLOAD", filepath.Join(root, "uploads")),
		JWTSecret: env("RIPPLE_JWT_SECRET", "ripple-dev-secret-change-me"),
		MaxRounds: 6,
		MaxUpload: 2 << 20,
	}, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
