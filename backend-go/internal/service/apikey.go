package service

import (
	"context"
	"errors"
	"strings"

	"github.com/heycode/backend-go/internal/crypto"
	"github.com/heycode/backend-go/internal/store"
	"github.com/heycode/backend-go/internal/types"
)

type ApiKeyService struct {
	store     *store.ApiKeyStore
	masterKey []byte
}

func NewApiKeyService(s *store.ApiKeyStore, masterKey []byte) *ApiKeyService {
	return &ApiKeyService{store: s, masterKey: masterKey}
}

// UpsertApiKeyParams 是 POST /api/api-keys 的入参。
type UpsertApiKeyParams struct {
	Cli string `json:"cli"`
	Key string `json:"key"`
}

// List 返回全部 6 个支持 cli 的 ApiKeyMeta，未配置的 cli 也列出（hasKey=false）。
func (s *ApiKeyService) List(ctx context.Context) ([]*types.ApiKeyMeta, error) {
	existing, err := s.store.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	byCli := make(map[types.CliKind]*store.ApiKey, len(existing))
	for _, k := range existing {
		byCli[k.Cli] = k
	}
	out := make([]*types.ApiKeyMeta, 0, len(types.SupportedCliKinds))
	for _, cli := range types.SupportedCliKinds {
		out = append(out, s.toMeta(cli, byCli[cli]))
	}
	return out, nil
}

// Upsert 保存或更新某 cli 的 API Key。
// last4 取 key 末尾 4 字符（不足 4 用全量）。
func (s *ApiKeyService) Upsert(ctx context.Context, p UpsertApiKeyParams) (*types.ApiKeyMeta, error) {
	cli := types.CliKind(p.Cli)
	if !types.IsSupportedCliKind(cli) {
		return nil, errors.New("unsupported cli: " + p.Cli)
	}
	if p.Key == "" {
		return nil, errors.New("key is required")
	}
	sealed, err := crypto.Encrypt(s.masterKey, []byte(p.Key))
	if err != nil {
		return nil, err
	}
	last4 := last4Of(p.Key)
	k := &store.ApiKey{
		Cli:        cli,
		CipherText: sealed.CipherText,
		IV:         sealed.IV,
		Tag:        sealed.Tag,
		Last4:      last4,
		UpdatedAt:  nowUTC(),
	}
	if err := s.store.Upsert(ctx, k); err != nil {
		return nil, err
	}
	return s.toMeta(cli, k), nil
}

// Delete 删除某 cli 的 API Key。未配置返回 ErrNotFound（handler 决定状态码）。
func (s *ApiKeyService) Delete(ctx context.Context, cli types.CliKind) error {
	if !types.IsSupportedCliKind(cli) {
		return errors.New("unsupported cli: " + string(cli))
	}
	if err := s.store.Delete(ctx, cli); err != nil {
		return mapErr(err)
	}
	return nil
}

// GetDecryptedKey 供 CLI 适配器（M4）使用：解密某 cli 的 API Key 明文。
func (s *ApiKeyService) GetDecryptedKey(ctx context.Context, cli types.CliKind) (string, error) {
	k, err := s.store.GetByCli(ctx, cli)
	if err != nil {
		return "", mapErr(err)
	}
	plaintext, err := crypto.Decrypt(s.masterKey, &crypto.Sealed{
		IV:         k.IV,
		Tag:        k.Tag,
		CipherText: k.CipherText,
	})
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (s *ApiKeyService) toMeta(cli types.CliKind, k *store.ApiKey) *types.ApiKeyMeta {
	m := &types.ApiKeyMeta{Cli: cli, HasKey: false}
	if k == nil {
		return m
	}
	m.HasKey = true
	m.Last4 = k.Last4
	if !k.UpdatedAt.IsZero() {
		t := k.UpdatedAt
		m.UpdatedAt = &t
	}
	return m
}

// last4Of 取字符串末尾 4 字符；不足 4 返回原串。
func last4Of(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}
