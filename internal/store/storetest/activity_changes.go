package storetest

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// chronological is the entries of a filter, oldest first.
func chronological(t *testing.T, s Subject, kind string) []store.ActivityLog {
	t.Helper()
	rows := listActivity(t, s, store.ListActivityLogParams{SubjectKind: pgtype.Text{String: kind, Valid: true}})
	slices.Reverse(rows)
	return rows
}

func actionsOf(rows []store.ActivityLog) []string {
	var got []string
	for _, r := range rows {
		got = append(got, r.Action)
	}
	return got
}

func changesOf(t *testing.T, r store.ActivityLog) []store.FieldChange {
	t.Helper()
	var changes []store.FieldChange
	if err := json.Unmarshal(r.Changes, &changes); err != nil {
		t.Fatal(err)
	}
	return changes
}

// runActivityOfUsersAndOrders is the part of the contract that is the Activity log of User
// and Order changes (activity-log R2-R4, R6, R11): each writes its entry with the change, or
// neither is saved.
func runActivityOfUsersAndOrders(t *testing.T, newSubject func(t *testing.T) Subject) {
	ctx := context.Background()

	t.Run("writes an entry for each User change, and none holds a secret", func(t *testing.T) {
		s := newSubject(t)
		boss := s.AddUser(t, ownerUser("malee@example.com"))
		by := userBy(boss.ID, at)
		created, err := s.Store.CreateUser(ctx, store.CreateUserParams{
			Name: "Somchai", Email: "somchai@example.com", Role: "staff", TokenHash: []byte("secret-link-1"), ExpiresAt: at.Add(time.Hour),
		}, by)
		if err != nil {
			t.Fatal(err)
		}
		change := func(arg store.ChangeUserParams) {
			t.Helper()
			arg.ID = created.ID
			if _, err := s.Store.UpdateUser(ctx, arg, by); err != nil {
				t.Fatal(err)
			}
		}
		change(store.ChangeUserParams{Name: "Somchai P.", Role: "admin"})
		change(store.ChangeUserParams{Name: "Somchai P."}) // nothing changes: no entry
		change(store.ChangeUserParams{Status: "inactive"})
		change(store.ChangeUserParams{Status: "active"})
		if err := s.Store.ResetPassword(ctx, store.IssueSetupLinkParams{UserID: created.ID, TokenHash: []byte("secret-link-2"), ExpiresAt: at.Add(time.Hour)}, by); err != nil {
			t.Fatal(err)
		}
		own := store.ActivityBy{At: at, IP: "198.51.100.9", UserAgent: "Safari"}
		if used, err := s.Store.SetPassword(ctx, store.SetPasswordParams{PasswordHash: "secret-hash", TokenHash: []byte("secret-link-2"), At: at}, own); err != nil || used != 1 {
			t.Fatalf("set password = %d, %v", used, err)
		}
		change(store.ChangeUserParams{Status: "inactive"})
		if err := s.Store.DeleteUser(ctx, store.DeleteParams{ID: created.ID, UserID: boss.ID, At: at}, by); err != nil {
			t.Fatal(err)
		}

		rows := chronological(t, s, "user")

		want := []string{"created", "setup-link-issued", "name-changed", "role-changed", "made-inactive", "made-active", "password-reset", "setup-link-issued", "password-set", "made-inactive", "deleted"}
		if got := actionsOf(rows); !slices.Equal(got, want) {
			t.Fatalf("actions = %v, want %v", got, want)
		}
		if c := changesOf(t, rows[0]); len(c) != 1 || c[0].Field != "role" || c[0].Before != nil || c[0].After != "staff" {
			t.Errorf("created changes = %+v, want the Role", c)
		}
		if c := changesOf(t, rows[2]); len(c) != 1 || c[0].Field != "name" || c[0].Before != "Somchai" || c[0].After != "Somchai P." {
			t.Errorf("name-changed changes = %+v", c)
		}
		if c := changesOf(t, rows[3]); len(c) != 1 || c[0].Field != "role" || c[0].Before != "staff" || c[0].After != "admin" {
			t.Errorf("role-changed changes = %+v", c)
		}
		for _, r := range rows {
			if r.SubjectKind != "user" || r.SubjectID != created.ID || r.SubjectRef != "somchai@example.com" {
				t.Errorf("subject of %s = %+v", r.Action, r)
			}
			// activity-log R6: never a password, code, link or token.
			if strings.Contains(string(r.Changes), "secret") || strings.Contains(r.ActorName+r.ActorEmail+r.SubjectName+r.SubjectRef, "secret") {
				t.Errorf("%s holds a secret: %+v", r.Action, r)
			}
		}
		// The Owner acted, except for the password the User set for themselves.
		if rows[2].ActorID.Int64 != boss.ID || rows[2].ActorName != "Somying" {
			t.Errorf("actor of name-changed = %+v", rows[2])
		}
		set := rows[8]
		if set.ActorKind != "user" || set.ActorID.Int64 != created.ID || set.ActorName != "Somchai P." || set.ActorEmail != "somchai@example.com" ||
			set.ActorRole != "admin" || set.Ip != "198.51.100.9" || set.UserAgent != "Safari" || set.SubjectName != "Somchai P." {
			t.Errorf("password-set = %+v, want the User as actor, as they were then", set)
		}
	})

	t.Run("writes an entry for each Order change, naming a system cancellation", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somchai@example.com"))
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 10))
		by := userBy(user.ID, at)
		first, err := s.Store.CreateOrder(ctx, newOrder(user, "Somying", line(tea, 1)), by)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Store.CompleteOrder(ctx, finish(first.Number, user), by); err != nil {
			t.Fatal(err)
		}
		second, err := s.Store.CreateOrder(ctx, newOrder(user, "", line(tea, 2)), by)
		if err != nil {
			t.Fatal(err)
		}
		system := store.ActivityBy{At: at, Actor: store.ActivityActor{Kind: store.ActorSystem, Name: "System"}}
		if _, err := s.Store.CancelOrder(ctx, store.FinishOrderParams{Number: second.Number, Now: clock(at)}, system); err != nil {
			t.Fatal(err)
		}
		if err := s.Store.DeleteOrder(ctx, store.DeleteOrderParams{Number: first.Number, UserID: user.ID, At: at}, by); err != nil {
			t.Fatal(err)
		}

		rows := chronological(t, s, "order")

		want := []string{"created", "completed", "created", "cancelled", "deleted"}
		if got := actionsOf(rows); !slices.Equal(got, want) {
			t.Fatalf("actions = %v, want %v", got, want)
		}
		if r := rows[0]; r.SubjectName != "Somying" || r.SubjectRef != "1" || r.SubjectID != first.ID || r.ActorID.Int64 != user.ID {
			t.Errorf("created = %+v", r)
		}
		if r := rows[3]; r.ActorKind != "system" || r.ActorID.Valid || r.SubjectRef != "2" {
			t.Errorf("cancelled = %+v, want the system as actor", r)
		}
		// activity-log R11: a Deleted Order is still named, by its number and customer.
		if r := rows[4]; r.SubjectRef != "1" || r.SubjectName != "Somying" {
			t.Errorf("deleted = %+v", r)
		}
	})

	t.Run("saves no User or Order change when its entry fails", func(t *testing.T) {
		s := newSubject(t)
		boss := s.AddUser(t, ownerUser("malee@example.com"))
		user := s.AddUser(t, staff("somchai@example.com"))
		gone := s.AddUser(t, store.User{Name: "Gone", Email: "gone@example.com", Role: "staff", Status: "inactive"})
		tea := s.AddProduct(t, priced("Thai tea", "TEA-01", 4500, 10))
		good := userBy(boss.ID, at)
		order, err := s.Store.CreateOrder(ctx, newOrder(user, "", line(tea, 1)), good)
		if err != nil {
			t.Fatal(err)
		}
		done, err := s.Store.CreateOrder(ctx, newOrder(user, "", line(tea, 1)), good)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Store.CompleteOrder(ctx, finish(done.Number, user), good); err != nil {
			t.Fatal(err)
		}
		before := len(listActivity(t, s, store.ListActivityLogParams{}))
		bad := good
		bad.Actor = store.ActivityActor{Kind: "robot"} // the table refuses it (activity-log R3)

		failures := map[string]error{}
		_, failures["create user"] = s.Store.CreateUser(ctx, store.CreateUserParams{Name: "New", Email: "new@example.com", Role: "staff", TokenHash: []byte("l"), ExpiresAt: at.Add(time.Hour)}, bad)
		_, failures["create email user"] = s.Store.CreateEmailUser(ctx, store.CreateEmailUserParams{Name: "Mail", Email: "mail@example.com", Role: "staff"}, bad)
		failures["delete user"] = s.Store.DeleteUser(ctx, store.DeleteParams{ID: gone.ID, UserID: boss.ID, At: at}, bad)
		_, failures["update user"] = s.Store.UpdateUser(ctx, store.ChangeUserParams{ID: user.ID, Name: "Changed"}, bad)
		failures["reset password"] = s.Store.ResetPassword(ctx, store.IssueSetupLinkParams{UserID: user.ID, TokenHash: []byte("l2"), ExpiresAt: at.Add(time.Hour)}, bad)
		_, failures["complete order"] = s.Store.CompleteOrder(ctx, finish(order.Number, user), bad)
		_, failures["cancel order"] = s.Store.CancelOrder(ctx, finish(order.Number, user), bad)
		failures["delete order"] = s.Store.DeleteOrder(ctx, store.DeleteOrderParams{Number: done.Number, UserID: user.ID, At: at}, bad)
		_, failures["create order"] = s.Store.CreateOrder(ctx, newOrder(user, "", line(tea, 1)), bad)
		for name, err := range failures {
			if err == nil {
				t.Errorf("%s: err = nil, want the entry's refusal", name)
			}
		}

		if rows := listActivity(t, s, store.ListActivityLogParams{}); len(rows) != before {
			t.Errorf("entries = %d, want %d: nothing was saved", len(rows), before)
		}
		if got, err := s.Store.GetUserByEmail(ctx, "somchai@example.com"); err != nil || got.Name != "Somchai" {
			t.Errorf("user = %+v, %v, want it unchanged", got, err)
		}
		for _, email := range []string{"new@example.com", "mail@example.com"} {
			if _, err := s.Store.GetUserByEmail(ctx, email); err == nil {
				t.Errorf("%s was saved although its entry failed", email)
			}
		}
		if got, err := s.Store.GetUserByEmail(ctx, "gone@example.com"); err != nil || got.ID != gone.ID {
			t.Errorf("the inactive User = %+v, %v, want them not Deleted", got, err)
		}
		if got, err := s.Store.GetOrder(ctx, order.Number); err != nil || got.Status != store.OrderPending {
			t.Errorf("order = %+v, %v, want it Pending", got.ReadOrderRow, err)
		}
		if got, err := s.Store.GetOrder(ctx, done.Number); err != nil || got.Status != store.OrderCompleted {
			t.Errorf("deleted order = %+v, %v, want it not Deleted", got.ReadOrderRow, err)
		}
		if p, err := s.Store.GetProduct(ctx, tea.ID); err != nil || p.Stock != 8 {
			t.Errorf("stock = %d, %v, want 8: the failed cancel and create changed none", p.Stock, err)
		}
	})

	t.Run("records the installation as the actor of the Owner and their Setup link", func(t *testing.T) {
		s := newSubject(t)

		if _, err := auth.IssueOwnerSetupLink(ctx, s.Store, at, "Malee", "malee@example.com"); err != nil {
			t.Fatal(err)
		}
		if _, err := auth.IssueOwnerSetupLink(ctx, s.Store, at.Add(time.Hour), "", ""); err != nil {
			t.Fatal(err)
		}

		rows := chronological(t, s, "user")
		if got, want := actionsOf(rows), []string{"created", "setup-link-issued", "setup-link-issued"}; !slices.Equal(got, want) {
			t.Fatalf("actions = %v, want %v: the second install makes no Owner", got, want)
		}
		for _, r := range rows {
			if r.ActorKind != "installation" || r.ActorID.Valid || r.SubjectRef != "malee@example.com" || r.SubjectName != "Malee" {
				t.Errorf("%s = %+v, want the installation as actor", r.Action, r)
			}
		}
		if c := changesOf(t, rows[0]); len(c) != 1 || c[0].After != "owner" {
			t.Errorf("created changes = %+v, want the Role owner", c)
		}
	})
}

func ownerUser(email string) store.User {
	return store.User{Name: "Malee", Email: email, Role: "owner", Status: "active"}
}
