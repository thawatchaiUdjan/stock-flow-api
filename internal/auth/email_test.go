package auth_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
)

// Independent HMAC-SHA-256 fixture, calculated with Python's standard hmac module.
// This existing public auth-helper seam catches losing the secret/purpose binding.
func TestFirstTimeCodeDigestBindsSecretPurposeAndCaseInsensitiveUser(t *testing.T) {
	secret := []byte("test-secret-with-at-least-32-bytes")
	got := auth.HashFirstTimeCode(secret, "Napa@Example.com", "012345")
	if hex.EncodeToString(got) != "5ddaee87423dcf7af31d1f867eaa841a1fc002f269a9b4c1ab6fc2ea1477f7d0" {
		t.Fatalf("digest does not match independent fixture: %x", got)
	}
	if !bytes.Equal(got, auth.HashFirstTimeCode(secret, "napa@example.com", "012345")) {
		t.Fatal("email letter case changed code identity")
	}
	for name, digest := range map[string][]byte{
		"different secret":     auth.HashFirstTimeCode([]byte("another-test-secret-with-32-bytes"), "napa@example.com", "012345"),
		"different User":       auth.HashFirstTimeCode(secret, "other@example.com", "012345"),
		"different code":       auth.HashFirstTimeCode(secret, "napa@example.com", "012346"),
		"random-token purpose": auth.HashToken("012345"),
	} {
		if bytes.Equal(got, digest) {
			t.Fatalf("digest was reused for %s", name)
		}
	}
}
