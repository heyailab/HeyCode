package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/heycode/backend-go/internal/ssh"
	"github.com/heycode/backend-go/internal/store"
)

// SnapshotService 处理文件快照查询与回滚（见 SPEC-GO-REWRITE.md §2.3.7 / §2.8.3）。
//
// 回滚依赖远端 git 仓库：
//   - action=create → git clean -f -- <relPath>（method=git-clean）
//   - action=edit/delete → git checkout HEAD -- <relPath>（method=git-checkout）
//   - 非 git 仓库 → method=skip, rolled=false
//   - relPath = 去掉 cwd 前缀后的相对路径
type SnapshotService struct {
	store *store.SnapshotStore
	pool  *ssh.Pool
}

// NewSnapshotService 创建 SnapshotService。
func NewSnapshotService(s *store.SnapshotStore, pool *ssh.Pool) *SnapshotService {
	return &SnapshotService{store: s, pool: pool}
}

// FileSnapshotDTO 是 FileSnapshot 的对外响应 DTO（§2.3.7）。
type FileSnapshotDTO struct {
	ID           string     `json:"id"`
	SessionID    string     `json:"sessionId"`
	Path         string     `json:"path"`
	Action       string     `json:"action"`
	Diff         string     `json:"diff,omitempty"`
	AddedLines   *int       `json:"addedLines,omitempty"`
	RemovedLines *int       `json:"removedLines,omitempty"`
	CreatedAt    string     `json:"createdAt"`
}

// RollbackResult 是单条快照回滚结果（§2.3.7）。
//
// Method ∈ git-checkout / git-clean / skip。
// Rolled=true 表示已执行回滚命令；false 表示跳过（非 git 仓库或命令失败）。
type RollbackResult struct {
	SnapshotID string `json:"snapshotId"`
	Path       string `json:"path"`
	Action     string `json:"action"`
	Rolled     bool   `json:"rolled"`
	Method     string `json:"method"`
	Message    string `json:"message,omitempty"`
}

// RollbackParams 是 POST rollback 端点的 body（§2.3.7）。
type RollbackParams struct {
	ServerID string `json:"serverId"`
	Cwd      string `json:"cwd"`
}

// ListBySession 返回某会话的全部快照（GET /api/sessions/:id/snapshots）。
func (s *SnapshotService) ListBySession(ctx context.Context, sessionID string) ([]*FileSnapshotDTO, error) {
	list, err := s.store.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return snapshotsToDTOs(list), nil
}

// ListByPath 返回某会话+某路径的快照（GET /api/sessions/:id/snapshots/by-path?path=）。
func (s *SnapshotService) ListByPath(ctx context.Context, sessionID, path string) ([]*FileSnapshotDTO, error) {
	if path == "" {
		return nil, errors.New("path is required")
	}
	list, err := s.store.ListByPath(ctx, sessionID, path)
	if err != nil {
		return nil, err
	}
	return snapshotsToDTOs(list), nil
}

// RollbackSnapshot 回滚单条快照（POST /api/snapshots/:snapshotId/rollback）。
func (s *SnapshotService) RollbackSnapshot(ctx context.Context, snapshotID string, p RollbackParams) (*RollbackResult, error) {
	snap, err := s.store.GetByID(ctx, snapshotID)
	if err != nil {
		return nil, mapErr(err)
	}
	return s.rollbackOne(ctx, snap, p.ServerID, p.Cwd)
}

// RollbackSession 回滚某会话的全部快照（POST /api/sessions/:sessionId/rollback）。
// 按 createdAt 倒序逐个回滚（最近变更先回滚）。
func (s *SnapshotService) RollbackSession(ctx context.Context, sessionID string, p RollbackParams) ([]*RollbackResult, error) {
	list, err := s.store.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	// 倒序：最近变更先回滚
	results := make([]*RollbackResult, 0, len(list))
	for i := len(list) - 1; i >= 0; i-- {
		r, rerr := s.rollbackOne(ctx, list[i], p.ServerID, p.Cwd)
		if rerr != nil {
			// 单条失败不阻断整体，记录 skip 结果继续
			results = append(results, &RollbackResult{
				SnapshotID: list[i].ID,
				Path:       list[i].Path,
				Action:     list[i].Action,
				Rolled:     false,
				Method:     "skip",
				Message:    "回滚失败: " + rerr.Error(),
			})
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// rollbackOne 执行单条快照的回滚逻辑（§2.8.3）。
func (s *SnapshotService) rollbackOne(ctx context.Context, snap *store.FileSnapshot, serverID, cwd string) (*RollbackResult, error) {
	result := &RollbackResult{
		SnapshotID: snap.ID,
		Path:       snap.Path,
		Action:     snap.Action,
		Rolled:     false,
		Method:     "skip",
	}

	if s.pool == nil {
		result.Message = "ssh pool not configured"
		return result, nil
	}

	client, err := s.pool.Acquire(ctx, serverID)
	if err != nil {
		return nil, mapSSHErr(err)
	}

	// 检测是否 git 仓库：git rev-parse --is-inside-work-tree
	gres, gerr := ssh.Exec(ctx, client, "git rev-parse --is-inside-work-tree", ssh.ExecOptions{Cwd: cwd, TimeoutMs: 5000})
	if gerr != nil || gres.ExitCode != 0 || !strings.Contains(gres.Stdout, "true") {
		// 非 git 仓库 → skip
		result.Message = "not a git repository"
		return result, nil
	}

	// 计算相对路径：去掉 cwd 前缀
	relPath := relPath(cwd, snap.Path)
	if relPath == "" {
		result.Message = "cannot compute relative path"
		return result, nil
	}
	quotedRel := ssh.ShellQuote(relPath)

	// 按动作选择回滚命令（§2.8.3）
	var cmd string
	var method string
	switch snap.Action {
	case "create":
		// create → git clean -f -- <relPath>（删除未跟踪文件）
		cmd = "git clean -f -- " + quotedRel
		method = "git-clean"
	case "edit", "delete":
		// edit/delete → git checkout HEAD -- <relPath>（恢复到 HEAD 版本）
		cmd = "git checkout HEAD -- " + quotedRel
		method = "git-checkout"
	default:
		result.Message = "unsupported action: " + snap.Action
		return result, nil
	}

	res, err := ssh.Exec(ctx, client, cmd, ssh.ExecOptions{Cwd: cwd, TimeoutMs: 10000})
	if err != nil {
		result.Message = "exec failed: " + err.Error()
		return result, nil
	}
	if res.ExitCode != 0 {
		result.Message = fmt.Sprintf("exit %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))
		return result, nil
	}

	result.Rolled = true
	result.Method = method
	return result, nil
}

// relPath 计算从 cwd 到 absPath 的相对路径。
// absPath 不是 cwd 子路径时返回空串（无法回滚）。
func relPath(cwd, absPath string) string {
	if cwd == "" {
		return absPath
	}
	// 规范化：清理 . / .. / 多余分隔符
	cleanCwd := filepath.Clean(cwd)
	cleanAbs := filepath.Clean(absPath)

	rel, err := filepath.Rel(cleanCwd, cleanAbs)
	if err != nil {
		return ""
	}
	// rel 形如 "subdir/file.txt"；若 cwd 不是 absPath 的前缀，rel 会以 "../" 开头，应拒绝
	if strings.HasPrefix(rel, "..") {
		return ""
	}
	return rel
}

// snapshotsToDTOs 把 store 实体切片转为 DTO 切片。
func snapshotsToDTOs(list []*store.FileSnapshot) []*FileSnapshotDTO {
	out := make([]*FileSnapshotDTO, 0, len(list))
	for _, snap := range list {
		out = append(out, snapshotToDTO(snap))
	}
	return out
}

func snapshotToDTO(snap *store.FileSnapshot) *FileSnapshotDTO {
	return &FileSnapshotDTO{
		ID:           snap.ID,
		SessionID:    snap.SessionID,
		Path:         snap.Path,
		Action:       snap.Action,
		Diff:         snap.Diff,
		AddedLines:   snap.AddedLines,
		RemovedLines: snap.RemovedLines,
		CreatedAt:    snap.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
