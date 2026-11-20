package storetest

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// hashed is a token's hash for a test: any distinct bytes will do.
func hashed(s string) []byte { return bytes.Repeat([]byte(s), 32)[:32] }

func customer(email string) store.Customer {
	return store.Customer{
		Name: "Somying", Email: email, Phone: "0812345678", Status: "active", CreatedAt: at,
		PasswordHash: pgtype.Text{String: "hash", Valid: true},
	}
}

// inUTC is c with its time in UTC, as the fake keeps it, whichever zone Postgres answers in.
func inUTC(c store.Customer) store.Customer {
	c.CreatedAt = c.CreatedAt.UTC()
	return c
}

// signupBy is a sign-up's request: when and from where, with no actor, which the store names
// from the new Customer (activity-log R2).
var signupBy = store.ActivityBy{At: at, IP: "203.0.113.9", UserAgent: "Safari"}

// signupGrant issues code for the email and exchanges it for grant: what the Storefront's
// first two sign-up steps do.
func signupGrant(t *testing.T, s Subject, email, code, grant string) {
	t.Helper()
	ctx := context.Background()
	if err := s.Store.IssueSignupCode(ctx, store.SignupCodeParams{Email: email, CodeHash: hashed(code), ExpiresAt: at.Add(10 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	ok, err := s.Store.VerifySignupCode(ctx, store.VerifyFirstTimeCodeParams{
		Email: email, CodeHash: hashed(code), GrantHash: hashed(grant), GrantExpiresAt: at.Add(10 * time.Minute), At: at,
	})
	if err != nil || !ok {
		t.Fatalf("verify: %v, %v", ok, err)
	}
}

func complete(grant string, hash string) store.CompleteSignupParams {
	return store.CompleteSignupParams{GrantHash: hashed(grant), At: at, Name: "Somying", Phone: "0812345678", PasswordHash: hash}
}

// runCustomers is the part of the contract that is Customers and their sign-up (customers
// R3-R5, R8, R9, R10; users-and-access R36).
func runCustomers(t *testing.T, newSubject func(t *testing.T) Subject) {
	ctx := context.Background()

	t.Run("finds a Customer by email whatever its letter case, and not a Deleted one", func(t *testing.T) {
		s := newSubject(t)
		added := s.AddCustomer(t, customer("Somying@Example.com"))
		gone := customer("gone@example.com")
		gone.Status = "inactive"
		gone.DeletedAt, gone.DeletedBy = pgtype.Timestamptz{Time: at, Valid: true}, pgtype.Int8{Int64: s.AddUser(t, ownerUser("malee@example.com")).ID, Valid: true}
		s.AddCustomer(t, gone)

		got, err := s.Store.GetCustomerByEmail(ctx, "sOMYING@example.COM")
		if err != nil || inUTC(got) != added || got.Email != "Somying@Example.com" {
			t.Errorf("got %+v, %v, want %+v", got, err, added)
		}
		if _, err := s.Store.GetCustomerByEmail(ctx, "gone@example.com"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("a Deleted Customer: err = %v, want ErrNotFound", err)
		}
		if _, err := s.Store.GetCustomerByEmail(ctx, "nobody@example.com"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("an unknown email: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("signs a Customer up only when the code, then the sign-up, are done", func(t *testing.T) {
		s := newSubject(t)
		signupGrant(t, s, "Somying@Example.com", "c1", "g1")
		if _, err := s.Store.GetCustomerByEmail(ctx, "somying@example.com"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("after the code: err = %v, want no Customer yet (customers R5)", err)
		}

		created, err := s.Store.CompleteSignup(ctx, complete("g1", "argon"), signupBy)

		if err != nil {
			t.Fatal(err)
		}
		if created.ID == 0 || created.Name != "Somying" || created.Email != "somying@example.com" || created.Phone != "0812345678" ||
			created.Status != "active" || created.PasswordHash.String != "argon" || !created.CreatedAt.Equal(at) {
			t.Errorf("created = %+v", created)
		}
		if got, err := s.Store.GetCustomerByEmail(ctx, "SOMYING@example.com"); err != nil || inUTC(got) != inUTC(created) {
			t.Errorf("found %+v, %v, want %+v", got, err, created)
		}
		if _, err := s.Store.CompleteSignup(ctx, complete("g1", "again"), signupBy); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("the grant used twice: err = %v, want ErrNotFound (single use)", err)
		}
	})

	t.Run("writes the sign-up's entry, with the Customer as the actor, in the same transaction", func(t *testing.T) {
		s := newSubject(t)
		signupGrant(t, s, "somying@example.com", "c1", "g1")

		created, err := s.Store.CompleteSignup(ctx, complete("g1", "argon"), signupBy)
		if err != nil {
			t.Fatal(err)
		}

		rows := listActivity(t, s, store.ListActivityLogParams{})
		if len(rows) != 1 {
			t.Fatalf("entries = %+v, want one", rows)
		}
		got := rows[0]
		if got.ActorKind != store.ActorCustomer || got.ActorID.Int64 != created.ID || got.ActorName != "Somying" || got.ActorEmail != "somying@example.com" || got.ActorRole != "" ||
			got.Action != store.ActionCreated || got.SubjectKind != store.SubjectCustomer || got.SubjectID != created.ID || got.SubjectName != "Somying" || got.SubjectRef != "somying@example.com" ||
			got.Ip != "203.0.113.9" || got.UserAgent != "Safari" || !got.At.Equal(at) {
			t.Errorf("entry = %+v", got)
		}
	})

	t.Run("saves no Customer when its entry fails", func(t *testing.T) {
		s := newSubject(t)
		signupGrant(t, s, "somying@example.com", "c1", "g1")
		bad := signupBy
		bad.UserAgent = "Safari\x00" // no text column holds a NUL, so the entry is refused

		_, err := s.Store.CompleteSignup(ctx, complete("g1", "argon"), bad)

		if err == nil {
			t.Fatal("completed although its entry failed")
		}
		if _, err := s.Store.GetCustomerByEmail(ctx, "somying@example.com"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("the Customer was saved although its entry failed: %v", err)
		}
		if _, err := s.Store.CompleteSignup(ctx, complete("g1", "argon"), signupBy); err != nil {
			t.Errorf("the grant was used up by the failed sign-up: %v", err)
		}
	})

	t.Run("refuses a name or phone number the rules do not allow, and takes their edges", func(t *testing.T) {
		s := newSubject(t)
		signupGrant(t, s, "somying@example.com", "c1", "g1")
		letters := func(n int) string { return string(bytes.Repeat([]byte("ก"), n)) }
		refused := map[string]store.CompleteSignupParams{}
		for name, change := range map[string]func(*store.CompleteSignupParams){
			"a blank name":              func(p *store.CompleteSignupParams) { p.Name = " 	" },
			"a name of 121 letters":     func(p *store.CompleteSignupParams) { p.Name = letters(121) },
			"a phone not starting at 0": func(p *store.CompleteSignupParams) { p.Phone = "812345678" },
			"a phone of 8 digits":       func(p *store.CompleteSignupParams) { p.Phone = "01234567" },
			"a phone of 11 digits":      func(p *store.CompleteSignupParams) { p.Phone = "08123456789" },
			"a phone with a dash":       func(p *store.CompleteSignupParams) { p.Phone = "081-234567" },
		} {
			p := complete("g1", "argon")
			change(&p)
			refused[name] = p
		}

		for name, p := range refused {
			if _, err := s.Store.CompleteSignup(ctx, p, signupBy); err == nil {
				t.Errorf("%s: signed up (customers R2, R4)", name)
			}
		}

		edge := complete("g1", "argon")
		edge.Name, edge.Phone = letters(120), "021234567"
		if c, err := s.Store.CompleteSignup(ctx, edge, signupBy); err != nil || c.Name != letters(120) || c.Phone != "021234567" {
			t.Errorf("a name of 120 letters and a phone of 9 digits: %+v, %v", c, err)
		}
	})

	t.Run("refuses a code for an email that has a Customer, and accepts a Deleted Customer's", func(t *testing.T) {
		s := newSubject(t)
		s.AddCustomer(t, customer("Somying@Example.com"))
		gone := customer("gone@example.com")
		gone.Status = "inactive"
		gone.DeletedAt, gone.DeletedBy = pgtype.Timestamptz{Time: at, Valid: true}, pgtype.Int8{Int64: s.AddUser(t, ownerUser("malee@example.com")).ID, Valid: true}
		s.AddCustomer(t, gone)

		err := s.Store.IssueSignupCode(ctx, store.SignupCodeParams{Email: "SOMYING@example.com", CodeHash: hashed("c1"), ExpiresAt: at.Add(time.Hour)})
		if !errors.Is(err, store.ErrCustomerEmailTaken) {
			t.Errorf("err = %v, want ErrCustomerEmailTaken (customers R6)", err)
		}
		signupGrant(t, s, "gone@example.com", "c2", "g2")
		fresh, err := s.Store.CompleteSignup(ctx, complete("g2", "argon"), signupBy)
		if err != nil || fresh.ID == gone.ID {
			t.Errorf("a new Customer for a Deleted Customer's email: %+v, %v", fresh, err)
		}
	})

	t.Run("refuses to complete a sign-up for an email a Customer took meanwhile", func(t *testing.T) {
		s := newSubject(t)
		signupGrant(t, s, "somying@example.com", "c1", "g1")
		s.AddCustomer(t, customer("Somying@Example.com"))

		_, err := s.Store.CompleteSignup(ctx, complete("g1", "argon"), signupBy)

		if !errors.Is(err, store.ErrCustomerEmailTaken) {
			t.Errorf("err = %v, want ErrCustomerEmailTaken (customers R3)", err)
		}
	})

	t.Run("a code works once, expires in time, and only the newest works", func(t *testing.T) {
		s := newSubject(t)
		issue := func(code string) {
			t.Helper()
			if err := s.Store.IssueSignupCode(ctx, store.SignupCodeParams{Email: "somying@example.com", CodeHash: hashed(code), ExpiresAt: at.Add(10 * time.Minute)}); err != nil {
				t.Fatal(err)
			}
		}
		verify := func(code string, when time.Time) bool {
			t.Helper()
			ok, err := s.Store.VerifySignupCode(ctx, store.VerifyFirstTimeCodeParams{
				Email: "SOMYING@example.com", CodeHash: hashed(code), GrantHash: hashed("g-" + code), GrantExpiresAt: when.Add(10 * time.Minute), At: when,
			})
			if err != nil {
				t.Fatal(err)
			}
			return ok
		}
		issue("old")
		issue("new")

		if verify("old", at) {
			t.Error("a replaced code worked (customers R8)")
		}
		if verify("new", at.Add(10*time.Minute)) {
			t.Error("a code worked at its expiry (customers R8)")
		}
		if !verify("new", at.Add(10*time.Minute-time.Second)) {
			t.Error("the newest code did not work just before it expired")
		}
		if verify("new", at.Add(time.Minute)) {
			t.Error("a code worked twice (customers R8)")
		}
		if verify("nothing", at) {
			t.Error("a code that was never issued worked")
		}
	})

	t.Run("five wrong codes block codes for the email for ten minutes, whatever is resent", func(t *testing.T) {
		s := newSubject(t)
		issue := func(code string, when time.Time) {
			t.Helper()
			if err := s.Store.IssueSignupCode(ctx, store.SignupCodeParams{Email: "somying@example.com", CodeHash: hashed(code), ExpiresAt: when.Add(10 * time.Minute)}); err != nil {
				t.Fatal(err)
			}
		}
		verify := func(code string, when time.Time) bool {
			t.Helper()
			ok, err := s.Store.VerifySignupCode(ctx, store.VerifyFirstTimeCodeParams{
				Email: "somying@example.com", CodeHash: hashed(code), GrantHash: hashed("g"), GrantExpiresAt: when.Add(10 * time.Minute), At: when,
			})
			if err != nil {
				t.Fatal(err)
			}
			return ok
		}
		issue("right", at)
		for range 4 {
			verify("wrong", at)
		}
		issue("right2", at.Add(time.Minute)) // a resend neither resets the attempts nor lifts a block
		verify("wrong", at.Add(time.Minute))

		if verify("right2", at.Add(2*time.Minute)) {
			t.Error("the right code worked during the block (customers R8)")
		}
		issue("right3", at.Add(3*time.Minute))
		if verify("right3", at.Add(time.Minute+10*time.Minute-time.Second)) {
			t.Error("the block ended early")
		}
		if !verify("right3", at.Add(time.Minute+10*time.Minute)) {
			t.Error("the block did not end after ten minutes")
		}
	})

	t.Run("four wrong codes still allow the right one", func(t *testing.T) {
		s := newSubject(t)
		if err := s.Store.IssueSignupCode(ctx, store.SignupCodeParams{Email: "somying@example.com", CodeHash: hashed("right"), ExpiresAt: at.Add(10 * time.Minute)}); err != nil {
			t.Fatal(err)
		}
		check := func(code string) bool {
			ok, err := s.Store.VerifySignupCode(ctx, store.VerifyFirstTimeCodeParams{
				Email: "somying@example.com", CodeHash: hashed(code), GrantHash: hashed("g"), GrantExpiresAt: at.Add(10 * time.Minute), At: at,
			})
			if err != nil {
				t.Fatal(err)
			}
			return ok
		}
		for range 4 {
			check("wrong")
		}
		if !check("right") {
			t.Error("four wrong attempts blocked the right code")
		}
	})

	t.Run("a sign-up grant expires with its ten minutes", func(t *testing.T) {
		s := newSubject(t)
		signupGrant(t, s, "somying@example.com", "c1", "g1")
		late := complete("g1", "argon")
		late.At = at.Add(10 * time.Minute)

		if _, err := s.Store.CompleteSignup(ctx, late, signupBy); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound for an expired grant", err)
		}
		if err := s.Store.ReadSignupGrant(ctx, store.SignupGrantParams{TokenHash: hashed("g1"), At: at.Add(10 * time.Minute)}); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("read: err = %v, want ErrNotFound for an expired grant", err)
		}
		if err := s.Store.ReadSignupGrant(ctx, store.SignupGrantParams{TokenHash: hashed("g1"), At: at}); err != nil {
			t.Errorf("read: %v", err)
		}
	})

	t.Run("a Customer's session starts for the Customer as they were, and ends on its own", func(t *testing.T) {
		s := newSubject(t)
		c := s.AddCustomer(t, customer("somying@example.com"))
		entry := func() store.AddLoginLogEntryParams {
			return store.AddLoginLogEntryParams{
				At: at, Email: "somying@example.com", CustomerID: pgtype.Int8{Int64: c.ID, Valid: true},
				App: pgtype.Text{String: store.AppStorefront, Valid: true}, Ip: "203.0.113.9", UserAgent: "Safari",
			}
		}
		start := func(token string, hash pgtype.Text) bool {
			t.Helper()
			started, err := s.Store.StartCustomerSession(ctx, store.StartCustomerSessionParams{
				Session: store.CreateCustomerSessionParams{TokenHash: hashed(token), CustomerID: c.ID, ExpiresAt: at.Add(7 * 24 * time.Hour), PasswordHash: hash},
				Entry:   entry(),
			})
			if err != nil {
				t.Fatal(err)
			}
			return started
		}

		if !start("one", c.PasswordHash) || !start("two", c.PasswordHash) {
			t.Fatal("a Customer cannot be logged in on two devices (customers R10)")
		}
		if start("stale", pgtype.Text{String: "other", Valid: true}) {
			t.Error("a session started although the password changed while it was checked")
		}
		got, err := s.Store.GetCustomerSession(ctx, hashed("one"))
		if err != nil || got.CustomerID != c.ID || got.Name != "Somying" || got.Email != "somying@example.com" || got.Phone != "0812345678" || !got.ExpiresAt.Equal(at.Add(7*24*time.Hour)) {
			t.Errorf("session = %+v, %v", got, err)
		}

		if err := s.Store.EndCustomerSession(ctx, store.EndCustomerSessionParams{TokenHash: hashed("one"), Entry: entry()}); err != nil {
			t.Fatal(err)
		}
		if err := s.Store.EndCustomerSession(ctx, store.EndCustomerSessionParams{TokenHash: hashed("one"), Entry: entry()}); err != nil {
			t.Fatal(err)
		}

		if _, err := s.Store.GetCustomerSession(ctx, hashed("one")); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("the ended session: err = %v, want ErrNotFound", err)
		}
		if _, err := s.Store.GetCustomerSession(ctx, hashed("two")); err != nil {
			t.Errorf("logout ended the other device's session (customers R10): %v", err)
		}
		var results []string
		for _, row := range listLog(t, s, store.ListLoginLogParams{App: pgtype.Text{String: store.AppStorefront, Valid: true}}) {
			if row.CustomerID.Int64 != c.ID || row.CustomerName.String != "Somying" || row.UserID.Valid {
				t.Errorf("entry = %+v, want the Customer", row)
			}
			results = append(results, row.Result)
		}
		want := []string{"logout", "failed-login", "login", "login"}
		if len(results) != len(want) {
			t.Fatalf("results = %v, want %v: one logout, not two", results, want)
		}
		for i := range want {
			if results[i] != want[i] {
				t.Fatalf("results = %v, want %v", results, want)
			}
		}
	})

	t.Run("no session starts for an Inactive or Deleted Customer", func(t *testing.T) {
		s := newSubject(t)
		inactive := customer("inactive@example.com")
		inactive.Status = "inactive"
		c := s.AddCustomer(t, inactive)

		started, err := s.Store.StartCustomerSession(ctx, store.StartCustomerSessionParams{
			Session: store.CreateCustomerSessionParams{TokenHash: hashed("x"), CustomerID: c.ID, ExpiresAt: at.Add(time.Hour), PasswordHash: c.PasswordHash},
			Entry: store.AddLoginLogEntryParams{
				At: at, Email: c.Email, CustomerID: pgtype.Int8{Int64: c.ID, Valid: true}, App: pgtype.Text{String: store.AppStorefront, Valid: true},
			},
		})

		if err != nil || started {
			t.Errorf("started = %v, %v, want a refusal (customers R9)", started, err)
		}
	})

	t.Run("a Storefront entry names a Customer who exists, and a StockFlow entry names none", func(t *testing.T) {
		s := newSubject(t)
		c := s.AddCustomer(t, customer("somying@example.com"))
		add := func(app string, customerID int64) error {
			return s.Store.AddLoginLogEntry(ctx, store.AddLoginLogEntryParams{
				At: at, Email: "x@example.com", Result: store.ResultFailedLogin, App: pgtype.Text{String: app, Valid: true},
				CustomerID: pgtype.Int8{Int64: customerID, Valid: customerID != 0},
			})
		}

		if err := add(store.AppStorefront, c.ID+100); err == nil {
			t.Error("an entry named a Customer who does not exist")
		}
		if err := add(store.AppStockFlow, c.ID); err == nil {
			t.Error("a StockFlow entry named a Customer")
		}
		if err := add(store.AppStorefront, c.ID); err != nil {
			t.Error(err)
		}
	})
}
