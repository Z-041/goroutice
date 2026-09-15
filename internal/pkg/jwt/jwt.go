package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenTypeAccess 标识 access token 类型，防止其他类型令牌被误用为访问令牌。
const TokenTypeAccess = "access"

// Claims 是自定义 JWT 载荷。
type Claims struct {
	UserID       string   `json:"user_id"`
	Username     string   `json:"username"`
	Roles        []string `json:"roles"`
	TokenType    string   `json:"token_type"`
	TokenVersion int      `json:"token_version"`
	jwt.RegisteredClaims
}

// Manager 负责签发与解析 access token。
type Manager struct {
	secret       []byte
	accessExpire time.Duration
	issuer       string
}

// NewManager 构造 JWT 管理器。
func NewManager(secret string, accessExpireMinutes int, issuer string) *Manager {
	return &Manager{
		secret:       []byte(secret),
		accessExpire: time.Duration(accessExpireMinutes) * time.Minute,
		issuer:       issuer,
	}
}

// GenerateAccessToken 签发 access token，并返回过期时间。
// tokenVersion 用于主动失效控制：改密/注销后递增，旧令牌随之失效。
func (m *Manager) GenerateAccessToken(userID string, username string, roles []string, tokenVersion int) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(m.accessExpire)

	claims := Claims{
		UserID:       userID,
		Username:     username,
		Roles:        roles,
		TokenType:    TokenTypeAccess,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   username,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	return signed, expiresAt, err
}

// ParseToken 解析并校验 token。
func (m *Manager) ParseToken(tokenStr string) (*Claims, error) {
	opts := []jwt.ParserOption{}
	if m.issuer != "" {
		opts = append(opts, jwt.WithIssuer(m.issuer))
	}

	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	}, opts...)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.TokenType != TokenTypeAccess {
		return nil, errors.New("invalid token type")
	}
	return claims, nil
}
