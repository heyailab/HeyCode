// Package store 是数据访问层，每个实体一个文件，直接操作 database/sql。
//
// 时间字段在 DB 中以 ISO8601/RFC3339Nano 字符串存储；store 层负责 string ↔ time.Time 转换。
// 命名类型（如 SshAuthKind）通过临时 string 变量中转 Scan，避免反射兼容性问题。
package store

import (
	"database/sql"
	"time"
)

// ErrNotFound 表示按 ID 查询未命中。
// store 层把 sql.ErrNoRows 统一转换为 ErrNotFound，便于 service/handler 层判断。
var ErrNotFound = errNotFound{}

type errNotFound struct{}

func (errNotFound) Error() string { return "record not found" }

// scanner 兼容 *sql.Row 与 *sql.Rows。
type scanner interface {
	Scan(dest ...any) error
}

// timeToStr 把 time.Time 序列化为 UTC RFC3339Nano 字符串；零值返回空串。
func timeToStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// strToTime 解析 RFC3339Nano 字符串为 time.Time；空串返回零值。
func strToTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

// nullableTimeStr 把 *time.Time 转换为可写入 DB 的值（nil 或零值 → NULL）。
func nullableTimeStr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return timeToStr(*t)
}

// nullableStr 把字符串转换为可写入 DB 的值（空串 → NULL）。
func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullTime 从 sql.NullString 解析 *time.Time。
func nullTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, ns.String)
	if err != nil {
		return nil
	}
	return &t
}
