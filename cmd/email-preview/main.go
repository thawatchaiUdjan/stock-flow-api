// Command email-preview renders the actual delivery templates with fictional data.
// It sends no email and reads no credentials.
package main

//go:generate go run . ../../docs/email/previews

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/email"
)

func main() {
	output := "docs/email/previews"
	if len(os.Args) > 1 {
		output = os.Args[1]
	}
	if err := render(output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func render(output string) error {
	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}
	for _, kind := range []email.Kind{email.FirstTimeCode, email.ResetLink, email.ResetComplete} {
		link := "https://stock-flow.example/password-setup"
		if kind == email.ResetLink {
			link = "https://stock-flow.example/reset-password#token=preview-token-not-valid"
		}
		if kind == email.ResetComplete {
			link = "https://stock-flow.example/login"
		}
		content, err := email.Render(email.Message{
			Kind: kind, To: "napa@example.com", Name: "Napa", Code: "012345", Link: link,
			ExpiresAt: time.Date(2026, 11, 11, 9, 30, 0, 0, time.UTC),
		})
		if err != nil {
			return err
		}
		for extension, text := range map[string]string{".html": content.HTML, ".txt": content.Text} {
			if err := os.WriteFile(filepath.Join(output, string(kind)+extension), []byte(text), 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
