package service

import (
	"context"
	"errors"

	"github.com/heycode/backend-go/internal/store"
	"github.com/heycode/backend-go/internal/types"
)

type TaskService struct {
	store *store.TaskStore
}

func NewTaskService(s *store.TaskStore) *TaskService {
	return &TaskService{store: s}
}

// CreateTaskParams 是 POST /api/tasks 的入参。
type CreateTaskParams struct {
	ProjectID   string `json:"projectId"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// UpdateTaskParams 是 PATCH /api/tasks/:id 的入参。所有字段可选。
// Description 显式区分 nil（不改）与 ""（清空）。
type UpdateTaskParams struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
}

func (s *TaskService) Create(ctx context.Context, p CreateTaskParams) (*types.TaskDTO, error) {
	if p.ProjectID == "" {
		return nil, errors.New("projectId is required")
	}
	now := nowUTC()
	t := &store.Task{
		ID:          newID(),
		ProjectID:   p.ProjectID,
		Title:       p.Title,
		Description: p.Description,
		Status:      types.TaskStatusPlanned,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.Create(ctx, t); err != nil {
		return nil, err
	}
	return taskToDTO(t), nil
}

func (s *TaskService) Get(ctx context.Context, id string) (*types.TaskDTO, error) {
	t, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, mapErr(err)
	}
	return taskToDTO(t), nil
}

// ListByProject 返回某项目下全部任务。
func (s *TaskService) ListByProject(ctx context.Context, projectID string) ([]*types.TaskDTO, error) {
	list, err := s.store.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]*types.TaskDTO, 0, len(list))
	for _, t := range list {
		out = append(out, taskToDTO(t))
	}
	return out, nil
}

func (s *TaskService) Update(ctx context.Context, id string, p UpdateTaskParams) (*types.TaskDTO, error) {
	t, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, mapErr(err)
	}
	if p.Title != nil {
		t.Title = *p.Title
	}
	if p.Description != nil {
		t.Description = *p.Description
	}
	if p.Status != nil {
		t.Status = types.NormalizeTaskStatus(*p.Status)
	}
	t.UpdatedAt = nowUTC()
	if err := s.store.Update(ctx, t); err != nil {
		return nil, mapErr(err)
	}
	return taskToDTO(t), nil
}

func (s *TaskService) Delete(ctx context.Context, id string) error {
	if err := s.store.Delete(ctx, id); err != nil {
		return mapErr(err)
	}
	return nil
}

func taskToDTO(t *store.Task) *types.TaskDTO {
	return &types.TaskDTO{
		ID:          t.ID,
		ProjectID:   t.ProjectID,
		Title:       t.Title,
		Description: t.Description,
		Status:      t.Status,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}
