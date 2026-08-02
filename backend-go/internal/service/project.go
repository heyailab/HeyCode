package service

import (
	"context"
	"errors"

	"github.com/heycode/backend-go/internal/store"
	"github.com/heycode/backend-go/internal/types"
)

type ProjectService struct {
	store *store.ProjectStore
}

func NewProjectService(s *store.ProjectStore) *ProjectService {
	return &ProjectService{store: s}
}

// CreateProjectParams 是 POST /api/projects 的入参。
type CreateProjectParams struct {
	ServerID     string  `json:"serverId"`
	Name         string  `json:"name"`
	Cwd          string  `json:"cwd"`
	DefaultCli   string  `json:"defaultCli"`
	DefaultModel string  `json:"defaultModel,omitempty"`
	Rules        string  `json:"rules,omitempty"`
}

// UpdateProjectParams 是 PATCH /api/projects/:id 的入参。所有字段可选。
type UpdateProjectParams struct {
	ServerID     *string `json:"serverId,omitempty"`
	Name         *string `json:"name,omitempty"`
	Cwd          *string `json:"cwd,omitempty"`
	DefaultCli   *string `json:"defaultCli,omitempty"`
	DefaultModel *string `json:"defaultModel,omitempty"`
	Rules        *string `json:"rules,omitempty"`
}

func (s *ProjectService) Create(ctx context.Context, p CreateProjectParams) (*types.ProjectDTO, error) {
	if p.ServerID == "" {
		return nil, errors.New("serverId is required")
	}
	proj := &store.Project{
		ID:           newID(),
		ServerID:     p.ServerID,
		Name:         p.Name,
		Cwd:          p.Cwd,
		DefaultCli:   types.CliKind(p.DefaultCli),
		DefaultModel: p.DefaultModel,
		Rules:        p.Rules,
		CreatedAt:    nowUTC(),
	}
	if err := s.store.Create(ctx, proj); err != nil {
		return nil, err
	}
	return projectToDTO(proj), nil
}

func (s *ProjectService) Get(ctx context.Context, id string) (*types.ProjectDTO, error) {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, mapErr(err)
	}
	return projectToDTO(p), nil
}

// List 按 serverID 过滤；serverID 为空时返回全部。
func (s *ProjectService) List(ctx context.Context, serverID string) ([]*types.ProjectDTO, error) {
	list, err := s.store.ListByServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	out := make([]*types.ProjectDTO, 0, len(list))
	for _, p := range list {
		out = append(out, projectToDTO(p))
	}
	return out, nil
}

func (s *ProjectService) Update(ctx context.Context, id string, p UpdateProjectParams) (*types.ProjectDTO, error) {
	proj, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, mapErr(err)
	}
	if p.ServerID != nil {
		proj.ServerID = *p.ServerID
	}
	if p.Name != nil {
		proj.Name = *p.Name
	}
	if p.Cwd != nil {
		proj.Cwd = *p.Cwd
	}
	if p.DefaultCli != nil {
		proj.DefaultCli = types.CliKind(*p.DefaultCli)
	}
	if p.DefaultModel != nil {
		proj.DefaultModel = *p.DefaultModel
	}
	if p.Rules != nil {
		proj.Rules = *p.Rules
	}
	if err := s.store.Update(ctx, proj); err != nil {
		return nil, mapErr(err)
	}
	return projectToDTO(proj), nil
}

func (s *ProjectService) Delete(ctx context.Context, id string) error {
	if err := s.store.Delete(ctx, id); err != nil {
		return mapErr(err)
	}
	return nil
}

func projectToDTO(p *store.Project) *types.ProjectDTO {
	return &types.ProjectDTO{
		ID:           p.ID,
		ServerID:     p.ServerID,
		Name:         p.Name,
		Cwd:          p.Cwd,
		DefaultCli:   p.DefaultCli,
		DefaultModel: p.DefaultModel,
		Rules:        p.Rules,
		CreatedAt:    p.CreatedAt,
	}
}
