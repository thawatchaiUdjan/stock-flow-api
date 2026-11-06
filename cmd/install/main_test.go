package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store/storetest"
)

var now = time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)

func issueWith(fake *storetest.Fake) issueFunc {
	return func(name, email string) (auth.SetupLink, error) {
		return auth.IssueOwnerSetupLink(context.Background(), fake, now, name, email)
	}
}

func printedToken(t *testing.T, out string) string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return lines[len(lines)-1]
}

func TestInstallCreatesTheOwnerAndPrintsAWorkingSetupLinkToken(t *testing.T) {
	ctx := context.Background()
	fake := storetest.NewFake()
	var out bytes.Buffer

	err := install([]string{"-name", "Malee", "-email", "malee@example.com"}, &out, issueWith(fake))

	if err != nil {
		t.Fatal(err)
	}
	owner, err := fake.GetOwner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Name != "Malee" || owner.Email != "malee@example.com" {
		t.Errorf("owner = %+v", owner)
	}
	token := printedToken(t, out.String())
	got, err := fake.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: auth.HashToken(token), At: now})
	if err != nil {
		t.Fatalf("printed token %q does not work: %v", token, err)
	}
	if got.Email != "malee@example.com" {
		t.Errorf("token is for %q", got.Email)
	}
	if !strings.Contains(out.String(), "malee@example.com") || !strings.Contains(out.String(), "2026-11-07T09:00:00Z") {
		t.Errorf("output names neither the Owner nor the expiry:\n%s", out.String())
	}
}

// users-and-access R34.
func TestInstallRunAgainNeedsNoDetailsAndStopsTheEarlierToken(t *testing.T) {
	ctx := context.Background()
	fake := storetest.NewFake()
	var first, second bytes.Buffer
	if err := install([]string{"-name", "Malee", "-email", "malee@example.com"}, &first, issueWith(fake)); err != nil {
		t.Fatal(err)
	}

	if err := install(nil, &second, issueWith(fake)); err != nil {
		t.Fatal(err)
	}

	earlier := auth.HashToken(printedToken(t, first.String()))
	_, err := fake.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: earlier, At: now})
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("earlier token: err = %v, want ErrNotFound", err)
	}
	newest := auth.HashToken(printedToken(t, second.String()))
	if _, err := fake.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{TokenHash: newest, At: now}); err != nil {
		t.Errorf("newest token: %v", err)
	}
}

// users-and-access R20, R34: it takes no password, so no password can leak from here.
func TestInstallTakesNoPassword(t *testing.T) {
	fake := storetest.NewFake()
	var out bytes.Buffer

	err := install([]string{"-name", "Malee", "-email", "malee@example.com", "-password", "secret123"}, &out, issueWith(fake))

	if err == nil {
		t.Fatal("err = nil, want the -password flag refused")
	}
	if _, err := fake.GetOwner(context.Background()); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("an Owner was created anyway: err = %v", err)
	}
	if strings.Contains(out.String(), "secret123") {
		t.Errorf("output repeats the password:\n%s", out.String())
	}
}

func TestInstallTakesNoStrayArguments(t *testing.T) {
	fake := storetest.NewFake()
	var out bytes.Buffer

	err := install([]string{"-name", "Malee", "-email", "malee@example.com", "secret123"}, &out, issueWith(fake))

	if err == nil {
		t.Fatal("err = nil, want the stray argument refused")
	}
	if strings.Contains(out.String(), "secret123") {
		t.Errorf("output repeats the argument:\n%s", out.String())
	}
	if _, err := fake.GetOwner(context.Background()); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("an Owner was created anyway: err = %v", err)
	}
}

func TestInstallSaysWhatIsMissingForTheFirstOwner(t *testing.T) {
	var out bytes.Buffer

	err := install([]string{"-email", "malee@example.com"}, &out, issueWith(storetest.NewFake()))

	if !errors.Is(err, auth.ErrOwnerDetailsMissing) {
		t.Errorf("err = %v, want ErrOwnerDetailsMissing", err)
	}
}
