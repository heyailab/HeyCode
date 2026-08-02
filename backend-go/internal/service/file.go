package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/heycode/backend-go/internal/ssh"
)

// FileService 处理远端 SFTP 文件操作（见 SPEC-GO-REWRITE.md §2.3.3）。
//
// 依赖 ssh.Pool 获取 *ssh.Client，再通过 ssh.SFTPClient 操作远端文件系统。
type FileService struct {
	pool *ssh.Pool
}

// NewFileService 创建 FileService。
func NewFileService(pool *ssh.Pool) *FileService {
	return &FileService{pool: pool}
}

// FileListing 是 GET /api/servers/:id/files 的响应。
type FileListing struct {
	Path    string          `json:"path"`
	Entries []ssh.FileEntry `json:"entries"`
}

// FileContent 是 GET/PUT /api/servers/:id/files/content 的响应。
// Content 是文件文本（UTF-8）。
type FileContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int64  `json:"size"`
}

// FileWriteResult 是 PUT /api/servers/:id/files/content 的响应。
type FileWriteResult struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// List 列出远端目录。
// 资源不存在返回 ErrNotFound；其它错误原样返回（handler 决定状态码）。
func (s *FileService) List(ctx context.Context, serverID, dir string) (*FileListing, error) {
	client, err := s.pool.Acquire(ctx, serverID)
	if err != nil {
		return nil, mapSSHErr(err)
	}
	sc, err := ssh.NewSFTPClient(client)
	if err != nil {
		return nil, fmt.Errorf("sftp init: %w", err)
	}
	defer sc.Close() // 注意：SFTPClient.Close 在 sftp.go 里没定义，让我下面补上

	entries, err := sc.ListDir(dir)
	if err != nil {
		return nil, err
	}
	return &FileListing{Path: dir, Entries: entries}, nil
}

// Read 读取远端文件内容。
func (s *FileService) Read(ctx context.Context, serverID, filePath string) (*FileContent, error) {
	client, err := s.pool.Acquire(ctx, serverID)
	if err != nil {
		return nil, mapSSHErr(err)
	}
	sc, err := ssh.NewSFTPClient(client)
	if err != nil {
		return nil, fmt.Errorf("sftp init: %w", err)
	}
	defer sc.Close()

	data, err := sc.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return &FileContent{
		Path:    filePath,
		Content: string(data),
		Size:    int64(len(data)),
	}, nil
}

// Write 写入远端文件（覆盖）。
func (s *FileService) Write(ctx context.Context, serverID, filePath, content string) (*FileWriteResult, error) {
	client, err := s.pool.Acquire(ctx, serverID)
	if err != nil {
		return nil, mapSSHErr(err)
	}
	sc, err := ssh.NewSFTPClient(client)
	if err != nil {
		return nil, fmt.Errorf("sftp init: %w", err)
	}
	defer sc.Close()

	n, err := sc.WriteFile(filePath, []byte(content))
	if err != nil {
		return nil, err
	}
	return &FileWriteResult{Path: filePath, Size: int64(n)}, nil
}

// Delete 删除远端文件或空目录。
func (s *FileService) Delete(ctx context.Context, serverID, filePath string) error {
	client, err := s.pool.Acquire(ctx, serverID)
	if err != nil {
		return mapSSHErr(err)
	}
	sc, err := ssh.NewSFTPClient(client)
	if err != nil {
		return fmt.Errorf("sftp init: %w", err)
	}
	defer sc.Close()

	return sc.Delete(filePath)
}

// mapSSHErr 把 SSH 层错误归一为 service 层错误。
// ErrServerNotFound → ErrNotFound；其它原样返回。
func mapSSHErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ssh.ErrServerNotFound) {
		return ErrNotFound
	}
	return err
}
