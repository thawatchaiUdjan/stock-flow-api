package storetest

import (
	"bytes"
	"context"
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
	usage := store.EmailRequestUsageRow{}
	for _, event := range f.emailRequests {
		if !event.At.After(arg.At.Add(-time.Hour)) {
			continue
		}
		kept = append(kept, event)
		if bytes.Equal(event.EmailHash, arg.EmailHash) {
			usage.EmailCount++
			if event.At.After(arg.At.Add(-time.Minute)) {
				usage.RecentCount++
			}
		}
		if bytes.Equal(event.IPHash, arg.IPHash) {
			usage.IpCount++
		}
	}
	f.emailRequests = kept
	if !usage.AllowsRequest() {
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

func (f *Fake) IssuePasswordCredential(_ context.Context, arg store.EmailCredentialParams) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.User{}, f.Err
	}
	for i := range f.users {
		user := &f.users[i]
		if strings.EqualFold(user.Email, arg.Email) && user.CanUseEmailCredential(arg.Kind) {
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
		credential, exists := f.credentials[user.ID]
		if !exists {
			continue
		}
		if strings.EqualFold(user.Email, arg.Email) && user.CanUseEmailCredential(store.FirstTimeCode) {
			next, accepted := credential.VerifyCode(arg)
			if accepted {
				for id, existing := range f.credentials {
					if id != user.ID && bytes.Equal(existing.TokenHash, next.TokenHash) {
						return false, errors.New("violates email_credentials_token_hash_key")
					}
				}
			}
			f.credentials[user.ID] = next
			return accepted, nil
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
		if user != nil && credential.AllowsPassword(*user, arg) {
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
