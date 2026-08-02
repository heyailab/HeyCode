package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/heycode/backend-go/internal/types"
)

type Task struct {
	ID          string
	ProjectID   string
	Title       string
	Description string // 可空
	Status      types.TaskStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TaskStore struct {
	db *sql.DB
}

func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db}
}

const taskColumns = `id, project_id, title, description, status, created_at, updated_at`

func (s *TaskStore) Create(ctx context.Context, t *Task) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO tasks (id, project_id, title, description, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.ProjectID, t.Title, nullableStr(t.Description), string(t.Status),
		timeToStr(t.CreatedAt), timeToStr(t.UpdatedAt),
	)
	return err
}

func (s *TaskStore) GetByID(ctx context.Context, id string) (*Task, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
	return scanTask(row)
}

// ListByProject 返回某项目下全部任务，按 created_at desc 排序。
func (s *TaskStore) ListByProject(ctx context.Context, projectID string) ([]*Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *TaskStore) Update(ctx context.Context, t *Task) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE tasks SET
	project_id = ?, title = ?, description = ?, status = ?, updated_at = ?
WHERE id = ?`,
		t.ProjectID, t.Title, nullableStr(t.Description), string(t.Status),
		timeToStr(t.UpdatedAt), t.ID,
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

func (s *TaskStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanTask(sc scanner) (*Task, error) {
	var t Task
	var status string
	var description, createdAt, updatedAt sql.NullString
	err := sc.Scan(
		&t.ID, &t.ProjectID, &t.Title, &description, &status, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if description.Valid {
		t.Description = description.String
	}
	t.Status = types.TaskStatus(status)
	if createdAt.Valid && createdAt.String != "" {
		tt, _ := strToTime(createdAt.String)
		t.CreatedAt = tt
	}
	if updatedAt.Valid && updatedAt.String != "" {
		tt, _ := strToTime(updatedAt.String)
		t.UpdatedAt = tt
	}
	return &t, nil
}
