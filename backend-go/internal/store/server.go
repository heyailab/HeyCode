package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/heycode/backend-go/internal/types"
)

// Server 是 servers 表的实体。
// EncryptedAuth 是 AES-256-GCM 密文的 JSON 字符串 {iv, tag, cipherText}，绝不外泄。
type Server struct {
	ID            string
	Name          string
	Host          string
	Port          int
	Username      string
	AuthKind      types.SshAuthKind
	EncryptedAuth string
	LastStatus    types.ServerStatus
	LastCheckedAt *time.Time
	CreatedAt     time.Time
}

type ServerStore struct {
	db *sql.DB
}

func NewServerStore(db *sql.DB) *ServerStore {
	return &ServerStore{db: db}
}

const serverColumns = `id, name, host, port, username, auth_kind, encrypted_auth, last_status, last_checked_at, created_at`

func (s *ServerStore) Create(ctx context.Context, sv *Server) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO servers (id, name, host, port, username, auth_kind, encrypted_auth, last_status, last_checked_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sv.ID, sv.Name, sv.Host, sv.Port, sv.Username, string(sv.AuthKind), sv.EncryptedAuth,
		string(sv.LastStatus), nullableTimeStr(sv.LastCheckedAt), timeToStr(sv.CreatedAt),
	)
	return err
}

func (s *ServerStore) GetByID(ctx context.Context, id string) (*Server, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+serverColumns+` FROM servers WHERE id = ?`, id)
	return scanServer(row)
}

// List 按 created_at desc 返回全部服务器。
func (s *ServerStore) List(ctx context.Context) ([]*Server, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+serverColumns+` FROM servers ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Server
	for rows.Next() {
		sv, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sv)
	}
	return out, rows.Err()
}

// Update 全量更新（除 id 与 created_at 外）。service 层负责合并 PATCH 字段。
func (s *ServerStore) Update(ctx context.Context, sv *Server) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE servers SET
	name = ?, host = ?, port = ?, username = ?, auth_kind = ?, encrypted_auth = ?,
	last_status = ?, last_checked_at = ?
WHERE id = ?`,
		sv.Name, sv.Host, sv.Port, sv.Username, string(sv.AuthKind), sv.EncryptedAuth,
		string(sv.LastStatus), nullableTimeStr(sv.LastCheckedAt), sv.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateStatus 仅更新连通性测试结果字段。
func (s *ServerStore) UpdateStatus(ctx context.Context, id string, status types.ServerStatus, checkedAt *time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE servers SET last_status = ?, last_checked_at = ? WHERE id = ?`,
		string(status), nullableTimeStr(checkedAt), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *ServerStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanServer(sc scanner) (*Server, error) {
	var sv Server
	var authKind, lastStatus, createdAt string
	var lastCheckedAt sql.NullString
	err := sc.Scan(
		&sv.ID, &sv.Name, &sv.Host, &sv.Port, &sv.Username, &authKind, &sv.EncryptedAuth,
		&lastStatus, &lastCheckedAt, &createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	sv.AuthKind = types.SshAuthKind(authKind)
	sv.LastStatus = types.ServerStatus(lastStatus)
	sv.LastCheckedAt = nullTime(lastCheckedAt)
	if createdAt != "" {
		t, _ := strToTime(createdAt)
		sv.CreatedAt = t
	}
	return &sv, nil
}
