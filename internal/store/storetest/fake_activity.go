package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// checkActivity is the activity_log table's CHECK constraints: the fake refuses the entry,
// and so the change it belongs to, as the database would (activity-log R3).
func (f *Fake) checkActivity(e store.ActivityEntry) error {
	if f.ActivityErr != nil {
		return f.ActivityErr
	}
	kinds := []string{store.ActorUser, store.ActorCustomer, store.ActorSystem, store.ActorInstallation}
	hasID := e.By.Actor.Kind == store.ActorUser || e.By.Actor.Kind == store.ActorCustomer
	if !slices.Contains(kinds, e.By.Actor.Kind) || hasID != (e.By.Actor.ID != 0) {
		return errors.New("violates activity_log actor check")
	}
	return nil
}

// record keeps the entries of a change that has passed checkActivity.
func (f *Fake) record(entries ...store.ActivityEntry) {
	for _, e := range entries {
		changes, _ := json.Marshal(append([]store.FieldChange{}, e.Changes...))
		f.activity = append(f.activity, store.ActivityLog{
			ID: int64(len(f.activity) + 1), At: e.By.At, ActorKind: e.By.Actor.Kind,
			ActorID:   pgtype.Int8{Int64: e.By.Actor.ID, Valid: e.By.Actor.ID != 0},
			ActorName: e.By.Actor.Name, ActorEmail: e.By.Actor.Email, ActorRole: e.By.Actor.Role,
			Action: e.Action, SubjectKind: e.SubjectKind, SubjectID: e.SubjectID, SubjectName: e.SubjectName, SubjectRef: e.SubjectRef,
			Changes: changes, Ip: e.By.IP, UserAgent: e.By.UserAgent,
		})
	}
}

func matchesActivity(row store.ActivityLog, arg store.CountActivityLogParams) bool {
	return (!arg.ActorKind.Valid || row.ActorKind == arg.ActorKind.String) &&
		(!arg.ActorID.Valid || row.ActorID == arg.ActorID) &&
		(!arg.SubjectKind.Valid || row.SubjectKind == arg.SubjectKind.String) &&
		(!arg.Action.Valid || row.Action == arg.Action.String) &&
		(!arg.FromAt.Valid || !row.At.Before(arg.FromAt.Time)) &&
		(!arg.ToAt.Valid || row.At.Before(arg.ToAt.Time))
}

func (f *Fake) CountActivityLog(_ context.Context, arg store.CountActivityLogParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return 0, f.Err
	}
	var n int64
	for _, row := range f.activity {
		if matchesActivity(row, arg) {
			n++
		}
	}
	return n, nil
}

func (f *Fake) ListActivityLog(_ context.Context, arg store.ListActivityLogParams) ([]store.ActivityLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	filter := store.CountActivityLogParams{ActorKind: arg.ActorKind, ActorID: arg.ActorID, SubjectKind: arg.SubjectKind, Action: arg.Action, FromAt: arg.FromAt, ToAt: arg.ToAt}
	var rows []store.ActivityLog
	for _, row := range slices.Backward(f.activity) {
		if matchesActivity(row, filter) {
			rows = append(rows, row)
		}
	}
	rows = rows[min(len(rows), int(arg.RowOffset)):]
	return rows[:min(len(rows), int(arg.RowLimit))], nil
}
