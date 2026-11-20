package email_test

import (
	"strings"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/email"
)

// customers R18, users-and-access R49: English, StockFlow branding, HTML and plain text.
func TestAVerificationCodeEmailIsEnglishBrandedAndHasHTMLAndPlainText(t *testing.T) {
	content, err := email.Render(email.Message{
		Kind: email.VerificationCode, To: "somying@example.com", Code: "012345", Link: "https://stock-flow.example/signup",
		ExpiresAt: time.Date(2026, 11, 20, 9, 10, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatal(err)
	}
	if content.Subject != "Your StockFlow verification code" {
		t.Errorf("subject = %q", content.Subject)
	}
	for name, body := range map[string]string{"HTML": content.HTML, "text": content.Text} {
		for _, want := range []string{"StockFlow", "012345", "Hello,", "expires in 10 minutes (20 Nov 2026, 09:10 UTC)", "https://stock-flow.example/signup", "online store"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s lacks %q: %s", name, want, body)
			}
		}
		if strings.Contains(body, "Hi ,") {
			t.Errorf("%s greets nobody by name: %s", name, body)
		}
	}
	if !strings.Contains(content.HTML, `<html lang="en">`) || !strings.Contains(content.HTML, "Verify your email") || !strings.Contains(content.HTML, "max-width:600px") {
		t.Errorf("HTML is not the branded, responsive template: %s", content.HTML)
	}
}

func TestAVerificationCodeEmailNeedsASixDigitCodeAndAnHTTPSLink(t *testing.T) {
	for name, message := range map[string]email.Message{
		"a five-digit code": {Kind: email.VerificationCode, Code: "12345", Link: "https://stock-flow.example/signup"},
		"no code":           {Kind: email.VerificationCode, Link: "https://stock-flow.example/signup"},
		"a plain link":      {Kind: email.VerificationCode, Code: "123456", Link: "http://stock-flow.example/signup"},
	} {
		if _, err := email.Render(message); err == nil {
			t.Errorf("%s was rendered", name)
		}
	}
}

// A name, when there is one, is greeted and escaped, as in the other emails.
func TestAnEmailGreetsByNameWhenThereIsOne(t *testing.T) {
	content, err := email.Render(email.Message{Kind: email.ResetComplete, Name: "<b>Napa</b>", Link: "https://stock-flow.example/login"})

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content.Text, "Hi <b>Napa</b>,") || !strings.Contains(content.HTML, "Hi &lt;b&gt;Napa&lt;/b&gt;,") {
		t.Errorf("greeting: text %q, HTML %q", content.Text, content.HTML)
	}
}
