package storetest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func userBy(id int64, when time.Time) store.ActivityBy {
	return store.ActivityBy{
		At: when, IP: "203.0.113.7", UserAgent: "Firefox",
		Actor: store.ActivityActor{Kind: store.ActorUser, ID: id, Name: "Somying", Email: "somying@example.com", Role: "admin"},
	}
}

func listActivity(t *testing.T, s Subject, arg store.ListActivityLogParams) []store.ActivityLog {
	t.Helper()
	if arg.RowLimit == 0 {
		arg.RowLimit = 100
	}
	rows, err := s.Store.ListActivityLog(context.Background(), arg)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// runActivityLog is the part of the contract that is the Activity log (activity-log R1-R4,
// R8, R10, R11): each Product change writes its entries with the change, or neither is saved.
func runActivityLog(t *testing.T, newSubject func(t *testing.T) Subject) {
	ctx := context.Background()
	text := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

	t.Run("writes each Product change's entry with the change", func(t *testing.T) {
		s := newSubject(t)
		user := s.AddUser(t, staff("somying@example.com"))
		by := userBy(user.ID, at)
		tea, err := s.Store.CreateProduct(ctx, store.InsertProductParams{Name: "Thai tea", Sku: "TEA-01", Unit: "box", Price: 12000, MinimumStock: 2}, by)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: tea.ID, Name: text("Green tea"), Price: pgtype.Int8{Int64: 13000, Valid: true}}, by); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: tea.ID, Status: text("archived")}, by); err != nil {
			t.Fatal(err)
		}
		if err := s.Store.DeleteProduct(ctx, store.DeleteParams{ID: tea.ID, UserID: user.ID, At: at}, by); err != nil {
			t.Fatal(err)
		}

		rows := listActivity(t, s, store.ListActivityLogParams{})

		var actions []string
		for _, r := range rows {
			actions = append(actions, r.Action)
		}
		if len(rows) != 4 || actions[0] != "deleted" || actions[1] != "archived" || actions[2] != "edited" || actions[3] != "added" {
			t.Fatalf("actions = %v, want deleted, archived, edited, added", actions)
		}
		edited := rows[2]
		var changes []store.FieldChange
		if err := json.Unmarshal(edited.Changes, &changes); err != nil {
			t.Fatal(err)
		}
		if len(changes) != 2 || changes[0].Field != "name" || changes[0].Before != "Thai tea" || changes[0].After != "Green tea" ||
			changes[1].Field != "price" || changes[1].Before != float64(12000) || changes[1].After != float64(13000) {
			t.Errorf("changes = %+v, want name and price only", changes)
		}
		if edited.ActorKind != "user" || edited.ActorID.Int64 != user.ID || edited.ActorName != "Somying" || edited.ActorEmail != "somying@example.com" ||
			edited.ActorRole != "admin" || edited.SubjectKind != "product" || edited.SubjectID != tea.ID || edited.SubjectName != "Green tea" ||
			edited.SubjectRef != "TEA-01" || edited.Ip != "203.0.113.7" || edited.UserAgent != "Firefox" || !edited.At.Equal(at) {
			t.Errorf("edited = %+v", edited)
		}
		if rows[0].SubjectName != "Green tea" || rows[0].SubjectRef != "TEA-01" {
			t.Errorf("deleted = %+v, want the name and SKU kept", rows[0])
		}
		if string(rows[3].Changes) != "[]" {
			t.Errorf("added changes = %s, want []", rows[3].Changes)
		}
	})

	t.Run("saves no change, and writes no entry, when the entry or the change fails", func(t *testing.T) {
		s := newSubject(t)
		good := userBy(0, at)
		good.Actor = store.ActivityActor{Kind: store.ActorSystem}
		tea, err := s.Store.CreateProduct(ctx, store.InsertProductParams{Name: "Thai tea", Sku: "TEA-01", Unit: "box"}, good)
		if err != nil {
			t.Fatal(err)
		}
		bad := good
		bad.Actor = store.ActivityActor{Kind: "robot"} // the table refuses it (activity-log R3)

		if _, err := s.Store.CreateProduct(ctx, store.InsertProductParams{Name: "Green tea", Sku: "TEA-02", Unit: "box"}, bad); err == nil {
			t.Error("create: err = nil, want the entry's refusal")
		}
		if _, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: tea.ID, Name: text("Changed")}, bad); err == nil {
			t.Error("update: err = nil, want the entry's refusal")
		}
		if _, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: tea.ID, Status: text("archived")}, bad); err == nil {
			t.Error("archive: err = nil, want the entry's refusal")
		}
		got, err := s.Store.GetProduct(ctx, tea.ID)
		if err != nil || got.Name != "Thai tea" || got.Status != "active" {
			t.Errorf("product = %+v, %v, want it unchanged and Active", got, err)
		}
		if _, err := s.Store.CreateProduct(ctx, store.InsertProductParams{Name: "Other", Sku: "TEA-01", Unit: "box"}, good); err == nil {
			t.Error("a taken SKU: err = nil")
		}
		if _, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: tea.ID, Name: text("Thai tea")}, good); err != nil {
			t.Fatal(err) // nothing changed: no entry
		}
		if _, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: 999, Name: text("x")}, good); err == nil {
			t.Error("no such Product: err = nil")
		}
		if err := s.Store.DeleteProduct(ctx, store.DeleteParams{ID: tea.ID, At: at}, good); err == nil {
			t.Error("delete an Active Product: err = nil")
		}

		rows := listActivity(t, s, store.ListActivityLogParams{})
		if len(rows) != 1 || rows[0].Action != "added" {
			t.Errorf("entries = %+v, want only the first Product's added", rows)
		}
	})

	t.Run("filters by actor, subject, action and a half-open time range, and counts them exactly", func(t *testing.T) {
		s := newSubject(t)
		first, second := userBy(1, at), userBy(2, at.Add(time.Hour))
		for i, by := range []store.ActivityBy{first, second, second} {
			if _, err := s.Store.CreateProduct(ctx, store.InsertProductParams{Name: "P", Sku: string(rune('A' + i)), Unit: "box"}, by); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.Store.UpdateProduct(ctx, store.ChangeProductParams{ID: 1, Price: pgtype.Int8{Int64: 5, Valid: true}}, first); err != nil {
			t.Fatal(err)
		}
		count := func(arg store.CountActivityLogParams) int64 {
			t.Helper()
			n, err := s.Store.CountActivityLog(ctx, arg)
			if err != nil {
				t.Fatal(err)
			}
			return n
		}
		stamp := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
		for name, tc := range map[string]struct {
			arg  store.CountActivityLogParams
			want int64
		}{
			"all":           {store.CountActivityLogParams{}, 4},
			"actor":         {store.CountActivityLogParams{ActorKind: text("user"), ActorID: pgtype.Int8{Int64: 2, Valid: true}}, 2},
			"other kind":    {store.CountActivityLogParams{ActorKind: text("system")}, 0},
			"subject":       {store.CountActivityLogParams{SubjectKind: text("product")}, 4},
			"no subject":    {store.CountActivityLogParams{SubjectKind: text("order")}, 0},
			"action":        {store.CountActivityLogParams{Action: text("edited")}, 1},
			"from included": {store.CountActivityLogParams{FromAt: stamp(at.Add(time.Hour))}, 2},
			"to excluded":   {store.CountActivityLogParams{ToAt: stamp(at.Add(time.Hour))}, 2},
		} {
			if got := count(tc.arg); got != tc.want {
				t.Errorf("%s: count = %d, want %d", name, got, tc.want)
			}
		}
		page := listActivity(t, s, store.ListActivityLogParams{RowLimit: 2, RowOffset: 1})
		if len(page) != 2 || page[0].ID <= page[1].ID {
			t.Errorf("page = %+v, want 2 entries, newest first", page)
		}
	})
}
