package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/heycode/backend-go/internal/types"
)

// Session 是 sessions 表的实体。
//
// TaskID 可空（WS 直接启动会话时为 NULL）；
// CliSessionID 是远端 CLI 返回的会话 ID（claude-code session_id / codex thread_id 等），用于多轮续接；
// EndedAt 在 status=ended/error 时非空。
type Session struct {
	ID           string
	TaskID       *string // 可空
	CliSessionID string  // 可空
	Cli          types.CliKind
	Model        string
	Status       types.SessionStatus
	CreatedAt    time.Time
	EndedAt      *time.Time
}

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

const sessionColumns = `id, task_id, cli_session_id, cli, model, status, created_at, ended_at`

// Create 插入新会话。status 默认 idle（REST 创建）/ running（WS 启动）由调用方设置。
func (s *SessionStore) Create(ctx context.Context, sess *Session) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions (id, task_id, cli_session_id, cli, model, status, created_at, ended_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, nullableStrPtr(sess.TaskID), nullableStr(sess.CliSessionID),
		string(sess.Cli), nullableStr(sess.Model), string(sess.Status),
		timeToStr(sess.CreatedAt), nullableTimeStr(sess.EndedAt),
	)
	return err
}

// GetByID 按 ID 查询会话，未命中返回 ErrNotFound。
func (s *SessionStore) GetByID(ctx context.Context, id string) (*Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id)
	sess, err := scanSession(row)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// ListByTask 返回某任务下全部会话（createdAt desc）。
func (s *SessionStore) ListByTask(ctx context.Context, taskID string) ([]*Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE task_id = ? ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// UpdateStatus 更新会话状态（含可选 endedAt）。
//
// 调用方语义：
//   - startSession: status=running, endedAt=nil
//   - 会话正常结束: status=idle/ended, endedAt=now
//   - 会话出错: status=error, endedAt=now
func (s *SessionStore) UpdateStatus(ctx context.Context, id string, status types.SessionStatus, endedAt *time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE sessions SET status = ?, ended_at = ? WHERE id = ?`,
		string(status), nullableTimeStr(endedAt), id)
	if err != nil {
		return err
	}
	return checkAffected(res)
}

// UpdateCliSessionID 在收到 session.init 后写回远端 CLI 会话 ID（用于多轮续接）。
func (s *SessionStore) UpdateCliSessionID(ctx context.Context, id, cliSessionID string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE sessions SET cli_session_id = ? WHERE id = ?`,
		nullableStr(cliSessionID), id)
	if err != nil {
		return err
	}
	return checkAffected(res)
}

// Delete 删除会话（events/file_snapshots 通过 ON DELETE CASCADE 级联删除）。
func (s *SessionStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return checkAffected(res)
}

// ---- 辅助 ----

// nullableStrPtr 把 *string 转为可写入 DB 的值（nil → NULL，空串 → NULL，否则原值）。
func nullableStrPtr(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func scanSession(sc scanner) (*Session, error) {
	var (
		sess         Session
		taskID       sql.NullString
		cliSessionID sql.NullString
		model        sql.NullString
		endedAt      sql.NullString
		createdAtStr string
		statusStr    string
		cliStr       string
	)
	if err := sc.Scan(&sess.ID, &taskID, &cliSessionID, &cliStr, &model, &statusStr, &createdAtStr, &endedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if taskID.Valid {
		t := taskID.String
		sess.TaskID = &t
	}
	sess.CliSessionID = cliSessionID.String
	sess.Cli = types.CliKind(cliStr)
	sess.Model = model.String
	sess.Status = types.NormalizeSessionStatus(statusStr)
	created, err := strToTime(createdAtStr)
	if err != nil {
		return nil, err
	}
	sess.CreatedAt = created
	sess.EndedAt = nullTime(endedAt)
	return &sess, nil
}

// checkAffected 校验 Exec 影响行数，未命中返回 ErrNotFound。
func checkAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
