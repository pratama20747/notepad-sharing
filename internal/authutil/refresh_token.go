package authutil

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateRefreshToken membuat refresh token acak (32 byte, hex-encoded).
// Token ASLI ini yang dikirim ke client — hanya hash-nya yang disimpan di DB.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashRefreshToken meng-hash refresh token dengan SHA-256 untuk disimpan di DB.
// Bukan bcrypt karena refresh token sudah random & high-entropy (beda kasus
// dengan password yang low-entropy dan butuh cost function lambat).
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
