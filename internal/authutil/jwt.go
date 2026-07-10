package authutil

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken dikembalikan untuk semua kasus token tidak valid/expired,
// sengaja tidak dirinci lebih jauh supaya tidak membocorkan info ke client.
var ErrInvalidToken = errors.New("token tidak valid atau sudah expired")

// Claims adalah payload custom di dalam access token JWT.
type Claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// JWTManager membungkus secret & TTL untuk generate/parse access token.
type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTManager(secret string, ttl time.Duration) *JWTManager {
	return &JWTManager{secret: []byte(secret), ttl: ttl}
}

// Generate membuat access token baru untuk userID yang diberikan.
func (m *JWTManager) Generate(userID string) (string, error) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Parse memvalidasi signature & expiry, mengembalikan Claims jika valid.
func (m *JWTManager) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
