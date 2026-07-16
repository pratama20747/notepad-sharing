package authutil

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateVerificationToken membuat token verifikasi email acak (32 byte, hex-encoded).
// Token ASLI ini yang dikirim lewat link email — hanya hash-nya yang disimpan di DB.
func GenerateVerificationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashVerificationToken meng-hash token verifikasi dengan SHA-256 untuk disimpan
// di DB. Sama alasannya dengan refresh token: token sudah random & high-entropy.
func HashVerificationToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
