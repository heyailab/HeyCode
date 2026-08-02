// Package service 是业务服务层，封装跨 store 的业务逻辑、SshAuth 加解密、ID 生成、时间戳。
// handler 层只与 service 层交互，不直接访问 store。
package service

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/heycode/backend-go/internal/crypto"
	"github.com/heycode/backend-go/internal/idgen"
	"github.com/heycode/backend-go/internal/store"
	"github.com/heycode/backend-go/internal/types"
)

// ErrNotFound 是 service 层统一的不存在错误，handler 据此返回 404。
var ErrNotFound = errors.New("not found")

// ---- SshAuth 加解密 ----

// encryptAuth 把 SshAuth 序列化为 JSON 并用 masterKey 加密，返回可入库的 JSON 字符串。
func encryptAuth(masterKey []byte, auth types.SshAuth) (string, error) {
	data, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}
	sealed, err := crypto.Encrypt(masterKey, data)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(sealed)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// decryptAuth 反向解密。
func decryptAuth(masterKey []byte, encrypted string) (types.SshAuth, error) {
	var sealed crypto.Sealed
	if err := json.Unmarshal([]byte(encrypted), &sealed); err != nil {
		return types.SshAuth{}, err
	}
	data, err := crypto.Decrypt(masterKey, &sealed)
	if err != nil {
		return types.SshAuth{}, err
	}
	var auth types.SshAuth
	if err := json.Unmarshal(data, &auth); err != nil {
		return types.SshAuth{}, err
	}
	return auth, nil
}

// mapErr 把 store 层错误归一为 service 层错误。
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// nowUTC 返回当前 UTC 时间（秒级精度足够，避免 nano 在 SQLite 文本列里冗长）。
func nowUTC() time.Time {
	return time.Now().UTC()
}

// reuseID 生成新 cuid。
func newID() string { return idgen.New() }
