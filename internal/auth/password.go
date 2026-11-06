// Package auth holds what logging in rests on, outside HTTP: password hashing, tokens,
// and issuing the Owner's Setup link.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	saltBytes = 16
	keyBytes  = 32
	threads   = 1
)

// Passwords hashes and verifies passwords with Argon2id (ADR 0020). Memory is in KiB.
type Passwords struct {
	Memory uint32
	Time   uint32
}

// DefaultPasswords is OWASP's minimum for Argon2id, which fits a Vercel instance (ADR 0020).
var DefaultPasswords = Passwords{Memory: 19 * 1024, Time: 2}

// Hash returns password in the PHC string format, which carries its own settings.
func (p Passwords) Hash(password string) string {
	salt := make([]byte, saltBytes)
	rand.Read(salt)
	key := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, threads, keyBytes)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, p.Memory, p.Time, threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

// Verify reports whether password is the one encoded. With nothing to verify against (no
// User, or no password yet) it still does a hash's work and answers false, so the time
// taken doesn't tell which case it was (users-and-access R5).
func (p Passwords) Verify(encoded, password string) bool {
	h, ok := parseHash(encoded)
	if !ok {
		argon2.IDKey([]byte(password), make([]byte, saltBytes), p.Time, p.Memory, threads, keyBytes)
		return false
	}
	key := argon2.IDKey([]byte(password), h.salt, h.time, h.memory, h.threads, uint32(len(h.key)))
	return subtle.ConstantTimeCompare(key, h.key) == 1
}

type parsedHash struct {
	memory, time uint32
	threads      uint8
	salt, key    []byte
}

func parseHash(encoded string) (parsedHash, bool) {
	var h parsedHash
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return h, false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &h.memory, &h.time, &h.threads); err != nil {
		return h, false
	}
	var err error
	if h.salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return h, false
	}
	if h.key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return h, false
	}
	// argon2.IDKey panics on a zero time or thread count.
	return h, h.time > 0 && h.threads > 0 && len(h.key) > 0
}
