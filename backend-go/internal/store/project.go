package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/heycode/backend-go/internal/types"
)

type Project struct {
	ID           string
	ServerID     string
	Name         string
	Cwd          string
	DefaultCli   types.CliKind
	DefaultModel string // 可空
	Rules        string // 可空
	CreatedAt    time.Time
}

type ProjectStore struct {
	db *sql.DB
}

func NewProjectStore(db *sql.DB) *ProjectStore {
	return &ProjectStore{db: db}
}

const projectColumns = `id, server_id, name, cwd, default_cli, default_model, rules, created_at`

func (s *ProjectStore) Create(ctx context.Context, p *Project) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO projects (id, server_id, name, cwd, default_cli, default_model, rules, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.ServerID, p.Name, p.Cwd, string(p.DefaultCli),
		nullableStr(p.DefaultModel), nullableStr(p.Rules), timeToStr(p.CreatedAt),
	)
	return err
}

func (s *ProjectStore) GetByID(ctx context.Context, id string) (*Project, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects WHERE id = ?`, id)
	return scanProject(row)
}

// ListByServer 按 serverID 过滤；serverID 为空时返回全部。按 created_at desc 排序。
func (s *ProjectStore) ListByServer(ctx context.Context, serverID string) ([]*Project, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if serverID == "" {
		rows, err = s.db.QueryContext(ctx, `SELECT `+projectColumns+` FROM projects ORDER BY created_at DESC`)
	} else {
		rows, err = s.db.QueryContext(ctx, `SELECT `+projectColumns+` FROM projects WHERE server_id = ? ORDER BY created_at DESC`, serverID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *ProjectStore) Update(ctx context.Context, p *Project) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE projects SET
	server_id = ?, name = ?, cwd = ?, default_cli = ?, default_model = ?, rules = ?
WHERE id = ?`,
		p.ServerID, p.Name, p.Cwd, string(p.DefaultCli),
		nullableStr(p.DefaultModel), nullableStr(p.Rules), p.ID,
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

func (s *ProjectStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanProject(sc scanner) (*Project, error) {
	var p Project
	var defaultCli string
	var defaultModel, rules, createdAt sql.NullString
	err := sc.Scan(
		&p.ID, &p.ServerID, &p.Name, &p.Cwd, &defaultCli, &defaultModel, &rules, &createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.DefaultCli = types.CliKind(defaultCli)
	if defaultModel.Valid {
		p.DefaultModel = defaultModel.String
	}
	if rules.Valid {
		p.Rules = rules.String
	}
	if createdAt.Valid && createdAt.String != "" {
		t, _ := strToTime(createdAt.String)
		p.CreatedAt = t
	}
	return &p, nil
}
