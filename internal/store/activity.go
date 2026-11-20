package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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

// What was done to a User or an Order (activity-log R4).
const (
	ActionCreated         = "created"
	ActionNameChanged     = "name-changed"
	ActionRoleChanged     = "role-changed"
	ActionMadeInactive    = "made-inactive"
	ActionMadeActive      = "made-active"
	ActionPasswordSet     = "password-set"
	ActionPasswordReset   = "password-reset"
	ActionSetupLinkIssued = "setup-link-issued"
	ActionCompleted       = "completed"
	ActionCancelled       = "cancelled"
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

// RecordActivity writes an entry on this transaction, for a change that is made outside the
// store, as the installation command makes the Owner (activity-log R3).
func (q *Queries) RecordActivity(ctx context.Context, e ActivityEntry) error {
	return q.addActivity(ctx, e)
}

// AsUser is by with the actor a User who acts on themselves, as when a password is set
// through a Setup link or a Reset link: the person is only known once the link is read.
func (by ActivityBy) AsUser(u UserSnapshot) ActivityBy {
	by.Actor = ActivityActor{Kind: ActorUser, ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role}
	return by
}

// UserSnapshot is a User as an entry records them (activity-log R2, R11): their email and
// name, as they were, and what a change may have altered.
type UserSnapshot struct {
	ID                        int64
	Name, Email, Role, Status string
}

func userEntry(u UserSnapshot, action string, by ActivityBy, changes []FieldChange) ActivityEntry {
	return ActivityEntry{By: by, Action: action, SubjectKind: SubjectUser, SubjectID: u.ID, SubjectName: u.Name, SubjectRef: u.Email, Changes: changes}
}

// UserCreated is the entry of a new User, with their Role (activity-log R4).
func UserCreated(u UserSnapshot, by ActivityBy) ActivityEntry {
	return userEntry(u, ActionCreated, by, []FieldChange{{Field: "role", Before: nil, After: u.Role}})
}

// UserDeleted, UserPasswordSet, UserPasswordReset and UserSetupLinkIssued carry no field: a
// password, code, link or token is never in an entry (activity-log R6).
func UserDeleted(u UserSnapshot, by ActivityBy) ActivityEntry {
	return userEntry(u, ActionDeleted, by, nil)
}

func UserPasswordSet(u UserSnapshot, by ActivityBy) ActivityEntry {
	return userEntry(u, ActionPasswordSet, by, nil)
}

func UserPasswordReset(u UserSnapshot, by ActivityBy) ActivityEntry {
	return userEntry(u, ActionPasswordReset, by, nil)
}

func UserSetupLinkIssued(u UserSnapshot, by ActivityBy) ActivityEntry {
	return userEntry(u, ActionSetupLinkIssued, by, nil)
}

// UserEdited is the entries of a change to a User (activity-log R4): their name changed,
// their Role changed, and made Inactive or Active, each with its before and after where it
// has them. A change that changed nothing has none. They name the User as they are after.
func UserEdited(before, after UserSnapshot, by ActivityBy) []ActivityEntry {
	var entries []ActivityEntry
	if before.Name != after.Name {
		entries = append(entries, userEntry(after, ActionNameChanged, by, []FieldChange{{Field: "name", Before: before.Name, After: after.Name}}))
	}
	if before.Role != after.Role {
		entries = append(entries, userEntry(after, ActionRoleChanged, by, []FieldChange{{Field: "role", Before: before.Role, After: after.Role}}))
	}
	if before.Status != after.Status {
		action := ActionMadeActive
		if after.Status == "inactive" {
			action = ActionMadeInactive
		}
		entries = append(entries, userEntry(after, action, by, nil))
	}
	return entries
}

// OrderEntry is the entry of a change to an Order: created, completed, cancelled (by whom
// is the entry's actor, a User or the system) or Deleted. It names the Order by its
// customer and its number as they were (activity-log R2, R11).
func OrderEntry(d ReadOrderRow, action string, by ActivityBy) ActivityEntry {
	return ActivityEntry{
		By: by, Action: action, SubjectKind: SubjectOrder, SubjectID: d.ID, SubjectName: d.Customer,
		SubjectRef: strconv.FormatInt(d.Number, 10),
	}
}
