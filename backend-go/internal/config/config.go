// Package config 加载 HeyCode 后端运行配置。
// 配置来源优先级：环境变量 > .env 文件（可选）> 默认值。
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const (
	defaultPort     = 8787
	defaultLogLevel = "info"
	defaultDbURL    = "file:./dev.db"

	// MasterKeyPlaceholder 是 MASTER_KEY 的开发占位符。
	// 出现该值时后端会生成临时内存密钥（重启即失效，仅用于本地调试）。
	MasterKeyPlaceholder = "replace_me_with_32_bytes_hex_string"
)

// Config 是后端运行配置。
type Config struct {
	Port        int    // HTTP 端口，默认 8787
	DatabaseURL string // SQLite DSN，默认 "file:./dev.db"
	MasterKey   string // 32 字节密钥的 hex 字符串（64 字符）；占位符触发 dev 模式
	JwtSecret   string // 保留字段，未来扩展用
	LogLevel    string // debug | info | warn | error，默认 info
	MockCli     bool   // true 时 claude-code 走 MockAdapter，无需 SSH
}

// Load 从环境变量（可选 .env）加载配置。
// .env 文件不存在不视为错误。
func Load() (*Config, error) {
	// .env 可选；忽略不存在错误
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		// 其它错误（如权限问题）也只是警告，不阻断启动
	}

	cfg := &Config{
		Port:        defaultPort,
		DatabaseURL: defaultDbURL,
		MasterKey:   MasterKeyPlaceholder,
		LogLevel:    defaultLogLevel,
		MockCli:     false,
	}

	if v := os.Getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT %q: %w", v, err)
		}
		cfg.Port = p
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("MASTER_KEY"); v != "" {
		cfg.MasterKey = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JwtSecret = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = strings.ToLower(v)
	}
	if v := os.Getenv("MOCK_CLI"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.MockCli = b
		}
	}

	return cfg, nil
}

// IsMasterKeyPlaceholder 判断当前 MasterKey 是否为开发占位符。
func (c *Config) IsMasterKeyPlaceholder() bool {
	return c.MasterKey == MasterKeyPlaceholder
}
