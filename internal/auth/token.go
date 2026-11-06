package auth

import (
	"crypto/rand"
	"crypto/sha256"
)

// NewToken returns an unguessable token for a session or a Setup link.
func NewToken() string { return rand.Text() }

// HashToken is how a token is stored, so reading the database reveals no token that
// works (context ADR 0002). A token has 128 random bits, so unlike a password it needs
// no slow hash.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
