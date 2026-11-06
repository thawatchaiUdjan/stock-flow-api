package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// SetupLinkLifetime is how long a Setup link works after it is issued (users-and-access R27).
const SetupLinkLifetime = 24 * time.Hour

// ErrOwnerDetailsMissing means there is no Owner yet and no name or email to create one.
var ErrOwnerDetailsMissing = errors.New("no Owner yet: a name and an email are needed to create one")

// OwnerStore is what issuing the Owner's Setup link needs from the database.
type OwnerStore interface {
	GetOwner(ctx context.Context) (store.User, error)
	CreateOwner(ctx context.Context, arg store.CreateOwnerParams) error
	IssueSetupLink(ctx context.Context, arg store.IssueSetupLinkParams) error
}

// SetupLink is an issued link. Token exists only here: the database keeps its hash.
type SetupLink struct {
	Token     string
	Email     string
	ExpiresAt time.Time
}

// IssueOwnerSetupLink creates the Owner when there is none, and issues their Setup link,
// which replaces any earlier one (users-and-access R9, R34). name and email are used only
// when creating: an existing Owner keeps theirs. Run it in one transaction.
func IssueOwnerSetupLink(ctx context.Context, q OwnerStore, now time.Time, name, email string) (SetupLink, error) {
	owner, err := q.GetOwner(ctx)
	if errors.Is(err, store.ErrNotFound) {
		name, email = strings.TrimSpace(name), strings.TrimSpace(email)
		if name == "" || email == "" { // users-and-access R14
			return SetupLink{}, ErrOwnerDetailsMissing
		}
		if err := q.CreateOwner(ctx, store.CreateOwnerParams{Name: name, Email: email}); err != nil {
			return SetupLink{}, fmt.Errorf("create owner: %w", err)
		}
		// Read back, not assumed: a concurrent install may have won.
		owner, err = q.GetOwner(ctx)
	}
	if err != nil {
		return SetupLink{}, fmt.Errorf("get owner: %w", err)
	}

	link := SetupLink{Token: NewToken(), Email: owner.Email, ExpiresAt: now.Add(SetupLinkLifetime)}
	err = q.IssueSetupLink(ctx, store.IssueSetupLinkParams{
		UserID: owner.ID, TokenHash: HashToken(link.Token), ExpiresAt: link.ExpiresAt,
	})
	if err != nil {
		return SetupLink{}, fmt.Errorf("issue setup link: %w", err)
	}
	return link, nil
}
