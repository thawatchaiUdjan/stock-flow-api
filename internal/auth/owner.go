package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// setupLinkLifetime is how long a Setup link works after it is issued (users-and-access R27).
const setupLinkLifetime = 24 * time.Hour

// ErrOwnerDetailsMissing means there is no Owner yet and no name or email to create one.
var ErrOwnerDetailsMissing = errors.New("no Owner yet: a name and an email are needed to create one")

// OwnerStore is what issuing the Owner's Setup link needs from the database.
type OwnerStore interface {
	GetOwner(ctx context.Context) (store.User, error)
	CreateOwner(ctx context.Context, arg store.CreateOwnerParams) (created int64, err error)
	IssueSetupLink(ctx context.Context, arg store.IssueSetupLinkParams) error
	RecordActivity(ctx context.Context, entry store.ActivityEntry) error
}

// SetupLink is an issued link. Token exists only here: the database keeps its hash.
// Email is who it is for, when the issuer has to say so.
type SetupLink struct {
	Token     string
	Email     string
	ExpiresAt time.Time
}

// NewSetupLink makes a link valid from now for 24 hours (users-and-access R27). Storing
// the hash of its token is the caller's job.
func NewSetupLink(now time.Time) SetupLink {
	return SetupLink{Token: NewToken(), ExpiresAt: now.Add(setupLinkLifetime)}
}

// IssueOwnerSetupLink creates the Owner when there is none, and issues their Setup link,
// which replaces any earlier one (users-and-access R9, R34). name and email are used only
// when creating: an existing Owner keeps theirs. Each is an Activity log entry made by the
// installation (activity-log R4). Run it in one transaction, so a change and its entry are
// saved together (activity-log R3).
func IssueOwnerSetupLink(ctx context.Context, q OwnerStore, now time.Time, name, email string) (SetupLink, error) {
	by := store.ActivityBy{At: now, Actor: store.ActivityActor{Kind: store.ActorInstallation, Name: "Installation"}}
	owner, err := q.GetOwner(ctx)
	if errors.Is(err, store.ErrNotFound) {
		name, email = strings.TrimSpace(name), strings.TrimSpace(email)
		if name == "" || email == "" { // users-and-access R14
			return SetupLink{}, ErrOwnerDetailsMissing
		}
		var created int64
		if created, err = q.CreateOwner(ctx, store.CreateOwnerParams{Name: name, Email: email}); err != nil {
			return SetupLink{}, fmt.Errorf("create owner: %w", err)
		}
		// Read back, not assumed: a concurrent install may have won.
		owner, err = q.GetOwner(ctx)
		// Only the install that made the Owner records it: the loser created nothing.
		if err == nil && created > 0 {
			if err := q.RecordActivity(ctx, store.UserCreated(ownerSnapshot(owner), by)); err != nil {
				return SetupLink{}, fmt.Errorf("record owner created: %w", err)
			}
		}
	}
	if err != nil {
		return SetupLink{}, fmt.Errorf("get owner: %w", err)
	}

	link := NewSetupLink(now)
	link.Email = owner.Email
	err = q.IssueSetupLink(ctx, store.IssueSetupLinkParams{
		UserID: owner.ID, TokenHash: HashToken(link.Token), ExpiresAt: link.ExpiresAt,
	})
	if err != nil {
		return SetupLink{}, fmt.Errorf("issue setup link: %w", err)
	}
	if err := q.RecordActivity(ctx, store.UserSetupLinkIssued(ownerSnapshot(owner), by)); err != nil {
		return SetupLink{}, fmt.Errorf("record setup link issued: %w", err)
	}
	return link, nil
}

func ownerSnapshot(u store.User) store.UserSnapshot {
	return store.UserSnapshot{ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role, Status: u.Status}
}
