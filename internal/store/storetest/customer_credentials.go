package storetest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func noPassword(c store.Customer) store.Customer {
	c.PasswordHash = pgtype.Text{}
	return c
}

func address() store.DeliveryAddress {
	return store.DeliveryAddress{
		Recipient: "Somying Jaidee", Phone: "0812345678", Line: "99/1 Rama IV Road", Subdistrict: "Silom",
		District: "Bang Rak", Province: "Bangkok", Postcode: "10500",
	}
}

func ptr(s string) *string { return &s }

// runCustomerCredentials is the part of the contract that is a Customer's password setup,
// recovery and profile (customers R7, R8, R12, R13, R15; activity-log R4).
func runCustomerCredentials(t *testing.T, newSubject func(t *testing.T) Subject) {
	ctx := context.Background()
	issue := func(t *testing.T, s Subject, address, kind, token string, expires time.Time) error {
		t.Helper()
		_, err := s.Store.IssueCustomerCredential(ctx, store.EmailCredentialParams{Email: address, Kind: kind, TokenHash: hashed(token), ExpiresAt: expires})
		return err
	}
	verify := func(t *testing.T, s Subject, address, code, grant string, when time.Time) bool {
		t.Helper()
		ok, err := s.Store.VerifyCustomerCode(ctx, store.VerifyFirstTimeCodeParams{
			Email: address, CodeHash: hashed(code), GrantHash: hashed(grant), GrantExpiresAt: when.Add(10 * time.Minute), At: when,
		})
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	setPassword := func(s Subject, kind, token string, when time.Time) (store.Customer, error) {
		return s.Store.SetCustomerPassword(ctx, store.EmailPasswordParams{Kind: kind, TokenHash: hashed(token), At: when, PasswordHash: "new-hash"}, signupBy)
	}

	t.Run("issues a code only to an Active Customer without a password, and a Reset link only to one with", func(t *testing.T) {
		s := newSubject(t)
		s.AddCustomer(t, noPassword(customer("none@example.com")))
		s.AddCustomer(t, customer("has@example.com"))
		inactive := noPassword(customer("inactive@example.com"))
		inactive.Status = "inactive"
		s.AddCustomer(t, inactive)
		gone := noPassword(customer("gone@example.com"))
		gone.Status, gone.DeletedAt, gone.DeletedBy = "inactive", pgtype.Timestamptz{Time: at, Valid: true}, pgtype.Int8{Int64: s.AddUser(t, ownerUser("malee@example.com")).ID, Valid: true}
		s.AddCustomer(t, gone)
		expires := at.Add(time.Hour)

		for name, c := range map[string]struct {
			email, kind string
			ok          bool
		}{
			"a code for one without a password":       {"None@Example.com", store.FirstTimeCode, true},
			"a Reset link for one with a password":    {"HAS@example.com", store.ResetLink, true},
			"a code for one with a password":          {"has@example.com", store.FirstTimeCode, false},
			"a Reset link for one without a password": {"none@example.com", store.ResetLink, false},
			"a code for an Inactive Customer":         {"inactive@example.com", store.FirstTimeCode, false},
			"a code for a Deleted Customer":           {"gone@example.com", store.FirstTimeCode, false},
			"a code for nobody":                       {"nobody@example.com", store.FirstTimeCode, false},
		} {
			err := issue(t, s, c.email, c.kind, name, expires)
			if c.ok && err != nil {
				t.Errorf("%s: %v", name, err)
			}
			if !c.ok && !errors.Is(err, store.ErrNotFound) {
				t.Errorf("%s: err = %v, want ErrNotFound (customers R7, R12, R15)", name, err)
			}
		}
	})

	t.Run("a code sets the first password once, and the entry names the Customer", func(t *testing.T) {
		s := newSubject(t)
		c := s.AddCustomer(t, noPassword(customer("somying@example.com")))
		if err := issue(t, s, "somying@example.com", store.FirstTimeCode, "code", at.Add(10*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if !verify(t, s, "somying@example.com", "code", "grant", at) {
			t.Fatal("the right code was refused")
		}
		if verify(t, s, "somying@example.com", "code", "grant2", at) {
			t.Error("a code worked twice (customers R8)")
		}

		got, err := setPassword(s, store.SetupGrant, "grant", at)

		if err != nil || got.ID != c.ID || got.PasswordHash.String != "new-hash" {
			t.Fatalf("got %+v, %v", got, err)
		}
		if _, err := setPassword(s, store.SetupGrant, "grant", at); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("the grant used twice: err = %v, want ErrNotFound", err)
		}
		if found, _ := s.Store.GetCustomerByEmail(ctx, "somying@example.com"); found.PasswordHash.String != "new-hash" {
			t.Errorf("stored password = %q", found.PasswordHash.String)
		}
		rows := listActivity(t, s, store.ListActivityLogParams{})
		if len(rows) != 1 || rows[0].Action != store.ActionPasswordSet || rows[0].ActorKind != store.ActorCustomer || rows[0].ActorID.Int64 != c.ID ||
			rows[0].SubjectKind != store.SubjectCustomer || rows[0].SubjectRef != "somying@example.com" || string(rows[0].Changes) != "[]" || rows[0].Ip != "203.0.113.9" {
			t.Errorf("entries = %+v, want one password-set by the Customer with no field (activity-log R4, R6)", rows)
		}
	})

	t.Run("a code expires in time, only the newest works, and five wrong ones block", func(t *testing.T) {
		s := newSubject(t)
		s.AddCustomer(t, noPassword(customer("somying@example.com")))
		_ = issue(t, s, "somying@example.com", store.FirstTimeCode, "old", at.Add(10*time.Minute))
		_ = issue(t, s, "somying@example.com", store.FirstTimeCode, "new", at.Add(10*time.Minute))

		if verify(t, s, "somying@example.com", "old", "g", at) {
			t.Error("a replaced code worked (customers R8)")
		}
		if verify(t, s, "somying@example.com", "new", "g", at.Add(10*time.Minute)) {
			t.Error("a code worked at its expiry (customers R8)")
		}
		for range 4 {
			verify(t, s, "somying@example.com", "wrong", "g", at)
		}
		// The replaced code counted too: that is five wrong.
		if verify(t, s, "somying@example.com", "new", "g", at.Add(time.Minute)) {
			t.Error("the right code worked during a block (customers R8)")
		}
		_ = issue(t, s, "somying@example.com", store.FirstTimeCode, "newer", at.Add(30*time.Minute))
		if verify(t, s, "somying@example.com", "newer", "g", at.Add(10*time.Minute-time.Second)) {
			t.Error("the block ended early, or a resend lifted it")
		}
		if !verify(t, s, "somying@example.com", "newer", "g", at.Add(10*time.Minute)) {
			t.Error("the block did not end after ten minutes")
		}
	})

	t.Run("a Reset link sets the password once, and ends every session of that Customer only", func(t *testing.T) {
		s := newSubject(t)
		c := s.AddCustomer(t, customer("somying@example.com"))
		other := s.AddCustomer(t, customer("other@example.com"))
		login := func(c store.Customer, token string) {
			t.Helper()
			ok, err := s.Store.StartCustomerSession(ctx, store.StartCustomerSessionParams{
				Session: store.CreateCustomerSessionParams{TokenHash: hashed(token), CustomerID: c.ID, ExpiresAt: at.Add(time.Hour), PasswordHash: c.PasswordHash},
				Entry: store.AddLoginLogEntryParams{
					At: at, Email: c.Email, CustomerID: pgtype.Int8{Int64: c.ID, Valid: true}, App: pgtype.Text{String: store.AppStorefront, Valid: true},
				},
			})
			if err != nil || !ok {
				t.Fatalf("login: %v, %v", ok, err)
			}
		}
		login(c, "phone")
		login(c, "laptop")
		login(other, "theirs")
		_ = issue(t, s, "somying@example.com", store.ResetLink, "first-link", at.Add(30*time.Minute))
		_ = issue(t, s, "somying@example.com", store.ResetLink, "link", at.Add(30*time.Minute))

		if _, err := setPassword(s, store.ResetLink, "first-link", at); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("a replaced link: err = %v, want ErrNotFound (customers R12)", err)
		}
		if _, err := setPassword(s, store.ResetLink, "link", at.Add(30*time.Minute)); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("a link at its expiry: err = %v, want ErrNotFound (customers R12)", err)
		}
		if _, err := setPassword(s, store.SetupGrant, "link", at); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("a link used as a setup grant: err = %v, want ErrNotFound", err)
		}
		got, err := setPassword(s, store.ResetLink, "link", at.Add(30*time.Minute-time.Second))

		if err != nil || got.PasswordHash.String != "new-hash" {
			t.Fatalf("got %+v, %v", got, err)
		}
		if _, err := setPassword(s, store.ResetLink, "link", at); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("a link used twice: err = %v, want ErrNotFound (customers R12)", err)
		}
		for _, token := range []string{"phone", "laptop"} {
			if _, err := s.Store.GetCustomerSession(ctx, hashed(token)); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("session %s: err = %v, want ended (customers R12)", token, err)
			}
		}
		if _, err := s.Store.GetCustomerSession(ctx, hashed("theirs")); err != nil {
			t.Errorf("another Customer's session ended: %v", err)
		}
		rows := listActivity(t, s, store.ListActivityLogParams{})
		if len(rows) != 1 || rows[0].Action != store.ActionPasswordReset || rows[0].ActorID.Int64 != c.ID {
			t.Errorf("entries = %+v, want one password-reset by the Customer (activity-log R4)", rows)
		}
	})

	t.Run("saves no password when its entry fails", func(t *testing.T) {
		s := newSubject(t)
		s.AddCustomer(t, customer("somying@example.com"))
		_ = issue(t, s, "somying@example.com", store.ResetLink, "link", at.Add(time.Hour))
		bad := signupBy
		bad.UserAgent = "Safari\x00"

		_, err := s.Store.SetCustomerPassword(ctx, store.EmailPasswordParams{Kind: store.ResetLink, TokenHash: hashed("link"), At: at, PasswordHash: "new-hash"}, bad)

		if err == nil {
			t.Fatal("changed the password although its entry failed")
		}
		if got, _ := s.Store.GetCustomerByEmail(ctx, "somying@example.com"); got.PasswordHash.String != "hash" {
			t.Errorf("password = %q, want it unchanged", got.PasswordHash.String)
		}
		if _, err := setPassword(s, store.ResetLink, "link", at); err != nil {
			t.Errorf("the link was used up by the failed change: %v", err)
		}
	})

	t.Run("changes name, phone number and Delivery address, each with its entry before and after", func(t *testing.T) {
		s := newSubject(t)
		c := s.AddCustomer(t, customer("somying@example.com"))
		addr := address()

		got, err := s.Store.UpdateProfile(ctx, store.ChangeProfileParams{ID: c.ID, Name: ptr("Somying J."), Phone: ptr("021234567"), Address: &addr}, signupBy)

		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "Somying J." || got.Phone != "021234567" || got.Email != "somying@example.com" || got.DeliveryAddress() == nil || *got.DeliveryAddress() != addr {
			t.Errorf("got %+v", got)
		}
		rows := listActivity(t, s, store.ListActivityLogParams{})
		if len(rows) != 3 {
			t.Fatalf("entries = %+v, want three", rows)
		}
		want := map[string][2]string{
			store.ActionNameChanged:    {`"Somying"`, `"Somying J."`},
			store.ActionPhoneChanged:   {`"0812345678"`, `"021234567"`},
			store.ActionAddressChanged: {`null`, mustJSON(addr)},
		}
		for _, r := range rows {
			var changes []struct {
				Field         string
				Before, After json.RawMessage
			}
			if err := json.Unmarshal(r.Changes, &changes); err != nil || len(changes) != 1 {
				t.Fatalf("%s changes = %s, %v", r.Action, r.Changes, err)
			}
			w, ok := want[r.Action]
			if !ok || canon(changes[0].Before) != canon(json.RawMessage(w[0])) || canon(changes[0].After) != canon(json.RawMessage(w[1])) || r.ActorKind != store.ActorCustomer || r.ActorID.Int64 != c.ID || r.SubjectName != "Somying J." {
				t.Errorf("entry %s = %s", r.Action, r.Changes)
			}
		}

		// A second change shows the address before it, and an unchanged field writes nothing.
		next := address()
		next.Postcode = "10110"
		if _, err := s.Store.UpdateProfile(ctx, store.ChangeProfileParams{ID: c.ID, Name: ptr("Somying J."), Address: &next}, signupBy); err != nil {
			t.Fatal(err)
		}
		rows = listActivity(t, s, store.ListActivityLogParams{})
		if len(rows) != 4 || rows[0].Action != store.ActionAddressChanged || !sameJSON(rows[0].Changes, `[{"field":"deliveryAddress","before":`+mustJSON(addr)+`,"after":`+mustJSON(next)+`}]`) {
			t.Errorf("entries = %+v, want one more: the address, from 10500 to 10110", rows)
		}
	})

	t.Run("refuses a profile the rules do not allow, and takes its edges", func(t *testing.T) {
		s := newSubject(t)
		c := s.AddCustomer(t, customer("somying@example.com"))
		letters := func(n int) string { return string(bytes.Repeat([]byte("ก"), n)) }
		with := func(change func(*store.DeliveryAddress)) store.ChangeProfileParams {
			a := address()
			change(&a)
			return store.ChangeProfileParams{ID: c.ID, Address: &a}
		}
		for name, p := range map[string]store.ChangeProfileParams{
			"a blank name":             {ID: c.ID, Name: ptr("  ")},
			"a name of 121":            {ID: c.ID, Name: ptr(letters(121))},
			"a phone of 8 digits":      {ID: c.ID, Phone: ptr("01234567")},
			"a blank recipient":        with(func(a *store.DeliveryAddress) { a.Recipient = " " }),
			"a recipient of 121":       with(func(a *store.DeliveryAddress) { a.Recipient = letters(121) }),
			"a recipient phone of 11":  with(func(a *store.DeliveryAddress) { a.Phone = "08123456789" }),
			"a line of 201":            with(func(a *store.DeliveryAddress) { a.Line = letters(201) }),
			"a blank line":             with(func(a *store.DeliveryAddress) { a.Line = "" }),
			"a sub-district of 81":     with(func(a *store.DeliveryAddress) { a.Subdistrict = letters(81) }),
			"a district of 81":         with(func(a *store.DeliveryAddress) { a.District = letters(81) }),
			"a blank province":         with(func(a *store.DeliveryAddress) { a.Province = "" }),
			"a postcode of 4 digits":   with(func(a *store.DeliveryAddress) { a.Postcode = "1050" }),
			"a postcode of 6 digits":   with(func(a *store.DeliveryAddress) { a.Postcode = "105000" }),
			"a postcode with a letter": with(func(a *store.DeliveryAddress) { a.Postcode = "1050a" }),
		} {
			if _, err := s.Store.UpdateProfile(ctx, p, signupBy); err == nil {
				t.Errorf("%s: saved (customers R4, storefront R7)", name)
			}
		}
		edge := with(func(a *store.DeliveryAddress) {
			a.Recipient, a.Line, a.Subdistrict, a.District = letters(120), letters(200), letters(80), letters(80)
		})
		edge.Name = ptr(letters(120))
		if _, err := s.Store.UpdateProfile(ctx, edge, signupBy); err != nil {
			t.Errorf("the longest of each: %v", err)
		}
	})

	t.Run("saves no profile change when its entry fails, and knows no such Customer", func(t *testing.T) {
		s := newSubject(t)
		c := s.AddCustomer(t, customer("somying@example.com"))
		bad := signupBy
		bad.UserAgent = "Safari\x00"

		if _, err := s.Store.UpdateProfile(ctx, store.ChangeProfileParams{ID: c.ID, Name: ptr("Changed")}, bad); err == nil {
			t.Error("saved although its entry failed")
		}
		if got, _ := s.Store.GetCustomerByEmail(ctx, "somying@example.com"); got.Name != "Somying" {
			t.Errorf("name = %q, want it unchanged", got.Name)
		}
		if _, err := s.Store.UpdateProfile(ctx, store.ChangeProfileParams{ID: c.ID + 50, Name: ptr("x")}, signupBy); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("no such Customer: err = %v, want ErrNotFound", err)
		}
	})
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// canon is JSON in one form, whichever spacing and key order Postgres's jsonb gives it.
func canon(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return mustJSON(v)
}

func sameJSON(got []byte, want string) bool { return canon(got) == canon(json.RawMessage(want)) }
