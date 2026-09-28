package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type MySQL struct {
	DSN string `yaml:"dsn"`
}

type Config struct {
	Addr      string `yaml:"addr"`
	MySQL     MySQL  `yaml:"mysql"`
	JWTSecret string `yaml:"jwt_secret"`
	UploadDir string `yaml:"upload_dir"` // 相对仓库根目录
	MaxRounds int    `yaml:"max_rounds"`
	MaxUpload int64  `yaml:"max_upload"`
}

// repoRoot 兼容从 server/ 或仓库根目录启动，统一推导配置文件所在根目录
func repoRoot() string {
	cwd, _ := os.Getwd()
	root := filepath.Clean(filepath.Join(cwd, ".."))
	if filepath.Base(cwd) != "server" {
		root = cwd
	}
	return root
}

// Load 读仓库根目录 config.yaml，环境变量可覆盖；DSN 两处皆空则启动报错
func Load() (Config, error) {
	root := repoRoot()
	var c Config
	data, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return Config{}, err
	}
	if data != nil {
		if err := yaml.Unmarshal(data, &c); err != nil {
			return Config{}, fmt.Errorf("解析 config.yaml: %w", err)
		}
	}
	// env 覆盖（优先级：env > config.yaml）
	if v := env("RIPPLE_ADDR"); v != "" {
		c.Addr = v
	}
	if v := env("RIPPLE_MYSQL_DSN"); v != "" {
		c.MySQL.DSN = v
	}
	if v := env("RIPPLE_UPLOAD"); v != "" {
		c.UploadDir = v
	}
	if v := env("RIPPLE_JWT_SECRET"); v != "" {
		c.JWTSecret = v
	}
	// 缺省值
	if c.Addr == "" {
		c.Addr = ":8080"
	}
	if c.JWTSecret == "" {
		c.JWTSecret = "ripple-dev-secret-change-me"
	}
	if c.UploadDir == "" {
		c.UploadDir = "uploads"
	}
	if c.MaxRounds <= 0 {
		c.MaxRounds = 6
	}
	if c.MaxUpload <= 0 {
		c.MaxUpload = 2 << 20
	}
	if !filepath.IsAbs(c.UploadDir) {
		c.UploadDir = filepath.Join(root, c.UploadDir)
	}
	if c.MySQL.DSN == "" {
		return Config{}, errors.New("MySQL DSN 未配置：请在仓库根目录创建 config.yaml 并填写 mysql.dsn（格式见 README），或设置环境变量 RIPPLE_MYSQL_DSN（优先级：env > config.yaml）")
	}
	return c, nil
}

func env(k string) string {
	return os.Getenv(k)
}
