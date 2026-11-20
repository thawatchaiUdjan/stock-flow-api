// Package email owns StockFlow's transactional email content and delivery adapter.
package email

import "time"

type Kind string

const (
	FirstTimeCode Kind = "first-time-code"
	ResetLink     Kind = "reset-link"
	ResetComplete Kind = "reset-complete"
	// VerificationCode is a Customer's code to confirm their email on sign-up (customers R5, R18).
	VerificationCode Kind = "verification-code"
)

// Message contains only the data for one transactional email, never a password.
type Message struct {
	Kind      Kind
	To        string
	Name      string
	Code      string
	Link      string
	ExpiresAt time.Time
}
