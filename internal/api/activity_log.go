package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// activityBy is who is making this request's change, as they are now, for the Activity log
// entry its transaction writes (activity-log R2, R3). Only operations behind a session
// call it.
func activityBy(ctx context.Context, deps Deps) store.ActivityBy {
	session := sessionOf(ctx)
	info, _ := ctx.Value(requestInfoKey{}).(requestInfo)
	return store.ActivityBy{
		At:        deps.Now(),
		Actor:     store.ActivityActor{Kind: store.ActorUser, ID: session.UserID, Name: session.Name, Email: session.Email, Role: session.Role},
		IP:        info.ip,
		UserAgent: info.userAgent,
	}
}

type activityActor struct {
	Kind  string `json:"kind" enum:"user,customer,system,installation" doc:"A User, a Customer, the system, or the installation."`
	ID    *int64 `json:"id" doc:"The id of the User or Customer; null for the system and the installation."`
	Name  string `json:"name" doc:"As it was when the change was made."`
	Email string `json:"email" doc:"As it was when the change was made. Empty for the system and the installation."`
	Role  string `json:"role" doc:"A User's Role as it was; empty for any other actor."`
}

type activitySubject struct {
	Kind string `json:"kind" enum:"product,order,user,customer" doc:"What was changed."`
	ID   int64  `json:"id" doc:"Its id."`
	Name string `json:"name" doc:"Its name as it was when the change was made, so the entry stays readable once it is Deleted."`
	Ref  string `json:"ref" doc:"Its SKU, Order number or email as it was."`
}

type activityChange struct {
	Field  string `json:"field" doc:"The field that changed."`
	Before any    `json:"before" doc:"Its value before."`
	After  any    `json:"after" doc:"Its value after."`
}

type activityEntryBody struct {
	ID        int64            `json:"id" doc:"Identifies the entry. Later entries have higher ids."`
	At        time.Time        `json:"at" doc:"When the change was made."`
	Actor     activityActor    `json:"actor" doc:"Who made it."`
	Action    string           `json:"action" doc:"What was done: added, edited, archived, restored or deleted for a Product."`
	Subject   activitySubject  `json:"subject" doc:"What it was done to."`
	Changes   []activityChange `json:"changes" nullable:"false" doc:"For an edit, each changed field with its value before and after; empty otherwise."`
	IP        string           `json:"ip" doc:"The IP address of the request. Empty when no person made the change."`
	UserAgent string           `json:"userAgent" doc:"The browser or device. Empty when none."`
}

type listActivityLogInput struct {
	ActorKind string `query:"actorKind" enum:"user,customer,system,installation" doc:"Only the changes made by this kind of actor."`
	ActorID   int64  `query:"actorId" minimum:"1" doc:"Only the changes made by the User or Customer with this id; use with actorKind."`
	Kind      string `query:"kind" enum:"product,order,user,customer" doc:"Only the changes to this kind of record."`
	Action    string `query:"action" maxLength:"40" pattern:"^[a-z-]+$" doc:"Only this action, such as edited."`
	From      string `query:"from" format:"date" doc:"Only the changes on or after this day of the Shop, in Bangkok time (YYYY-MM-DD)."`
	To        string `query:"to" format:"date" doc:"Only the changes on or before this day of the Shop, in Bangkok time (YYYY-MM-DD)."`
	Offset    int32  `query:"offset" minimum:"0" maximum:"2147483647" default:"0" doc:"How many matching entries to skip: page number times limit."`
	Limit     int32  `query:"limit" enum:"10,25,50" default:"25" doc:"The most entries on a page."`
}

// Resolve refuses a range that ends before it starts, which no schema can say.
func (in *listActivityLogInput) Resolve(huma.Context) []error {
	if in.From != "" && in.To != "" && in.To < in.From {
		return []error{&huma.ErrorDetail{Location: "query.to", Message: "must not be before from", Value: in.To}}
	}
	return nil
}

type listActivityLogOutput struct {
	Body struct {
		Entries []activityEntryBody `json:"entries" nullable:"false" doc:"Newest first."`
		Total   int64               `json:"total" minimum:"0" doc:"Exact count of the entries matching every filter, whatever the page."`
	}
}

func registerActivityLog(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-activity-log",
		Method:      http.MethodGet,
		Path:        "/v2/activity-log",
		Summary:     "List the Activity log",
		Description: "Every recorded change, newest first, in numbered pages of 10, 25 or 50 with an exact total. " +
			"An out-of-range page is empty with the actual total. Entries cannot be changed or removed. " + ownerOnlyErrors,
		Tags:        []string{"Activity log"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{ownerOnly(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, func(ctx context.Context, input *listActivityLogInput) (*listActivityLogOutput, error) {
		filter := store.CountActivityLogParams{
			ActorKind:   pgtype.Text{String: input.ActorKind, Valid: input.ActorKind != ""},
			ActorID:     pgtype.Int8{Int64: input.ActorID, Valid: input.ActorID != 0},
			SubjectKind: pgtype.Text{String: input.Kind, Valid: input.Kind != ""},
			Action:      pgtype.Text{String: input.Action, Valid: input.Action != ""},
			FromAt:      startOfShopDay(input.From, 0),
			ToAt:        startOfShopDay(input.To, 1),
		}
		rows, err := deps.Store.ListActivityLog(ctx, store.ListActivityLogParams{
			ActorKind: filter.ActorKind, ActorID: filter.ActorID, SubjectKind: filter.SubjectKind, Action: filter.Action,
			FromAt: filter.FromAt, ToAt: filter.ToAt, RowOffset: input.Offset, RowLimit: input.Limit,
		})
		if err != nil {
			return nil, internalError(ctx, "list activity log", err)
		}
		total, err := deps.Store.CountActivityLog(ctx, filter)
		if err != nil {
			return nil, internalError(ctx, "count activity log", err)
		}
		out := &listActivityLogOutput{}
		out.Body.Total = total
		out.Body.Entries = make([]activityEntryBody, 0, len(rows))
		for _, row := range rows {
			entry, err := activityEntryOf(row)
			if err != nil {
				return nil, internalError(ctx, "read activity entry", err)
			}
			out.Body.Entries = append(out.Body.Entries, entry)
		}
		return out, nil
	})
}

func activityEntryOf(row store.ActivityLog) (activityEntryBody, error) {
	changes := []activityChange{}
	if err := json.Unmarshal(row.Changes, &changes); err != nil {
		return activityEntryBody{}, fmt.Errorf("entry %d changes: %w", row.ID, err)
	}
	actor := activityActor{Kind: row.ActorKind, Name: row.ActorName, Email: row.ActorEmail, Role: row.ActorRole}
	if row.ActorID.Valid {
		actor.ID = &row.ActorID.Int64
	}
	return activityEntryBody{
		ID: row.ID, At: row.At, Actor: actor, Action: row.Action,
		Subject: activitySubject{Kind: row.SubjectKind, ID: row.SubjectID, Name: row.SubjectName, Ref: row.SubjectRef},
		Changes: changes, IP: row.Ip, UserAgent: row.UserAgent,
	}, nil
}
