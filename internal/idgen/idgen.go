// Package idgen menghasilkan ID unik pendek yang dipakai sebagai slug pada
// link share (mis. https://domain.com/n/aB3xQ9Kd1mZp).
package idgen

import (
	"crypto/rand"
	"math/big"
)

const (
	alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	length   = 12
)

// New menghasilkan ID acak sepanjang 12 karakter alfanumerik.
// Menggunakan crypto/rand agar ID tidak mudah ditebak (penting karena ID
// ini adalah satu-satunya "kunci akses" untuk note mode public).
func New() (string, error) {
	id := make([]byte, length)
	max := big.NewInt(int64(len(alphabet)))

	for i := range id {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		id[i] = alphabet[n.Int64()]
	}

	return string(id), nil
}
