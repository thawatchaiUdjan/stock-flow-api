package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

type Config struct{ APIKey, SenderEmail string }

type Brevo struct {
	config Config
	client *http.Client
}

func NewBrevo(config Config, client *http.Client) (*Brevo, error) {
	address, err := mail.ParseAddress(config.SenderEmail)
	if config.APIKey == "" || strings.ContainsAny(config.APIKey, "\r\n") || err != nil || address.Address != config.SenderEmail {
		return nil, errors.New("email delivery requires an API key and a verified sender email")
	}
	if client == nil {
		client = &http.Client{}
	}
	bounded := *client
	if bounded.Timeout <= 0 || bounded.Timeout > 10*time.Second {
		bounded.Timeout = 10 * time.Second
	}
	// Provider redirects must not forward the API key to a different origin.
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Brevo{config: config, client: &bounded}, nil
}

// Send waits for provider acceptance inside the request lifetime: no background
// goroutine can be frozen after a Vercel response. Never expose provider payloads.
func (b *Brevo) Send(ctx context.Context, message Message) error {
	content, err := Render(message)
	if err != nil {
		return err
	}
	type address struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	payload := struct {
		Sender  address   `json:"sender"`
		To      []address `json:"to"`
		Subject string    `json:"subject"`
		HTML    string    `json:"htmlContent"`
		Text    string    `json:"textContent"`
	}{address{"StockFlow", b.config.SenderEmail}, []address{{message.Name, message.To}}, content.Subject, content.HTML, content.Text}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return errors.New("cannot encode transactional email")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.brevo.com/v3/smtp/email", bytes.NewReader(encoded))
	if err != nil {
		return errors.New("cannot prepare transactional email")
	}
	req.Header.Set("api-key", b.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, err := b.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("email delivery request failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("email delivery refused (status %d)", response.StatusCode)
	}
	return nil
}
