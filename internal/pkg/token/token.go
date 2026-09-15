package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// Generate 生成一个加密安全的随机令牌（32 字节，hex 编码 64 字符）。
func Generate() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Hash 计算令牌的 SHA-256 哈希，用于刷新令牌等仅需比对而不回读的场景。
func Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
