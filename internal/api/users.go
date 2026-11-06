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
	PasswordSet bool   `json:"passwordSet" doc:"False until the person sets a password through a Setup link."`
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
	// pattern \S: a name of only spaces is no name (users-and-access R14). The name is
	// stored as it was given, in any language.
	Name string `json:"name" pattern:"\\S" patternDescription:"contain a character that is not a space" doc:"The User's name, in any language."`
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
		link := auth.NewSetupLink(deps.Now(), input.Body.Email)
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
}
