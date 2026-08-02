package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/heycode/backend-go/internal/types"
)

// ApiKey 是 api_keys 表的实体。cipher_text/iv/tag 是 AES-256-GCM 密文的三部分。
type ApiKey struct {
	Cli        types.CliKind
	CipherText string
	IV         string
	Tag        string
	Last4      string // 可空
	UpdatedAt  time.Time
}

type ApiKeyStore struct {
	db *sql.DB
}

func NewApiKeyStore(db *sql.DB) *ApiKeyStore {
	return &ApiKeyStore{db: db}
}

const apiKeyColumns = `cli, cipher_text, iv, tag, last4, updated_at`

// Upsert 插入或更新（按 cli 主键冲突时覆盖）。
func (s *ApiKeyStore) Upsert(ctx context.Context, k *ApiKey) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO api_keys (cli, cipher_text, iv, tag, last4, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(cli) DO UPDATE SET
	cipher_text = excluded.cipher_text,
	iv          = excluded.iv,
	tag         = excluded.tag,
	last4       = excluded.last4,
	updated_at  = excluded.updated_at`,
		string(k.Cli), k.CipherText, k.IV, k.Tag, nullableStr(k.Last4), timeToStr(k.UpdatedAt),
	)
	return err
}

func (s *ApiKeyStore) GetByCli(ctx context.Context, cli types.CliKind) (*ApiKey, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+apiKeyColumns+` FROM api_keys WHERE cli = ?`, string(cli))
	return scanApiKey(row)
}

// ListAll 返回全部已存的 API Key。service 层负责补齐未配置的 cli 为 hasKey=false。
func (s *ApiKeyStore) ListAll(ctx context.Context) ([]*ApiKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+apiKeyColumns+` FROM api_keys ORDER BY cli`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ApiKey
	for rows.Next() {
		k, err := scanApiKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *ApiKeyStore) Delete(ctx context.Context, cli types.CliKind) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE cli = ?`, string(cli))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanApiKey(sc scanner) (*ApiKey, error) {
	var k ApiKey
	var cli string
	var last4 sql.NullString
	var updatedAt string
	err := sc.Scan(&cli, &k.CipherText, &k.IV, &k.Tag, &last4, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	k.Cli = types.CliKind(cli)
	if last4.Valid {
		k.Last4 = last4.String
	}
	if updatedAt != "" {
		t, _ := strToTime(updatedAt)
		k.UpdatedAt = t
	}
	return &k, nil
}
