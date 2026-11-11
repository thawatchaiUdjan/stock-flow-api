package store

import (
	"crypto/subtle"
	"time"
)

// AllowsRequest applies users-and-access R43's limits to the persisted rolling-window counts.
func (usage EmailRequestUsageRow) AllowsRequest() bool {
	return usage.EmailCount < 5 && usage.RecentCount == 0 && usage.IpCount < 30
}

// CanUseEmailCredential is shared pure policy; only database transactions promise
// serialized changes. The fake uses this policy without pretending to prove locking.
// It enforces users-and-access R32, R34 and R45.
func (user User) CanUseEmailCredential(kind string) bool {
	if user.Status != "active" {
		return false
	}
	if kind == ResetLink {
		return user.PasswordHash.Valid
	}
	return !user.PasswordHash.Valid && user.Role != roleOwner
}

// VerifyCode derives the next code/grant state (users-and-access R27, R41, R42). Callers persist the
// returned state under their own storage boundary; this method takes no locks.
func (credential EmailCredential) VerifyCode(arg VerifyFirstTimeCodeParams) (EmailCredential, bool) {
	if credential.Kind != FirstTimeCode || !credential.ExpiresAt.After(arg.At) || credential.BlockedUntil.After(arg.At) {
		return credential, false
	}
	// users-and-access R41: a completed block restores the allowance; a resend never does.
	if credential.FailedAttempts == 5 {
		credential.FailedAttempts = 0
	}
	if subtle.ConstantTimeCompare(credential.TokenHash, arg.CodeHash) != 1 {
		credential.FailedAttempts++
		if credential.FailedAttempts == 5 {
			credential.BlockedUntil = arg.At.Add(10 * time.Minute)
		}
		return credential, false
	}
	credential.Kind, credential.TokenHash, credential.ExpiresAt = SetupGrant, arg.GrantHash, arg.GrantExpiresAt
	return credential, true
}

// AllowsPassword requires both the expected purpose and current User eligibility.
// It enforces users-and-access R31, R42 and R48.
func (credential EmailCredential) AllowsPassword(user User, arg EmailPasswordParams) bool {
	return (arg.Kind == SetupGrant || arg.Kind == ResetLink) && user.CanUseEmailCredential(arg.Kind) &&
		credential.Kind == arg.Kind && credential.ExpiresAt.After(arg.At) &&
		subtle.ConstantTimeCompare(credential.TokenHash, arg.TokenHash) == 1
}
