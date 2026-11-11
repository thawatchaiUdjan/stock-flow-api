package email_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/email"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// users-and-access R49: assert actual provider content, not a separate mock template.
func TestBrevoSendsStockFlowCodeAsEscapedHTMLAndPlainText(t *testing.T) {
	var payload struct {
		Sender                            struct{ Name, Email string }
		To                                []struct{ Name, Email string }
		Subject, HTMLContent, TextContent string
	}
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != "https://api.brevo.com/v3/smtp/email" || r.Header.Get("api-key") != "test-key" {
			t.Fatalf("provider request = %s %s", r.Method, r.URL)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"messageId":"accepted"}`)), Header: http.Header{}}, nil
	})}
	sender, err := email.NewBrevo(email.Config{APIKey: "test-key", SenderEmail: "sender@example.com"}, client)
	if err != nil {
		t.Fatal(err)
	}
	message := email.Message{Kind: email.FirstTimeCode, To: "new@example.com", Name: "<script>New</script>", Code: "012345", Link: "https://stock-flow.example/password-setup", ExpiresAt: time.Date(2026, 11, 11, 9, 10, 0, 0, time.UTC)}
	if err := sender.Send(t.Context(), message); err != nil {
		t.Fatal(err)
	}
	if payload.Sender.Name != "StockFlow" || payload.Sender.Email != "sender@example.com" || len(payload.To) != 1 || payload.To[0].Email != message.To {
		t.Fatalf("sender/recipient = %+v / %+v", payload.Sender, payload.To)
	}
	if payload.Subject != "Your StockFlow setup code" || !strings.Contains(payload.HTMLContent, "012345") || !strings.Contains(payload.TextContent, "012345") || !strings.Contains(payload.TextContent, "10 minutes") {
		t.Fatalf("missing code/lifetime: %+v", payload)
	}
	if strings.Contains(payload.HTMLContent, "<script>") || !strings.Contains(payload.HTMLContent, "&lt;script&gt;") {
		t.Fatal("User's name is not HTML-escaped")
	}
	for _, color := range []string{"#3A6B4F", "#F6F7F4", "#20261F"} {
		if !strings.Contains(payload.HTMLContent, color) {
			t.Fatalf("missing existing design color %s", color)
		}
	}
}

func TestBrevoRendersResetAndCompletionWithExistingBrandAndFragmentLink(t *testing.T) {
	for _, c := range []struct {
		kind          email.Kind
		subject, text string
	}{
		{email.ResetLink, "Reset your StockFlow password", "30 minutes"},
		{email.ResetComplete, "Your StockFlow password was reset", "contact the person who maintains your system"},
	} {
		t.Run(string(c.kind), func(t *testing.T) {
			var payload map[string]any
			client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}, nil
			})}
			sender, err := email.NewBrevo(email.Config{APIKey: "test-key", SenderEmail: "sender@example.com"}, client)
			if err != nil {
				t.Fatal(err)
			}
			link := "https://stock-flow.example/reset-password#token=sample-token"
			if c.kind == email.ResetComplete {
				link = "https://stock-flow.example/login"
			}
			if err := sender.Send(t.Context(), email.Message{Kind: c.kind, To: "user@example.com", Name: "Napa", Link: link, ExpiresAt: time.Date(2026, 11, 11, 9, 30, 0, 0, time.UTC)}); err != nil {
				t.Fatal(err)
			}
			if payload["subject"] != c.subject {
				t.Fatalf("subject = %v", payload["subject"])
			}
			for _, key := range []string{"htmlContent", "textContent"} {
				body, _ := payload[key].(string)
				if !strings.Contains(body, link) || !strings.Contains(body, c.text) {
					t.Fatalf("%s missing action/content: %s", key, body)
				}
			}
		})
	}
}

func TestBrevoHidesProviderErrorsAndBoundsRequests(t *testing.T) {
	for _, c := range []struct {
		name           string
		status         int
		transportError error
	}{
		{"provider refusal", 400, nil}, {"network failure", 0, errors.New("failed with private-api-key and token")},
	} {
		t.Run(c.name, func(t *testing.T) {
			client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 10*time.Second {
					t.Fatal("provider request has no bounded timeout")
				}
				if c.transportError != nil {
					return nil, c.transportError
				}
				return &http.Response{StatusCode: c.status, Body: io.NopCloser(strings.NewReader("private-provider-body")), Header: http.Header{}}, nil
			})}
			sender, err := email.NewBrevo(email.Config{APIKey: "private-api-key", SenderEmail: "sender@example.com"}, client)
			if err != nil {
				t.Fatal(err)
			}
			err = sender.Send(t.Context(), email.Message{Kind: email.FirstTimeCode, To: "new@example.com", Name: "New", Code: "123456", Link: "https://stock-flow.example/password-setup"})
			if err == nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "token") {
				t.Fatalf("unsafe or missing error = %v", err)
			}
		})
	}
}

func TestBrevoRejectsInvalidConfigurationAndUnsafeTemplateInputs(t *testing.T) {
	for _, config := range []email.Config{{SenderEmail: "sender@example.com"}, {APIKey: "key"}, {APIKey: "key\r\nprivate-header", SenderEmail: "sender@example.com"}} {
		if _, err := email.NewBrevo(config, nil); err == nil || strings.Contains(err.Error(), "private-header") {
			t.Fatalf("invalid config error = %v", err)
		}
	}
	for _, message := range []email.Message{
		{Kind: email.FirstTimeCode, Code: "12345", Link: "https://stock-flow.example/password-setup"},
		{Kind: email.FirstTimeCode, Code: "123456", Link: "javascript:alert(1)"},
		{Kind: email.Kind("unsupported"), Link: "https://stock-flow.example/login"},
	} {
		if _, err := email.Render(message); err == nil {
			t.Fatalf("unsafe message accepted: %+v", message)
		}
	}
}

func TestBrevoDoesNotFollowRedirectsOrExposeSecretsOnCancellation(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://untrusted.example/private"}}, Body: io.NopCloser(strings.NewReader("private"))}, nil
	})}
	sender, err := email.NewBrevo(email.Config{APIKey: "private-key", SenderEmail: "sender@example.com"}, client)
	if err != nil {
		t.Fatal(err)
	}
	message := email.Message{Kind: email.FirstTimeCode, Code: "123456", Link: "https://stock-flow.example/password-setup", To: "new@example.com"}
	if err := sender.Send(t.Context(), message); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatalf("redirect = %v", err)
	}
	if calls != 1 {
		t.Fatalf("followed provider redirect (%d calls)", calls)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })}
	sender, err = email.NewBrevo(email.Config{APIKey: "private-key", SenderEmail: "sender@example.com"}, client)
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(ctx, message); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}
