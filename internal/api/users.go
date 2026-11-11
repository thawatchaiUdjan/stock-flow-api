package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

type user struct {
	ID          int64  `json:"id" doc:"Identifies the User. It never changes."`
	Name        string `json:"name" doc:"The User's name, in any language."`
	Email       string `json:"email" doc:"As it was entered when the User was created. It never changes."`
	Role        string `json:"role" enum:"owner,admin,staff" doc:"The User's Role."`
	Status      string `json:"status" enum:"active,inactive" doc:"An Inactive User cannot log in."`
	PasswordSet bool   `json:"passwordSet" doc:"False until the person sets their own password."`
}

// setupLink is what an Owner is shown once, to pass on by hand (users-and-access R28).
type setupLink struct {
	Token     string    `json:"token" doc:"The Setup link's token. It is shown only this once: the API keeps a hash."`
	ExpiresAt time.Time `json:"expiresAt" doc:"When the Setup link stops working."`
}

type listUsersOutput struct {
	Body struct {
		Users []user `json:"users" nullable:"false" doc:"Every User, in the order they were created."`
	}
}

type createUserBody struct {
	// pattern: a name of only whitespace, in any script, is no name (users-and-access R14).
	// It is every character unicode.IsSpace holds, as the database constraint has it
	// (migration 00009). They are written out, not as \p{Z} or \x{85}, which JavaScript
	// reads differently or not at all. The name is stored as it was given, in any language.
	Name string `json:"name" pattern:"[^\\t-\\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]" patternDescription:"contain a character that is not whitespace" doc:"The User's name, in any language."`
	// users-and-access R15: the email is fixed from here on.
	Email string `json:"email" format:"email" doc:"Compared without regard to letter case; it cannot change afterwards."`
	// users-and-access R9, R10: the Owner is made at installation, so only these two.
	Role string `json:"role" enum:"admin,staff" doc:"The Role of the new User. There is one Owner, and it is not made here."`
}

type createUserInput struct {
	Body createUserBody
}

type createUserOutput struct {
	Body struct {
		User      user      `json:"user"`
		SetupLink setupLink `json:"setupLink" doc:"How the new User sets their password (users-and-access R20, R28)."`
	}
}

// targetErrors is how every operation on one User describes its refusals of that User.
const targetErrors = "Answers 403 `target-is-owner` when the target is the Owner, and a plain 404 when there is no such User."

type updateUserBody struct {
	// A field left out stays as it is. There is no email: it cannot change
	// (users-and-access R15), and a body that names one is refused. pattern: a name of
	// only whitespace is no name (R14), written out as in createUserBody.
	Name string `json:"name,omitempty" pattern:"[^\\t-\\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]" patternDescription:"contain a character that is not whitespace" doc:"The User's name, in any language."`
	// users-and-access R9, R35: between these two, never to Owner.
	Role string `json:"role,omitempty" enum:"admin,staff" doc:"Applies at once to the sessions the User has open."`
	// users-and-access R16.
	Status string `json:"status,omitempty" enum:"active,inactive" doc:"Setting a User Inactive ends all their sessions and invalidates setup/recovery credentials. Setting them Active again revives none."`
}

type updateUserInput struct {
	ID   int64 `path:"id" doc:"The User's id."`
	Body updateUserBody
}

type updateUserOutput struct {
	Body user
}

type resetPasswordInput struct {
	ID int64 `path:"id" doc:"The User's id."`
}

type resetPasswordOutput struct {
	Body setupLink
}

func registerUsers(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-users",
		Method:      http.MethodGet,
		Path:        "/users",
		Summary:     "List the Users",
		Description: ownerOnlyErrors,
		Tags:        []string{"Users"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{ownerOnly(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
	}, func(ctx context.Context, _ *struct{}) (*listUsersOutput, error) {
		rows, err := deps.Store.ListUsers(ctx)
		if err != nil {
			return nil, internalError(ctx, "list users", err)
		}
		out := &listUsersOutput{}
		out.Body.Users = make([]user, 0, len(rows))
		for _, row := range rows {
			out.Body.Users = append(out.Body.Users, user(row))
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-user",
		Method:        http.MethodPost,
		Path:          "/users",
		Summary:       "Create a User",
		Description:   "Issues the new User's Setup link and answers its token, which is not shown again. Answers 409 `email-taken` when a User has the email already, in any letter case. " + ownerOnlyErrors,
		Tags:          []string{"Users"},
		Security:      sessionSecurity,
		Middlewares:   huma.Middlewares{ownerOnly(api, deps)},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict},
	}, func(ctx context.Context, input *createUserInput) (*createUserOutput, error) {
		link := auth.NewSetupLink(deps.Now())
		created, err := deps.Store.CreateUser(ctx, store.CreateUserParams{
			Name:      input.Body.Name,
			Email:     input.Body.Email,
			Role:      input.Body.Role,
			TokenHash: auth.HashToken(link.Token),
			ExpiresAt: link.ExpiresAt,
		})
		// No row back means the email is in use, in any letter case (users-and-access R3).
		if errors.Is(err, store.ErrNotFound) {
			return nil, domainError(http.StatusConflict, codeEmailTaken, "A User with this email already exists.")
		}
		if err != nil {
			return nil, internalError(ctx, "create user", err)
		}
		out := &createUserOutput{}
		out.Body.User = user{
			ID: created.ID, Name: created.Name, Email: created.Email, Role: created.Role,
			Status: created.Status, PasswordSet: false,
		}
		out.Body.SetupLink = setupLink{Token: link.Token, ExpiresAt: link.ExpiresAt}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-user",
		Method:      http.MethodPatch,
		Path:        "/users/{id}",
		Summary:     "Change a User's name, Role or status",
		Description: "Changes the fields the body names and leaves the rest. " + targetErrors + " " + ownerOnlyErrors,
		Tags:        []string{"Users"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{ownerOnly(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound},
	}, func(ctx context.Context, input *updateUserInput) (*updateUserOutput, error) {
		updated, err := deps.Store.UpdateUser(ctx, store.ChangeUserParams{
			ID: input.ID, Name: input.Body.Name, Role: input.Body.Role, Status: input.Body.Status,
		})
		if err != nil {
			return nil, targetError(ctx, "update user", err)
		}
		return &updateUserOutput{Body: user(updated)}, nil
	})

	// One operation for users-and-access R29 and R31: with no password or session to end,
	// all it does is issue the new link.
	huma.Register(api, huma.Operation{
		OperationID: "reset-password",
		Method:      http.MethodPost,
		Path:        "/users/{id}/reset-password",
		Summary:     "Reset a User's password",
		Description: "Ends the User's password and sessions, cancels any earlier Setup link, and issues a new one, " +
			"whose token is not shown again. Answers 409 `user-inactive` when the User is Inactive. " +
			targetErrors + " " + ownerOnlyErrors,
		Tags:        []string{"Users"},
		Security:    sessionSecurity,
		Middlewares: huma.Middlewares{ownerOnly(api, deps)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict},
	}, func(ctx context.Context, input *resetPasswordInput) (*resetPasswordOutput, error) {
		link := auth.NewSetupLink(deps.Now())
		err := deps.Store.ResetPassword(ctx, store.IssueSetupLinkParams{
			UserID: input.ID, TokenHash: auth.HashToken(link.Token), ExpiresAt: link.ExpiresAt,
		})
		if err != nil {
			return nil, targetError(ctx, "reset password", err)
		}
		return &resetPasswordOutput{Body: setupLink{Token: link.Token, ExpiresAt: link.ExpiresAt}}, nil
	})
}

// targetError is the answer of an operation on a User, when it fails.
func targetError(ctx context.Context, doing string, err error) huma.StatusError {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return huma.Error404NotFound("There is no such User.")
	case errors.Is(err, store.ErrTargetIsOwner): // users-and-access R18, R34
		return domainError(http.StatusForbidden, codeTargetIsOwner, "The Owner's own User cannot be changed.")
	case errors.Is(err, store.ErrUserInactive): // users-and-access R32
		return domainError(http.StatusConflict, codeUserInactive, "This User is Inactive. Set them Active first.")
	}
	return internalError(ctx, doing, err)
}
