package storetest

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func (f *Fake) ReserveEmailRequest(_ context.Context, arg store.EmailRequestParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return false, f.Err
	}
	kept := f.emailRequests[:0]
	emailCount, ipCount := 0, 0
	cooldown := false
	for _, event := range f.emailRequests {
		if !event.At.After(arg.At.Add(-time.Hour)) {
			continue
		}
		kept = append(kept, event)
		if bytes.Equal(event.EmailHash, arg.EmailHash) {
			emailCount++
			cooldown = cooldown || event.At.After(arg.At.Add(-time.Minute))
		}
		if bytes.Equal(event.IPHash, arg.IPHash) {
			ipCount++
		}
	}
	f.emailRequests = kept
	if cooldown || emailCount >= 5 || ipCount >= 30 {
		return false, nil
	}
	f.emailRequests = append(f.emailRequests, arg)
	return true, nil
}

func (f *Fake) CreateEmailUser(_ context.Context, arg store.CreateEmailUserParams) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.User{}, f.Err
	}
	if strings.TrimSpace(arg.Name) == "" {
		return store.User{}, errors.New("violates users_name_check")
	}
	if arg.Role != "owner" && arg.Role != "admin" && arg.Role != "staff" {
		return store.User{}, errors.New("violates users_role_check")
	}
	for _, user := range f.users {
		if strings.EqualFold(user.Email, arg.Email) {
			return store.User{}, store.ErrNotFound
		}
		if arg.Role == "owner" && user.Role == "owner" {
			return store.User{}, errors.New("violates users_one_owner_key")
		}
	}
	user := store.User{ID: int64(len(f.users) + 1), Name: arg.Name, Email: arg.Email, Role: arg.Role, Status: "active"}
	f.users = append(f.users, user)
	return user, nil
}

func credentialEligible(user *store.User, kind string) bool {
	if user == nil || user.Status != "active" {
		return false
	}
	if kind == store.ResetLink {
		return user.PasswordHash.Valid
	}
	return !user.PasswordHash.Valid && user.Role != "owner"
}

func (f *Fake) IssuePasswordCredential(_ context.Context, arg store.EmailCredentialParams) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.User{}, f.Err
	}
	for i := range f.users {
		user := &f.users[i]
		if strings.EqualFold(user.Email, arg.Email) && credentialEligible(user, arg.Kind) {
			if arg.Kind != store.FirstTimeCode && arg.Kind != store.SetupGrant && arg.Kind != store.ResetLink {
				return store.User{}, errors.New("violates email_credentials_kind_check")
			}
			for id, existing := range f.credentials {
				if id != user.ID && bytes.Equal(existing.TokenHash, arg.TokenHash) {
					return store.User{}, errors.New("violates email_credentials_token_hash_key")
				}
			}
			credential := f.credentials[user.ID]
			credential.UserID, credential.Kind, credential.TokenHash, credential.ExpiresAt = user.ID, arg.Kind, arg.TokenHash, arg.ExpiresAt
			f.credentials[user.ID] = credential
			return *user, nil
		}
	}
	return store.User{}, store.ErrNotFound
}

func (f *Fake) VerifyFirstTimeCode(_ context.Context, arg store.VerifyFirstTimeCodeParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return false, f.Err
	}
	for i := range f.users {
		user := &f.users[i]
		credential := f.credentials[user.ID]
		if strings.EqualFold(user.Email, arg.Email) && credentialEligible(user, store.FirstTimeCode) &&
			credential.Kind == store.FirstTimeCode && credential.ExpiresAt.After(arg.At) && !credential.BlockedUntil.After(arg.At) {
			if credential.FailedAttempts == 5 {
				credential.FailedAttempts = 0
			}
			if subtle.ConstantTimeCompare(credential.TokenHash, arg.CodeHash) != 1 {
				credential.FailedAttempts++
				if credential.FailedAttempts == 5 {
					credential.BlockedUntil = arg.At.Add(10 * time.Minute)
				}
				f.credentials[user.ID] = credential
				return false, nil
			}
			credential.Kind, credential.TokenHash, credential.ExpiresAt = store.SetupGrant, arg.GrantHash, arg.GrantExpiresAt
			f.credentials[user.ID] = credential
			return true, nil
		}
	}
	return false, nil
}

func (f *Fake) emailPasswordUser(arg store.EmailPasswordParams) (*store.User, error) {
	if arg.Kind != store.SetupGrant && arg.Kind != store.ResetLink {
		return nil, store.ErrNotFound
	}
	for id, credential := range f.credentials {
		user := f.userRef(id)
		if credential.Kind == arg.Kind && credential.ExpiresAt.After(arg.At) &&
			subtle.ConstantTimeCompare(credential.TokenHash, arg.TokenHash) == 1 && credentialEligible(user, arg.Kind) {
			return user, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *Fake) ReadEmailPasswordUser(_ context.Context, arg store.EmailPasswordParams) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.User{}, f.Err
	}
	user, err := f.emailPasswordUser(arg)
	if err != nil {
		return store.User{}, err
	}
	return *user, nil
}

func (f *Fake) SetEmailPassword(_ context.Context, arg store.EmailPasswordParams) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.User{}, f.Err
	}
	user, err := f.emailPasswordUser(arg)
	if err != nil {
		return store.User{}, err
	}
	user.PasswordHash = pgtype.Text{String: arg.PasswordHash, Valid: true}
	f.endSessions(user.ID)
	delete(f.links, user.ID)
	delete(f.credentials, user.ID)
	return *user, nil
}
