package store

import (
	"context"
	"database/sql"
	"time"
)

// FileSnapshot 是 file_snapshots 表的实体。
//
// 由 eventbus 在收到 file.change 事件时同步写入（见 §2.4.4）；
// action ∈ create | edit | delete；diff/addedLines/removedLines 可空。
type FileSnapshot struct {
	ID           string
	SessionID    string
	Path         string
	Action       string
	Diff         string
	AddedLines   *int
	RemovedLines *int
	CreatedAt    time.Time
}

type SnapshotStore struct {
	db *sql.DB
}

func NewSnapshotStore(db *sql.DB) *SnapshotStore {
	return &SnapshotStore{db: db}
}

// Insert 写入一条文件快照。ID 由调用方生成（cuid）。
func (s *SnapshotStore) Insert(ctx context.Context, snap *FileSnapshot) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO file_snapshots (id, session_id, path, action, diff, added_lines, removed_lines, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.SessionID, snap.Path, snap.Action, nullableStr(snap.Diff),
		nullableIntPtr(snap.AddedLines), nullableIntPtr(snap.RemovedLines),
		timeToStr(snap.CreatedAt),
	)
	return err
}

// ListBySession 按 createdAt 升序返回某会话的全部快照。
func (s *SnapshotStore) ListBySession(ctx context.Context, sessionID string) ([]*FileSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, path, action, diff, added_lines, removed_lines, created_at
FROM file_snapshots WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSnapshots(rows)
}

// ListByPath 返回某会话 + 某路径的全部快照（按 createdAt 升序）。
// 用于 GET /api/sessions/:id/snapshots/by-path?path=...
func (s *SnapshotStore) ListByPath(ctx context.Context, sessionID, path string) ([]*FileSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, path, action, diff, added_lines, removed_lines, created_at
FROM file_snapshots WHERE session_id = ? AND path = ? ORDER BY created_at ASC`, sessionID, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSnapshots(rows)
}

// GetByID 按 ID 查询单条快照（用于 rollback）。
func (s *SnapshotStore) GetByID(ctx context.Context, id string) (*FileSnapshot, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, session_id, path, action, diff, added_lines, removed_lines, created_at
FROM file_snapshots WHERE id = ?`, id)
	var snap FileSnapshot
	var diff sql.NullString
	var added, removed sql.NullInt64
	var createdAtStr string
	if err := row.Scan(&snap.ID, &snap.SessionID, &snap.Path, &snap.Action, &diff, &added, &removed, &createdAtStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	snap.Diff = diff.String
	snap.AddedLines = nullIntToPtr(added)
	snap.RemovedLines = nullIntToPtr(removed)
	t, err := strToTime(createdAtStr)
	if err != nil {
		return nil, err
	}
	snap.CreatedAt = t
	return &snap, nil
}

// ---- 辅助 ----

func nullableIntPtr(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

func nullIntToPtr(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	v := int(ni.Int64)
	return &v
}

func scanSnapshots(rows *sql.Rows) ([]*FileSnapshot, error) {
	var out []*FileSnapshot
	for rows.Next() {
		var snap FileSnapshot
		var diff sql.NullString
		var added, removed sql.NullInt64
		var createdAtStr string
		if err := rows.Scan(&snap.ID, &snap.SessionID, &snap.Path, &snap.Action, &diff, &added, &removed, &createdAtStr); err != nil {
			return nil, err
		}
		snap.Diff = diff.String
		snap.AddedLines = nullIntToPtr(added)
		snap.RemovedLines = nullIntToPtr(removed)
		t, err := strToTime(createdAtStr)
		if err != nil {
			return nil, err
		}
		snap.CreatedAt = t
		out = append(out, &snap)
	}
	return out, rows.Err()
}
