package email

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
)

//go:embed templates/message.html
var messageHTML string

var messageTemplate = template.Must(template.New("transactional-email").Parse(messageHTML))
var sixDigits = regexp.MustCompile(`^[0-9]{6}$`)

// Content is the exact multipart content submitted to the provider, also used for
// offline visual previews so preview and delivery cannot drift (users-and-access R49).
type Content struct{ Subject, HTML, Text string }

type templateData struct {
	Name, Title, Intro, Code, Action, Link, Expiry, Help string
}

func Render(message Message) (Content, error) {
	link, err := url.Parse(message.Link)
	if err != nil || link.Scheme != "https" || link.Host == "" || link.User != nil {
		return Content{}, errors.New("email requires a trusted HTTPS destination")
	}
	data := templateData{Name: message.Name, Link: message.Link, Help: "If you did not request this, you can ignore this email."}
	var content Content
	switch message.Kind {
	case FirstTimeCode:
		if !sixDigits.MatchString(message.Code) {
			return Content{}, errors.New("first-time email requires a six-digit code")
		}
		content.Subject = "Your StockFlow setup code"
		data.Title, data.Intro, data.Code, data.Action = "Set up your password", "Use this code to verify your email and choose your own StockFlow password.", message.Code, "Set up password"
		data.Expiry = "This code expires in 10 minutes (" + message.ExpiresAt.UTC().Format("02 Jan 2006, 15:04 UTC") + ")."
		content.Text = fmt.Sprintf("Hi %s,\n\n%s\n\nYour code: %s\n\n%s\n\nSet up password: %s\n\n%s\n\nStockFlow\n", message.Name, data.Intro, message.Code, data.Expiry, message.Link, data.Help)
	case ResetLink:
		content.Subject = "Reset your StockFlow password"
		data.Title, data.Intro, data.Action = "Reset your password", "Someone requested a new password for your StockFlow User. Choose your own new password using the link below.", "Reset password"
		data.Expiry = "This link expires in 30 minutes (" + message.ExpiresAt.UTC().Format("02 Jan 2006, 15:04 UTC") + ")."
		content.Text = fmt.Sprintf("Hi %s,\n\n%s\n\n%s\n\nReset password: %s\n\nYour current password remains unchanged until you complete this reset.\n\n%s\n\nStockFlow\n", message.Name, data.Intro, data.Expiry, message.Link, data.Help)
	case ResetComplete:
		content.Subject = "Your StockFlow password was reset"
		data.Title, data.Intro, data.Action = "Your password was reset", "Your StockFlow password has changed. All existing sessions have ended. Log in again using your new password.", "Go to Login"
		data.Help = "If you did not make this change, contact the person who maintains your system."
		content.Text = fmt.Sprintf("Hi %s,\n\n%s\n\nLogin: %s\n\n%s\n\nStockFlow\n", message.Name, data.Intro, message.Link, data.Help)
	default:
		return Content{}, errors.New("unsupported transactional email kind")
	}
	var rendered bytes.Buffer
	if err := messageTemplate.Execute(&rendered, data); err != nil {
		return Content{}, errors.New("cannot render transactional email")
	}
	content.HTML = rendered.String()
	return content, nil
}
