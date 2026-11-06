package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// A Setup link's token travels only in request bodies, which is why reading one is a
// POST: paths and query strings end up in request logs.

type readSetupLinkInput struct {
	Body struct {
		Token string `json:"token" doc:"The token from the Setup link."`
	}
}

type readSetupLinkOutput struct {
	Body struct {
		Name  string `json:"name" doc:"The name of the person the Setup link is for."`
		Email string `json:"email" doc:"The email of the person the Setup link is for."`
	}
}

type setPasswordInput struct {
	Body struct {
		Token string `json:"token" doc:"The token from the Setup link."`
		// users-and-access R19. minLength counts characters, not bytes.
		Password string `json:"password" minLength:"6" doc:"The new password, of at least 6 characters."`
	}
}

func registerSetupLinks(api huma.API, deps Deps) {
	invalidLink := func() huma.StatusError {
		return domainError(http.StatusGone, codeSetupLinkInvalid, "This Setup link no longer works. Ask the Owner for a new one.")
	}

	huma.Register(api, huma.Operation{
		OperationID: "read-setup-link",
		Method:      http.MethodPost,
		Path:        "/setup-link/read",
		Summary:     "Read who a Setup link is for",
		Tags:        []string{"Setup links"},
		Errors:      []int{http.StatusGone},
	}, func(ctx context.Context, input *readSetupLinkInput) (*readSetupLinkOutput, error) {
		user, err := deps.Store.GetSetupLinkUser(ctx, store.GetSetupLinkUserParams{
			TokenHash: auth.HashToken(input.Body.Token),
			At:        deps.Now(),
		})
		if errors.Is(err, store.ErrNotFound) {
			return nil, invalidLink()
		}
		if err != nil {
			return nil, internalError(ctx, "read setup link", err)
		}
		out := &readSetupLinkOutput{}
		out.Body.Name, out.Body.Email = user.Name, user.Email
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "set-password",
		Method:        http.MethodPost,
		Path:          "/setup-link/password",
		Summary:       "Set a password through a Setup link",
		Description:   "Uses up the Setup link. It starts no session: the person logs in next (users-and-access R30).",
		Tags:          []string{"Setup links"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusGone},
	}, func(ctx context.Context, input *setPasswordInput) (*struct{}, error) {
		used, err := deps.Store.SetPassword(ctx, store.SetPasswordParams{
			PasswordHash: deps.Passwords.Hash(input.Body.Password),
			TokenHash:    auth.HashToken(input.Body.Token),
			At:           deps.Now(),
		})
		if err != nil {
			return nil, internalError(ctx, "set password", err)
		}
		if used == 0 {
			return nil, invalidLink()
		}
		return nil, nil
	})
}
