package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"strings"
)

// NewFirstTimeCode uses uniform cryptographic randomness, including leading zeroes.
func NewFirstTimeCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		panic("cryptographic randomness unavailable")
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// HashFirstTimeCode needs a secret outside the database: six-digit codes alone
// cannot resist offline enumeration. Binding the email prevents cross-User reuse.
func HashFirstTimeCode(secret []byte, email, code string) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte("first-time-code\x00" + strings.ToLower(email) + "\x00" + code))
	return h.Sum(nil)
}
