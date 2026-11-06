package auth_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store/storetest"
)

// The cheapest settings Argon2id accepts (docs/agents/testing.md).
var cheap = auth.PasswordHasher{Memory: 8, Time: 1}

func TestVerifyAcceptsTheRightPasswordOnly(t *testing.T) {
	encoded := cheap.Hash("correct horse")

	if !cheap.Verify(encoded, "correct horse") {
		t.Error("the right password was refused")
	}
	if cheap.Verify(encoded, "correct hors") {
		t.Error("a wrong password was accepted")
	}
}

func TestAHashRevealsNeitherThePasswordNorThatTwoPasswordsMatch(t *testing.T) {
	first, second := cheap.Hash("correct horse"), cheap.Hash("correct horse")

	if strings.Contains(first, "correct horse") {
		t.Errorf("hash %q contains the password", first)
	}
	if first == second {
		t.Error("the same password hashed the same way twice: no salt")
	}
}

func TestAHashStaysVerifiableAfterTheSettingsChange(t *testing.T) {
	encoded := auth.PasswordHasher{Memory: 16, Time: 2}.Hash("correct horse")

	if !cheap.Verify(encoded, "correct horse") {
		t.Error("a hash made under other settings was refused")
	}
}

func TestDefaultPasswordsUseArgon2id(t *testing.T) {
	encoded := auth.DefaultPasswordHasher.Hash("correct horse")

	if want := "$argon2id$v=19$m=19456,t=2,p=1$"; !strings.HasPrefix(encoded, want) {
		t.Errorf("hash = %q, want prefix %q", encoded, want)
	}
}

func TestVerifyRefusesAnythingThatIsNotAHash(t *testing.T) {
	for _, encoded := range []string{"", "not-a-hash", "$argon2id$v=19$m=8,t=1,p=1$%%%$%%%", "$argon2id$v=19$m=0,t=0,p=0$AAAA$AAAA"} {
		if cheap.Verify(encoded, "") {
			t.Errorf("Verify(%q) accepted", encoded)
		}
	}
}

// users-and-access R5: nobody may learn from the time taken whether an email has a
// password. Real settings and real time, with a margin that a missing guard (about a
// thousandth of the time) can't reach.
func TestVerifyWithNoHashTakesAboutAsLongAsAWrongPassword(t *testing.T) {
	p := auth.DefaultPasswordHasher
	encoded := p.Hash("correct horse")
	median := func(verify func()) time.Duration {
		runs := make([]time.Duration, 5)
		for i := range runs {
			start := time.Now()
			verify()
			runs[i] = time.Since(start)
		}
		slices.Sort(runs)
		return runs[len(runs)/2]
	}

	wrong := median(func() { p.Verify(encoded, "wrong password") })
	none := median(func() { p.Verify("", "wrong password") })

	if none < wrong/4 || none > wrong*4 {
		t.Errorf("no hash took %v, a wrong password %v", none, wrong)
	}
}

func TestTokensAreUniqueAndHashedDeterministically(t *testing.T) {
	first, second := auth.NewToken(), auth.NewToken()

	if first == second {
		t.Error("two tokens are the same")
	}
	if len(first) < 26 { // base32 of 128 bits
		t.Errorf("token %q is too short to be unguessable", first)
	}
	if !bytes.Equal(auth.HashToken(first), auth.HashToken(first)) {
		t.Error("a token hashed two ways")
	}
	if bytes.Contains(auth.HashToken(first), []byte(first)) {
		t.Error("the hash contains the token")
	}
}

var now = time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)

func TestIssueOwnerSetupLinkCreatesTheOwnerWhenThereIsNone(t *testing.T) {
	ctx := context.Background()
	fake := storetest.NewFake()

	link, err := auth.IssueOwnerSetupLink(ctx, fake, now, "Malee", "malee@example.com")

	if err != nil {
		t.Fatal(err)
	}
	owner, err := fake.GetOwner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Name != "Malee" || owner.Email != "malee@example.com" || owner.Status != "active" || owner.PasswordHash.Valid {
		t.Errorf("owner = %+v", owner)
	}
	if link.Email != "malee@example.com" {
		t.Errorf("link email = %q", link.Email)
	}
	if want := now.Add(24 * time.Hour); !link.ExpiresAt.Equal(want) {
		t.Errorf("expires at %v, want %v", link.ExpiresAt, want)
	}
	got, err := fake.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: auth.HashToken(link.Token), At: now})
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "malee@example.com" {
		t.Errorf("link names %q", got.Email)
	}
}

// users-and-access R34: the Owner who forgot their password gets a new link, and the
// earlier one stops working (context ADR 0002).
func TestIssueOwnerSetupLinkRunAgainReplacesTheEarlierLinkForTheSameOwner(t *testing.T) {
	ctx := context.Background()
	fake := storetest.NewFake()
	first, err := auth.IssueOwnerSetupLink(ctx, fake, now, "Malee", "malee@example.com")
	if err != nil {
		t.Fatal(err)
	}

	second, err := auth.IssueOwnerSetupLink(ctx, fake, now, "", "")

	if err != nil {
		t.Fatal(err)
	}
	if second.Token == first.Token {
		t.Fatal("the same token was issued twice")
	}
	if second.Email != "malee@example.com" {
		t.Errorf("second link is for %q", second.Email)
	}
	_, err = fake.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: auth.HashToken(first.Token), At: now})
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("earlier link: err = %v, want ErrNotFound", err)
	}
	if _, err := fake.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: auth.HashToken(second.Token), At: now}); err != nil {
		t.Errorf("newest link: %v", err)
	}
}

// users-and-access R9: run again with other details, it still leaves the one Owner.
func TestIssueOwnerSetupLinkNeverMakesASecondOwner(t *testing.T) {
	ctx := context.Background()
	fake := storetest.NewFake()
	if _, err := auth.IssueOwnerSetupLink(ctx, fake, now, "Malee", "malee@example.com"); err != nil {
		t.Fatal(err)
	}

	link, err := auth.IssueOwnerSetupLink(ctx, fake, now, "Somchai", "somchai@example.com")

	if err != nil {
		t.Fatal(err)
	}
	if link.Email != "malee@example.com" {
		t.Errorf("link is for %q, want the existing Owner", link.Email)
	}
	if _, err := fake.GetUserByEmail(ctx, "somchai@example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want no such User", err)
	}
}

// users-and-access R14: the Owner, like every User, has a name and an email.
func TestIssueOwnerSetupLinkNeedsANameAndEmailToCreateTheOwner(t *testing.T) {
	for name, tc := range map[string]struct{ name, email string }{
		"no name":  {"", "malee@example.com"},
		"no email": {"Malee", ""},
	} {
		t.Run(name, func(t *testing.T) {
			fake := storetest.NewFake()

			_, err := auth.IssueOwnerSetupLink(context.Background(), fake, now, tc.name, tc.email)

			if !errors.Is(err, auth.ErrOwnerDetailsMissing) {
				t.Errorf("err = %v, want ErrOwnerDetailsMissing", err)
			}
			if _, err := fake.GetOwner(context.Background()); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("an Owner was created anyway: err = %v", err)
			}
		})
	}
}

// users-and-access R2, R15: a stray space would stay in the email for good, and the Owner
// could never log in with their real one.
func TestIssueOwnerSetupLinkTrimsTheOwnersDetails(t *testing.T) {
	ctx := context.Background()
	fake := storetest.NewFake()

	if _, err := auth.IssueOwnerSetupLink(ctx, fake, now, " Malee ", " malee@example.com "); err != nil {
		t.Fatal(err)
	}

	owner, err := fake.GetOwner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Name != "Malee" || owner.Email != "malee@example.com" {
		t.Errorf("owner = %q, %q", owner.Name, owner.Email)
	}
}

func TestIssueOwnerSetupLinkReportsAStoreFailureAsSuch(t *testing.T) {
	down := errors.New("connection refused")
	fake := &storetest.Fake{Err: down}

	_, err := auth.IssueOwnerSetupLink(context.Background(), fake, now, "", "")

	if !errors.Is(err, down) {
		t.Errorf("err = %v, want the store's failure", err)
	}
}
