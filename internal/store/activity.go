package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Who made a change (activity-log R2).
const (
	ActorUser         = "user"
	ActorCustomer     = "customer"
	ActorSystem       = "system"
	ActorInstallation = "installation"
)

// What a change was done to (activity-log R4).
const (
	SubjectProduct  = "product"
	SubjectOrder    = "order"
	SubjectUser     = "user"
	SubjectCustomer = "customer"
)

// What was done to a Product (activity-log R4).
const (
	ActionAdded    = "added"
	ActionEdited   = "edited"
	ActionArchived = "archived"
	ActionRestored = "restored"
	ActionDeleted  = "deleted"
)

// ActivityActor is who made a change, as they were then. ID is 0 for the system and the
// installation, and Role is empty unless the actor is a User.
type ActivityActor struct {
	Kind, Name, Email, Role string
	ID                      int64
}

// ActivityBy is who made a change, and when and from where. Every store mutation that
// activity-log R4 lists takes one and writes its entry in the same transaction (R3).
type ActivityBy struct {
	At        time.Time
	Actor     ActivityActor
	IP        string
	UserAgent string
}

// FieldChange is one changed field of an edit, with its value before and after.
type FieldChange struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// ActivityEntry is one entry of the Activity log, before it has an id.
type ActivityEntry struct {
	By                      ActivityBy
	Action, SubjectKind     string
	SubjectID               int64
	SubjectName, SubjectRef string
	Changes                 []FieldChange
}

// addActivity writes the entry on the transaction that made the change (activity-log R3).
func (q *Queries) addActivity(ctx context.Context, e ActivityEntry) error {
	changes, err := json.Marshal(append([]FieldChange{}, e.Changes...))
	if err != nil {
		return fmt.Errorf("encode changes: %w", err)
	}
	err = q.AddActivity(ctx, AddActivityParams{
		At: e.By.At, ActorKind: e.By.Actor.Kind, ActorID: pgtype.Int8{Int64: e.By.Actor.ID, Valid: e.By.Actor.ID != 0},
		ActorName: e.By.Actor.Name, ActorEmail: e.By.Actor.Email, ActorRole: e.By.Actor.Role,
		Action: e.Action, SubjectKind: e.SubjectKind, SubjectID: e.SubjectID, SubjectName: e.SubjectName, SubjectRef: e.SubjectRef,
		Changes: string(changes), Ip: e.By.IP, UserAgent: e.By.UserAgent,
	})
	if err != nil {
		return fmt.Errorf("add activity: %w", err)
	}
	return nil
}

func productEntry(p Product, action string, by ActivityBy, changes []FieldChange) ActivityEntry {
	return ActivityEntry{By: by, Action: action, SubjectKind: SubjectProduct, SubjectID: p.ID, SubjectName: p.Name, SubjectRef: p.Sku, Changes: changes}
}

// ProductAdded is the entry of a new Product (activity-log R4).
func ProductAdded(p Product, by ActivityBy) ActivityEntry {
	return productEntry(p, ActionAdded, by, nil)
}

// ProductDeleted is the entry of a Deleted Product, which keeps its name and SKU (R11).
func ProductDeleted(p Product, by ActivityBy) ActivityEntry {
	return productEntry(p, ActionDeleted, by, nil)
}

// ProductEdited is the entries of a change to a Product (activity-log R4): an edit with
// each changed field before and after, and Archived or restored when the status changed.
// A change that changed nothing has none. Stock is no field here: Stock movements are its
// record (R5). The entries name the Product as it is after the change.
func ProductEdited(before, after Product, by ActivityBy) []ActivityEntry {
	var changes []FieldChange
	add := func(field string, was, is any) {
		if was != is {
			changes = append(changes, FieldChange{Field: field, Before: was, After: is})
		}
	}
	add("name", before.Name, after.Name)
	add("sku", before.Sku, after.Sku)
	add("unit", before.Unit, after.Unit)
	add("price", before.Price, after.Price)
	add("minimumStock", before.MinimumStock, after.MinimumStock)
	var entries []ActivityEntry
	if len(changes) > 0 {
		entries = append(entries, productEntry(after, ActionEdited, by, changes))
	}
	if before.Status != after.Status {
		action := ActionRestored
		if after.Status == "archived" {
			action = ActionArchived
		}
		entries = append(entries, productEntry(after, action, by, nil))
	}
	return entries
}
