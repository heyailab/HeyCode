// Package crypto 提供 AES-256-GCM 对称加解密能力。
//
// 用途：
//   - Server.encryptedAuth：存储 SSH 凭据（password / privateKey+passphrase）
//   - ApiKey.cipherText/iv/tag：存储各 CLI 的 API Key
//
// 密文结构对齐前端契约：
//
//	Sealed{IV, Tag, CipherText} 三字段均为 hex 字符串
//	Server.encryptedAuth 存 JSON.stringify(Sealed)
//	ApiKey 拆三列 + last4
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

// Sealed 是序列化后的密文容器。
type Sealed struct {
	IV         string `json:"iv"`
	Tag        string `json:"tag"`
	CipherText string `json:"cipherText"`
}

// ParseMasterKey 把 64 字符 hex 字符串解析为 32 字节密钥。
func ParseMasterKey(s string) ([]byte, error) {
	if len(s) != 64 {
		return nil, fmt.Errorf("master key must be 64 hex chars, got %d", len(s))
	}
	key, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid master key hex: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("master key must decode to 32 bytes, got %d", len(key))
	}
	return key, nil
}

// GenerateMasterKey 生成随机 32 字节密钥并返回 hex 字符串。
// 用于 dev 兜底或首次部署时打印建议值。
func GenerateMasterKey() string {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(key)
}

// Encrypt 用 AES-256-GCM 加密 plaintext。
// IV 使用 12 字节随机；tag 与 cipherText 分别 hex 编码返回。
func Encrypt(key, plaintext []byte) (*Sealed, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm new: %w", err)
	}
	iv := make([]byte, gcm.NonceSize()) // 12
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("rand iv: %w", err)
	}
	// Seal 返回 ciphertext || tag
	sealed := gcm.Seal(nil, iv, plaintext, nil)
	tagLen := gcm.Overhead() // 16
	tagStart := len(sealed) - tagLen
	return &Sealed{
		IV:         hex.EncodeToString(iv),
		Tag:        hex.EncodeToString(sealed[tagStart:]),
		CipherText: hex.EncodeToString(sealed[:tagStart]),
	}, nil
}

// Decrypt 解密 Sealed。
func Decrypt(key []byte, sealed *Sealed) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes")
	}
	if sealed == nil {
		return nil, errors.New("sealed is nil")
	}
	iv, err := hex.DecodeString(sealed.IV)
	if err != nil {
		return nil, fmt.Errorf("invalid iv hex: %w", err)
	}
	tag, err := hex.DecodeString(sealed.Tag)
	if err != nil {
		return nil, fmt.Errorf("invalid tag hex: %w", err)
	}
	ciphertext, err := hex.DecodeString(sealed.CipherText)
	if err != nil {
		return nil, fmt.Errorf("invalid cipherText hex: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm new: %w", err)
	}
	if len(iv) != gcm.NonceSize() {
		return nil, fmt.Errorf("iv must be %d bytes, got %d", gcm.NonceSize(), len(iv))
	}
	if len(tag) != gcm.Overhead() {
		return nil, fmt.Errorf("tag must be %d bytes, got %d", gcm.Overhead(), len(tag))
	}
	combined := make([]byte, 0, len(ciphertext)+len(tag))
	combined = append(combined, ciphertext...)
	combined = append(combined, tag...)
	plaintext, err := gcm.Open(nil, iv, combined, nil)
	if err != nil {
		return nil, fmt.Errorf("gcm open: %w", err)
	}
	return plaintext, nil
}
