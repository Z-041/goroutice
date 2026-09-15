package jwt

import (
	"testing"
	"time"
)

func TestGenerateAndParse(t *testing.T) {
	m := NewManager("test-secret", 1, "test-issuer")

	token, expiresAt, err := m.GenerateAccessToken("42", "alice", []string{"admin"}, 0)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if !expiresAt.After(time.Now()) {
		t.Fatal("expected future expiry")
	}

	claims, err := m.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.UserID != "42" || claims.Username != "alice" || len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.Issuer != "test-issuer" {
		t.Fatalf("unexpected issuer: %q", claims.Issuer)
	}
}

func TestParseTokenInvalid(t *testing.T) {
	m := NewManager("test-secret", 1, "test-issuer")

	if _, err := m.ParseToken("not-a-token"); err == nil {
		t.Fatal("expected error for garbage token")
	}

	// 使用不同密钥签发的 token 应解析失败。
	other := NewManager("other-secret", 1, "test-issuer")
	token, _, err := other.GenerateAccessToken("1", "alice", []string{"user"}, 0)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if _, err := m.ParseToken(token); err == nil {
		t.Fatal("expected error for token signed with wrong secret")
	}
}
