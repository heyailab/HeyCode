package store

import (
	"context"
	"database/sql"
	"time"
)

// Event 是 events 表的实体。
//
// EventID 每会话独立单调递增（从 1 开始），UNIQUE(session_id, event_id) 保证不重；
// Payload 是 UnifiedEvent 的 JSON 字符串（见 types.MarshalEvent）；
// ID 是自增主键，仅用于内部排序，不暴露给客户端。
type Event struct {
	ID        int64
	SessionID string
	EventID   int64
	Payload   string
	CreatedAt time.Time
}

type EventStore struct {
	db *sql.DB
}

func NewEventStore(db *sql.DB) *EventStore {
	return &EventStore{db: db}
}

// InsertEvent 插入一条事件记录。
// 调用方负责保证 (sessionID, eventID) 唯一（eventbus 的 per-session 锁 + counter）。
func (s *EventStore) Insert(ctx context.Context, e *Event) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO events (session_id, event_id, payload, created_at)
VALUES (?, ?, ?, ?)`,
		e.SessionID, e.EventID, e.Payload, timeToStr(e.CreatedAt),
	)
	return err
}

// MaxEventID 返回某会话当前最大 eventId，无事件返回 0。
// 用于 eventbus counter 懒加载（首次 publish 时从 DB 取 max + 1）。
func (s *EventStore) MaxEventID(ctx context.Context, sessionID string) (int64, error) {
	var maxID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(event_id) FROM events WHERE session_id = ?`, sessionID).Scan(&maxID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	if !maxID.Valid {
		return 0, nil
	}
	return maxID.Int64, nil
}

// ListBySession 按 eventId 升序返回某会话的全部事件。
// since > 0 时只返回 eventId > since 的事件（用于断线重连 replay）。
func (s *EventStore) ListBySession(ctx context.Context, sessionID string, since int64) ([]*Event, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if since > 0 {
		rows, err = s.db.QueryContext(ctx, `
SELECT id, session_id, event_id, payload, created_at FROM events
WHERE session_id = ? AND event_id > ? ORDER BY event_id ASC`, sessionID, since)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT id, session_id, event_id, payload, created_at FROM events
WHERE session_id = ? ORDER BY event_id ASC`, sessionID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Event
	for rows.Next() {
		var e Event
		var createdAtStr string
		if err := rows.Scan(&e.ID, &e.SessionID, &e.EventID, &e.Payload, &createdAtStr); err != nil {
			return nil, err
		}
		t, err := strToTime(createdAtStr)
		if err != nil {
			return nil, err
		}
		e.CreatedAt = t
		out = append(out, &e)
	}
	return out, rows.Err()
}

// CountBySession 返回某会话的事件总数（可选用于诊断）。
func (s *EventStore) CountBySession(ctx context.Context, sessionID string) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id = ?`, sessionID).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}
