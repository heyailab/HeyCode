// Package db 负责 SQLite 连接管理与 goose 迁移。
//
// 使用 modernc.org/sqlite（纯 Go，无 CGO），便于交叉编译。
// 迁移 SQL 通过 go:embed 内嵌到二进制中，启动时自动执行 goose.Up。
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // 注册 "sqlite" 驱动
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// Open 打开 SQLite 数据库并执行迁移。
//
// databaseURL 接受以下形式：
//   - "file:./dev.db"
//   - "./dev.db"（自动转 file: URL）
//   - ":memory:"
//   - "file::memory:?cache=shared"
//
// 启用 WAL 模式与外键约束。
func Open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	dsn := normalizeDSN(databaseURL)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite 写入串行：单连接避免连接池内的写锁竞争（HeyCode 单用户自托管场景足够）
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(0)

	// 启动时执行一次 PRAGMA 校验
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set foreign_keys pragma: %w", err)
	}

	// 跑迁移
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		db.Close()
		return nil, fmt.Errorf("goose set dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		db.Close()
		return nil, fmt.Errorf("goose up: %w", err)
	}

	return db, nil
}

// normalizeDSN 把用户传入的 DATABASE_URL 归一化为 modernc.org/sqlite 接受的 DSN。
//
// modernc.org/sqlite 支持 file: URI 形式，可在 query 中带 _pragma 参数（每连接生效）。
// 我们借此把 foreign_keys / journal_mode 写进 DSN，避免连接池每次都 Exec PRAGMA。
// defaultPragmas 是每连接默认 PRAGMA：
//   - foreign_keys(1)：开启外键级联
//   - busy_timeout(5000)：锁竞争时等待 5s 而非立即失败
//
// journal_mode 由调用分支单独指定（WAL 用于文件库，MEMORY 用于内存库）。
const pragmaForeignBusy = "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"

func normalizeDSN(s string) string {
	if s == "" {
		return "file:./dev.db?" + pragmaForeignBusy + "&_pragma=journal_mode(WAL)"
	}
	if s == ":memory:" {
		return "file::memory:?cache=shared&" + pragmaForeignBusy + "&_pragma=journal_mode(MEMORY)"
	}
	if strings.HasPrefix(s, "file:") {
		if !strings.Contains(s, "_pragma=") {
			sep := "?"
			if strings.Contains(s, "?") {
				sep = "&"
			}
			return s + sep + pragmaForeignBusy + "&_pragma=journal_mode(WAL)"
		}
		return s
	}
	abs, err := filepath.Abs(s)
	if err != nil {
		abs = s
	}
	return "file:" + url.PathEscape(abs) + "?" + pragmaForeignBusy + "&_pragma=journal_mode(WAL)"
}
