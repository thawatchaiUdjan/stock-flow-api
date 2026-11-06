// Package storetest holds the contract suite of the store ports and an in-memory fake
// that passes it. Imported by tests only.
package storetest

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// Fake is an in-memory Store. The mutex only keeps it free of data races: what locking
// and transactions do for real is shown by the slow tests, not pretended here.
type Fake struct {
	// Err, when set, is returned by every operation, like a database that is down.
	Err error

	mu       sync.Mutex
	users    []store.User
	sessions map[string]store.Session
	links    map[int64]store.SetupLink
}

var _ Store = (*Fake)(nil)

// NewFake returns an empty Fake.
func NewFake() *Fake {
	return &Fake{sessions: map[string]store.Session{}, links: map[int64]store.SetupLink{}}
}

// AddUser seeds a User and returns it with its ID.
func (f *Fake) AddUser(user store.User) store.User {
	f.mu.Lock()
	defer f.mu.Unlock()
	user.ID = int64(len(f.users) + 1)
	f.users = append(f.users, user)
	return user
}

// SetupLinks returns what is stored, as anyone reading the database would see it.
func (f *Fake) SetupLinks() []store.SetupLink {
	f.mu.Lock()
	defer f.mu.Unlock()
	links := make([]store.SetupLink, 0, len(f.links))
	for _, link := range f.links {
		links = append(links, link)
	}
	return links
}

func (f *Fake) Ping(context.Context) error { return f.Err }

func (f *Fake) GetUserByEmail(_ context.Context, email string) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.User{}, f.Err
	}
	for _, user := range f.users {
		if strings.EqualFold(user.Email, email) {
			return user, nil
		}
	}
	return store.User{}, store.ErrNotFound
}

func (f *Fake) GetOwner(context.Context) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.User{}, f.Err
	}
	for _, user := range f.users {
		if user.Role == "owner" {
			return user, nil
		}
	}
	return store.User{}, store.ErrNotFound
}

func (f *Fake) CreateOwner(_ context.Context, arg store.CreateOwnerParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	if strings.TrimSpace(arg.Name) == "" {
		return errors.New("violates users_name_check")
	}
	for _, user := range f.users {
		if user.Role == "owner" || strings.EqualFold(user.Email, arg.Email) {
			return nil
		}
	}
	f.users = append(f.users, store.User{
		ID: int64(len(f.users) + 1), Name: arg.Name, Email: arg.Email, Role: "owner", Status: "active",
	})
	return nil
}

func (f *Fake) CreateSession(_ context.Context, arg store.CreateSessionParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.user(arg.UserID); !ok {
		return errors.New("violates sessions_user_id_fkey")
	}
	if _, ok := f.sessions[string(arg.TokenHash)]; ok {
		return errors.New("violates sessions_pkey")
	}
	f.sessions[string(arg.TokenHash)] = store.Session(arg)
	return nil
}

func (f *Fake) GetSession(_ context.Context, tokenHash []byte) (store.GetSessionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.GetSessionRow{}, f.Err
	}
	session, ok := f.sessions[string(tokenHash)]
	if !ok {
		return store.GetSessionRow{}, store.ErrNotFound
	}
	user, _ := f.user(session.UserID)
	return store.GetSessionRow{Name: user.Name, Email: user.Email, Role: user.Role, ExpiresAt: session.ExpiresAt}, nil
}

func (f *Fake) DeleteSession(_ context.Context, tokenHash []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	delete(f.sessions, string(tokenHash))
	return nil
}

func (f *Fake) IssueSetupLink(_ context.Context, arg store.IssueSetupLinkParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.user(arg.UserID); !ok {
		return errors.New("violates setup_links_user_id_fkey")
	}
	for userID, link := range f.links {
		if userID != arg.UserID && string(link.TokenHash) == string(arg.TokenHash) {
			return errors.New("violates setup_links_token_hash_key")
		}
	}
	f.links[arg.UserID] = store.SetupLink(arg)
	return nil
}

func (f *Fake) GetSetupLinkUser(_ context.Context, arg store.GetSetupLinkUserParams) (store.GetSetupLinkUserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.GetSetupLinkUserRow{}, f.Err
	}
	for _, link := range f.links {
		if string(link.TokenHash) == string(arg.TokenHash) && link.ExpiresAt.After(arg.At) {
			user, _ := f.user(link.UserID)
			return store.GetSetupLinkUserRow{Name: user.Name, Email: user.Email}, nil
		}
	}
	return store.GetSetupLinkUserRow{}, store.ErrNotFound
}

func (f *Fake) SetPassword(_ context.Context, arg store.SetPasswordParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return 0, f.Err
	}
	for userID, link := range f.links {
		if string(link.TokenHash) == string(arg.TokenHash) && link.ExpiresAt.After(arg.At) {
			delete(f.links, userID)
			for i := range f.users {
				if f.users[i].ID == userID {
					f.users[i].PasswordHash = pgtype.Text{String: arg.PasswordHash, Valid: true}
				}
			}
			return 1, nil
		}
	}
	return 0, nil
}

func (f *Fake) user(id int64) (store.User, bool) {
	for _, user := range f.users {
		if user.ID == id {
			return user, true
		}
	}
	return store.User{}, false
}
