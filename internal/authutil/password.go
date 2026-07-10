package authutil

import "golang.org/x/crypto/bcrypt"

// HashPassword meng-hash password dengan bcrypt (cost default).
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword membandingkan password plaintext dengan hash bcrypt.
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
