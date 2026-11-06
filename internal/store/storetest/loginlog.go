package storetest

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func entry(email string, user store.User, ip string) store.AddLoginLogEntryParams {
	return store.AddLoginLogEntryParams{
		At: at, Email: email, UserID: pgtype.Int8{Int64: user.ID, Valid: user.ID != 0}, Result: store.ResultFailedLogin,
		Ip: ip, UserAgent: "Firefox",
	}
}

func listLog(t *testing.T, s Subject, arg store.ListLoginLogParams) []store.ListLoginLogRow {
	t.Helper()
	if arg.RowLimit == 0 {
		arg.RowLimit = 100
	}
	rows, err := s.Store.ListLoginLog(context.Background(), arg)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// runLoginLog is the part of the contract that is the Login log (users-and-access R36–R39).
func runLoginLog(t *testing.T, newSubject func(t *testing.T) Subject) {
	ctx := context.Background()

	t.Run("keeps a failed login's email as typed, with a User only when it matches one", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		if err := s.Store.AddLoginLogEntry(ctx, entry("SOMCHAI@example.com", user, "203.0.113.7")); err != nil {
			t.Fatal(err)
		}
		if err := s.Store.AddLoginLogEntry(ctx, entry("Nobody@Example.com", store.User{}, "2001:db8::1")); err != nil {
			t.Fatal(err)
		}

		rows := listLog(t, s, store.ListLoginLogParams{})

		if len(rows) != 2 {
			t.Fatalf("rows = %+v, want 2", rows)
		}
		nobody, known := rows[0], rows[1]
		if nobody.Email != "Nobody@Example.com" || nobody.UserID.Valid || nobody.UserName.Valid || nobody.Ip != "2001:db8::1" {
			t.Errorf("unmatched entry = %+v", nobody)
		}
		if known.Email != "SOMCHAI@example.com" || known.UserID.Int64 != user.ID || known.UserName.String != "Somchai" ||
			known.Result != "failed-login" || !known.At.Equal(at) || known.Ip != "203.0.113.7" || known.UserAgent != "Firefox" {
			t.Errorf("matched entry = %+v", known)
		}
	})

	t.Run("refuses an entry with an unknown result or an unknown User", func(t *testing.T) {
		s := newSubject(t)
		bad := entry("somchai@example.com", store.User{}, "203.0.113.7")
		bad.Result = "success"
		if err := s.Store.AddLoginLogEntry(ctx, bad); err == nil {
			t.Error("unknown result: err = nil, want a refusal")
		}
		if err := s.Store.AddLoginLogEntry(ctx, entry("somchai@example.com", store.User{ID: 404}, "203.0.113.7")); err == nil {
			t.Error("unknown User: err = nil, want a refusal")
		}
	})

	t.Run("lists the newest entries first, a page at a time", func(t *testing.T) {
		s := newSubject(t)
		for _, email := range []string{"a@example.com", "b@example.com", "c@example.com", "d@example.com", "e@example.com"} {
			if err := s.Store.AddLoginLogEntry(ctx, entry(email, store.User{}, "203.0.113.7")); err != nil {
				t.Fatal(err)
			}
		}

		first := listLog(t, s, store.ListLoginLogParams{RowLimit: 2})
		second := listLog(t, s, store.ListLoginLogParams{RowLimit: 2, Before: pgtype.Int8{Int64: first[1].ID, Valid: true}})
		third := listLog(t, s, store.ListLoginLogParams{RowLimit: 2, Before: pgtype.Int8{Int64: second[1].ID, Valid: true}})

		var got []string
		for _, page := range [][]store.ListLoginLogRow{first, second, third} {
			for _, row := range page {
				got = append(got, row.Email)
			}
		}
		want := []string{"e@example.com", "d@example.com", "c@example.com", "b@example.com", "a@example.com"}
		if !slices.Equal(got, want) {
			t.Errorf("emails = %v, want %v", got, want)
		}
	})

	t.Run("lists no more than the limit", func(t *testing.T) {
		s := newSubject(t)
		for range 3 {
			if err := s.Store.AddLoginLogEntry(ctx, entry("a@example.com", store.User{}, "203.0.113.7")); err != nil {
				t.Fatal(err)
			}
		}

		if rows := listLog(t, s, store.ListLoginLogParams{RowLimit: 1}); len(rows) != 1 {
			t.Errorf("rows = %d, want 1", len(rows))
		}
	})

	t.Run("filters the log by User and by result", func(t *testing.T) {
		s := newSubject(t)
		somchai := s.AddUser(t, staff("somchai@example.com"))
		malee := s.AddUser(t, staff("malee@example.com"))
		out := entry("malee@example.com", malee, "203.0.113.7")
		out.Result = store.ResultLogout
		for _, e := range []store.AddLoginLogEntryParams{
			entry("somchai@example.com", somchai, "203.0.113.7"),
			entry("malee@example.com", malee, "203.0.113.7"),
			entry("nobody@example.com", store.User{}, "203.0.113.7"),
			out,
		} {
			if err := s.Store.AddLoginLogEntry(ctx, e); err != nil {
				t.Fatal(err)
			}
		}
		user := func(id int64) pgtype.Int8 { return pgtype.Int8{Int64: id, Valid: true} }
		result := func(r string) pgtype.Text { return pgtype.Text{String: r, Valid: true} }

		for name, c := range map[string]struct {
			arg  store.ListLoginLogParams
			want int
		}{
			"no filter":            {store.ListLoginLogParams{}, 4},
			"a User":               {store.ListLoginLogParams{UserID: user(malee.ID)}, 2},
			"a result":             {store.ListLoginLogParams{Result: result(store.ResultFailedLogin)}, 3},
			"a User and a result":  {store.ListLoginLogParams{UserID: user(malee.ID), Result: result(store.ResultLogout)}, 1},
			"a result nobody has":  {store.ListLoginLogParams{Result: result(store.ResultLogin)}, 0},
			"a User with no entry": {store.ListLoginLogParams{UserID: user(404)}, 0},
		} {
			if got := len(listLog(t, s, c.arg)); got != c.want {
				t.Errorf("%s: rows = %d, want %d", name, got, c.want)
			}
		}
	})

	t.Run("starting a session writes a login entry, and a refused one a failed login", func(t *testing.T) {
		s := newSubject(t)
		hash := pgtype.Text{String: "hash", Valid: true}
		active := staff("active@example.com")
		active.PasswordHash = hash
		active = s.AddUser(t, active)
		inactive := staff("inactive@example.com")
		inactive.Status, inactive.PasswordHash = "inactive", hash
		inactive = s.AddUser(t, inactive)
		start := func(user store.User, token string) bool {
			started, err := s.Store.StartSession(ctx, store.StartSessionParams{
				Session: store.CreateSessionParams{TokenHash: []byte(token), UserID: user.ID, ExpiresAt: at, PasswordHash: hash},
				// The result the caller leaves here is not trusted: the outcome decides it.
				Entry: entry(user.Email, user, "203.0.113.7"),
			})
			if err != nil {
				t.Fatal(err)
			}
			return started
		}

		if !start(active, "one") || start(inactive, "two") {
			t.Fatal("want the Active User started and the Inactive one refused")
		}

		rows := listLog(t, s, store.ListLoginLogParams{})
		if len(rows) != 2 || rows[1].Result != "login" || rows[1].UserID.Int64 != active.ID ||
			rows[0].Result != "failed-login" || rows[0].UserID.Int64 != inactive.ID {
			t.Errorf("rows = %+v, want a login then a failed login", rows)
		}
	})

	t.Run("a login whose entry cannot be written starts no session", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		bad := entry(user.Email, store.User{ID: 404}, "203.0.113.7")

		_, err := s.Store.StartSession(ctx, store.StartSessionParams{
			Session: store.CreateSessionParams{TokenHash: []byte("h"), UserID: user.ID, ExpiresAt: at},
			Entry:   bad,
		})

		if err == nil {
			t.Error("err = nil, want a refusal")
		}
		if _, err := s.Store.GetSession(ctx, []byte("h")); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("session: err = %v, want none", err)
		}
	})

	t.Run("ending a session writes a logout entry and leaves the other sessions", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		for _, hash := range []string{"one", "two"} {
			if _, err := s.Store.CreateSession(ctx, store.CreateSessionParams{TokenHash: []byte(hash), UserID: user.ID, ExpiresAt: at}); err != nil {
				t.Fatal(err)
			}
		}

		err := s.Store.EndSession(ctx, store.EndSessionParams{TokenHash: []byte("one"), Entry: entry(user.Email, user, "203.0.113.7")})

		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Store.GetSession(ctx, []byte("one")); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("ended session: err = %v, want ErrNotFound", err)
		}
		if _, err := s.Store.GetSession(ctx, []byte("two")); err != nil {
			t.Errorf("other session: %v", err)
		}
		if rows := listLog(t, s, store.ListLoginLogParams{}); len(rows) != 1 || rows[0].Result != "logout" || rows[0].UserID.Int64 != user.ID {
			t.Errorf("rows = %+v, want one logout", rows)
		}
	})

	t.Run("a logout whose entry cannot be written keeps the session", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		if _, err := s.Store.CreateSession(ctx, store.CreateSessionParams{TokenHash: []byte("one"), UserID: user.ID, ExpiresAt: at}); err != nil {
			t.Fatal(err)
		}
		bad := entry(user.Email, store.User{ID: 404}, "203.0.113.7")

		err := s.Store.EndSession(ctx, store.EndSessionParams{TokenHash: []byte("one"), Entry: bad})

		if err == nil {
			t.Error("err = nil, want a refusal")
		}
		if _, err := s.Store.GetSession(ctx, []byte("one")); err != nil {
			t.Errorf("session: %v, want it kept", err)
		}
	})

	// Two logouts with one cookie can both pass the live-session check; only one ends it.
	t.Run("ending a session that is already gone writes no entry", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("staff@example.com"))
		if _, err := s.Store.CreateSession(ctx, store.CreateSessionParams{TokenHash: []byte("one"), UserID: user.ID, ExpiresAt: at}); err != nil {
			t.Fatal(err)
		}
		end := store.EndSessionParams{TokenHash: []byte("one"), Entry: entry(user.Email, user, "203.0.113.7")}

		for range 2 {
			if err := s.Store.EndSession(ctx, end); err != nil {
				t.Fatal(err)
			}
		}

		if rows := listLog(t, s, store.ListLoginLogParams{}); len(rows) != 1 {
			t.Errorf("rows = %+v, want one logout", rows)
		}
	})
}
