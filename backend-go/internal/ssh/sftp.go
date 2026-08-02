package ssh

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"time"

	"github.com/pkg/sftp"
_ssh "golang.org/x/crypto/ssh"
)

// FileEntry 是 SFTP 目录项的统一表示（见 SPEC-GO-REWRITE.md §2.3.3 FileEntry）。
//
// Path 是绝对路径；ModifiedAt 是 ISO8601 字符串。
type FileEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsDir      bool   `json:"isDir"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modifiedAt"`
}

// SFTPClient 包装 sftp.Client，提供 listdir/read/write/delete/stat/mkdir。
//
// 每次操作都新建 sftp.Client（轻量；底层 *ssh.Client 由 Pool 复用）。
// 不缓存 sftp.Client 是为了避免并发使用的额外同步开销。
type SFTPClient struct {
	client *_ssh.Client
}

// NewSFTPClient 在已有 ssh.Client 上创建 SFTP 客户端。
func NewSFTPClient(client *_ssh.Client) (*SFTPClient, error) {
	// 提前校验：sftp.NewClient 内部会 NewSession 跑 subsystem，失败时给出明确错误
	if client == nil {
		return nil, errors.New("ssh client is nil")
	}
	return &SFTPClient{client: client}, nil
}

// Close 释放 SFTP 客户端。
//
// 注意：SFTPClient 每次操作内部新建 sftp.Client 并立即关闭，
// 因此本方法为 no-op，仅用于满足 io.Closer 习惯与 defer 语法。
func (s *SFTPClient) Close() error { return nil }

// sftpClient 内部辅助：建立 sftp.Client。
func (s *SFTPClient) sftpClient() (*sftp.Client, error) {
	return sftp.NewClient(s.client)
}

// ListDir 列出目录下所有条目（不递归）。
// 按 isDir 优先、name 字母序排序。
func (s *SFTPClient) ListDir(dir string) ([]FileEntry, error) {
	sc, err := s.sftpClient()
	if err != nil {
		return nil, fmt.Errorf("sftp new: %w", err)
	}
	defer sc.Close()

	infos, err := sc.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("sftp readdir %s: %w", dir, err)
	}

	entries := make([]FileEntry, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, FileEntry{
			Name:       info.Name(),
			Path:       path.Join(dir, info.Name()),
			IsDir:      info.IsDir(),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		// 目录优先
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

// ReadFile 读取文件全部内容。
func (s *SFTPClient) ReadFile(filePath string) ([]byte, error) {
	sc, err := s.sftpClient()
	if err != nil {
		return nil, fmt.Errorf("sftp new: %w", err)
	}
	defer sc.Close()

	f, err := sc.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("sftp open %s: %w", filePath, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("sftp read %s: %w", filePath, err)
	}
	return data, nil
}

// WriteFile 写入文件内容（覆盖；如不存在且父目录存在则创建）。
// 返回写入字节数。
func (s *SFTPClient) WriteFile(filePath string, content []byte) (int, error) {
	sc, err := s.sftpClient()
	if err != nil {
		return 0, fmt.Errorf("sftp new: %w", err)
	}
	defer sc.Close()

	f, err := sc.Create(filePath)
	if err != nil {
		return 0, fmt.Errorf("sftp create %s: %w", filePath, err)
	}
	defer f.Close()

	n, err := f.Write(content)
	if err != nil {
		return 0, fmt.Errorf("sftp write %s: %w", filePath, err)
	}
	return n, nil
}

// Delete 删除文件或空目录。
func (s *SFTPClient) Delete(filePath string) error {
	sc, err := s.sftpClient()
	if err != nil {
		return fmt.Errorf("sftp new: %w", err)
	}
	defer sc.Close()

	// 先 stat 区分文件/目录
	info, err := sc.Stat(filePath)
	if err != nil {
		return fmt.Errorf("sftp stat %s: %w", filePath, err)
	}

	if info.IsDir() {
		// 仅删除空目录（与 spec 一致；递归删除需 App 显式确认）
		if err := sc.RemoveDirectory(filePath); err != nil {
			return fmt.Errorf("sftp rmdir %s: %w", filePath, err)
		}
	} else {
		if err := sc.Remove(filePath); err != nil {
			return fmt.Errorf("sftp remove %s: %w", filePath, err)
		}
	}
	return nil
}

// Stat 返回文件/目录信息。
func (s *SFTPClient) Stat(filePath string) (os.FileInfo, error) {
	sc, err := s.sftpClient()
	if err != nil {
		return nil, fmt.Errorf("sftp new: %w", err)
	}
	defer sc.Close()

	info, err := sc.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("sftp stat %s: %w", filePath, err)
	}
	return info, nil
}

// Mkdir 创建目录。
func (s *SFTPClient) Mkdir(dirPath string) error {
	sc, err := s.sftpClient()
	if err != nil {
		return fmt.Errorf("sftp new: %w", err)
	}
	defer sc.Close()

	if err := sc.Mkdir(dirPath); err != nil {
		return fmt.Errorf("sftp mkdir %s: %w", dirPath, err)
	}
	return nil
}
