package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/email"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

const customerEmail = "somying@example.com"

type customerBody struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// addCustomer seeds an Active Customer with a password, unless change says otherwise.
func (h *harness) addCustomer(address string, change ...func(*store.Customer)) store.Customer {
	h.t.Helper()
	c := store.Customer{
		Name: "Somying", Email: address, Phone: "0812345678", Status: "active", CreatedAt: h.clock.Now(),
		PasswordHash: pgtype.Text{String: cheap.Hash(password), Valid: true},
	}
	for _, f := range change {
		f(&c)
	}
	return h.store.AddCustomer(c)
}

// deletedCustomer makes the Customer a Deleted one (customers R16).
func deletedCustomer(c *store.Customer) {
	c.Status = "inactive"
	c.DeletedAt = pgtype.Timestamptz{Time: c.CreatedAt, Valid: true}
	c.DeletedBy = pgtype.Int8{Int64: 1, Valid: true}
}

func (h *harness) requestSignup(address string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/v2/storefront/signup/request", map[string]string{"email": address})
}

func (h *harness) verifySignup(address, code string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/v2/storefront/signup/verify", map[string]string{"email": address, "code": code})
}

func (h *harness) completeSignup(token, name, phone, pw string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/v2/storefront/signup/complete", map[string]string{"token": token, "name": name, "phone": phone, "password": pw})
}

// grantFor runs the first two sign-up steps and returns the grant.
func (h *harness) grantFor(m *mailbox, address string) string {
	h.t.Helper()
	if rec := h.requestSignup(address); rec.Code != http.StatusAccepted {
		h.t.Fatalf("request a code: status = %d; body %s", rec.Code, rec.Body)
	}
	rec := h.verifySignup(address, m.messages[len(m.messages)-1].Code)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("verify: status = %d; body %s", rec.Code, rec.Body)
	}
	return decode[verificationBody](h.t, rec).Token
}

// signUp signs a Customer up completely and returns them.
func (h *harness) signUp(m *mailbox, address string) customerBody {
	h.t.Helper()
	rec := h.completeSignup(h.grantFor(m, address), "Somying", "0812345678", password)
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("complete: status = %d; body %s", rec.Code, rec.Body)
	}
	return decode[customerBody](h.t, rec)
}

func (h *harness) customerLogIn(address, pw string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/v2/storefront/login", map[string]string{"email": address, "password": pw})
}

func customerCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "customer_session" {
			return cookie
		}
	}
	t.Fatalf("no customer_session cookie in %v", rec.Header())
	return nil
}

// customerSession logs the Customer in and returns the cookie the API set.
func (h *harness) customerSession(address string) *http.Cookie {
	h.t.Helper()
	rec := h.customerLogIn(address, password)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("customer log in: status = %d; body %s", rec.Code, rec.Body)
	}
	return customerCookie(h.t, rec)
}

func (h *harness) customerMe(cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodGet, "/v2/storefront/me", nil, cookies...)
}

func (h *harness) customerLogOut(cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/v2/storefront/logout", nil, cookies...)
}

// customerLoginAt logs a Customer in from ip with the clock at t, as loginAt does a User.
func (h *harness) customerLoginAt(t time.Time, ip, address, pw string) *httptest.ResponseRecorder {
	h.t.Helper()
	h.clock.Set(t)
	body, _ := json.Marshal(map[string]string{"email": address, "password": pw})
	return h.from(ip+":1234", nil, http.MethodPost, "/v2/storefront/login", string(body))
}

func (h *harness) customerFailAt(t time.Time, ip, address string) {
	h.t.Helper()
	assertProblem(h.t, h.customerLoginAt(t, ip, address, "wrong password"), http.StatusUnauthorized, "login-failed")
}

func sixDigits(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// customers R5: the tracer bullet. R2, R4, R9, R10: what the Customer chose is theirs.
func TestACustomerSignsUpWithACodeThenCompletesAndLogsIn(t *testing.T) {
	h, m := newEmailHarness(t)

	rec := h.requestSignup("Somying@Example.com")
	if rec.Code != http.StatusAccepted || decode[struct {
		CodeDelivery string `json:"codeDelivery"`
	}](t, rec).CodeDelivery != "sent" {
		t.Fatalf("request: status = %d; body %s", rec.Code, rec.Body)
	}
	if len(m.messages) != 1 {
		t.Fatalf("emails = %d, want one", len(m.messages))
	}
	sent := m.messages[0]
	if sent.Kind != email.VerificationCode || sent.To != "Somying@Example.com" || !sixDigits(sent.Code) ||
		sent.Link != "https://stock-flow.example/signup" || !sent.ExpiresAt.Equal(h.clock.Now().Add(10*time.Minute)) {
		t.Errorf("email = %+v", sent)
	}
	verified := h.verifySignup("somying@example.com", sent.Code)
	if verified.Code != http.StatusOK {
		t.Fatalf("verify: status = %d; body %s", verified.Code, verified.Body)
	}
	grant := decode[verificationBody](t, verified)
	if grant.Token == "" || !grant.ExpiresAt.Equal(h.clock.Now().Add(10*time.Minute)) {
		t.Errorf("grant = %+v", grant)
	}
	completed := h.completeSignup(grant.Token, "Somying Jaidee", "0812345678", password)

	if completed.Code != http.StatusCreated {
		t.Fatalf("complete: status = %d; body %s", completed.Code, completed.Body)
	}
	got := decode[customerBody](t, completed)
	if got.ID == 0 || got.Name != "Somying Jaidee" || got.Email != "somying@example.com" || got.Phone != "0812345678" {
		t.Errorf("Customer = %+v", got)
	}
	if cookies := completed.Result().Cookies(); len(cookies) != 0 {
		t.Errorf("sign-up set cookies %v, want none: it starts no session", cookies)
	}
	if rec := h.customerMe(); rec.Code != http.StatusUnauthorized {
		t.Errorf("me without a session: status = %d, want 401", rec.Code)
	}
	logged := h.customerLogIn("SOMYING@example.com", password)
	if logged.Code != http.StatusOK || decode[customerBody](t, logged) != got {
		t.Fatalf("log in: status = %d; body %s", logged.Code, logged.Body)
	}
	cookie := customerCookie(t, logged)
	if !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" {
		t.Errorf("cookie = %+v, want HttpOnly, Secure, on /", cookie)
	}
	if me := h.customerMe(cookie); me.Code != http.StatusOK || decode[customerBody](t, me) != got {
		t.Errorf("me: status = %d; body %s", me.Code, me.Body)
	}
}

// customers R5: nothing exists until the sign-up is complete.
func TestNoCustomerExistsUntilTheSignUpIsCompleted(t *testing.T) {
	h, m := newEmailHarness(t)
	h.grantFor(m, customerEmail)

	if _, err := h.store.GetCustomerByEmail(t.Context(), customerEmail); err == nil {
		t.Error("a Customer exists after the code was verified")
	}
	assertProblem(t, h.customerLogIn(customerEmail, password), http.StatusUnauthorized, "login-failed")
	h.clock.Advance(time.Minute)
	if rec := h.requestSignup(customerEmail); rec.Code != http.StatusAccepted {
		t.Errorf("asking again: status = %d, want 202, not customer-email-taken", rec.Code)
	}
}

// customers R3, R6: nothing is sent to an email that has a Customer; a User does not count (R1).
func TestAnEmailThatHasACustomerIsToldSoAndNothingIsSent(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addCustomer("Somying@Example.com")

	assertProblem(t, h.requestSignup("sOMYING@example.com"), http.StatusConflict, "customer-email-taken")

	if len(m.messages) != 0 {
		t.Errorf("sent %d emails, want none", len(m.messages))
	}
	h.addUser("somchai@example.com")
	if rec := h.requestSignup("somchai@example.com"); rec.Code != http.StatusAccepted || len(m.messages) != 1 {
		t.Errorf("an email a User has: status = %d, emails = %d, want 202 and one (customers R1)", rec.Code, len(m.messages))
	}
}

// customers R6: a Deleted Customer's email starts a new Customer with no link to the old one.
func TestADeletedCustomersEmailSignsUpAfresh(t *testing.T) {
	h, m := newEmailHarness(t)
	old := h.addCustomer(customerEmail, deletedCustomer)

	fresh := h.signUp(m, customerEmail)

	if fresh.ID == old.ID {
		t.Errorf("the new Customer is the old one: %+v", fresh)
	}
	if rec := h.customerLogIn(customerEmail, password); rec.Code != http.StatusOK || decode[customerBody](t, rec).ID != fresh.ID {
		t.Errorf("log in: status = %d; body %s", rec.Code, rec.Body)
	}
}

// customers R8: six digits, ten minutes, once, newest only.
func TestAVerificationCodeWorksOnceForTenMinutesAndOnlyTheNewest(t *testing.T) {
	h, m := newEmailHarness(t)
	h.requestSignup(customerEmail)
	h.clock.Advance(time.Minute)
	h.requestSignup(customerEmail)
	old, newest := m.messages[0].Code, m.messages[1].Code
	if old == newest {
		t.Skip("two random codes were equal")
	}

	assertProblem(t, h.verifySignup(customerEmail, old), http.StatusGone, "signup-code-invalid")
	// The newest was sent at +1m, so it expires at +11m.
	h.clock.Set(m.messages[1].ExpiresAt.Add(-time.Second))
	if rec := h.verifySignup(customerEmail, newest); rec.Code != http.StatusOK {
		t.Fatalf("the newest code just before it expires: status = %d; body %s", rec.Code, rec.Body)
	}
	assertProblem(t, h.verifySignup(customerEmail, newest), http.StatusGone, "signup-code-invalid")
}

func TestAVerificationCodeExpiresTenMinutesAfterItIsSent(t *testing.T) {
	h, m := newEmailHarness(t)
	h.requestSignup(customerEmail)
	h.clock.Advance(10 * time.Minute)

	assertProblem(t, h.verifySignup(customerEmail, m.messages[0].Code), http.StatusGone, "signup-code-invalid")
}

// customers R8: five wrong attempts block codes for the email for ten minutes, even for the right code.
func TestFiveWrongVerificationCodesBlockTheEmailForTenMinutes(t *testing.T) {
	h, m := newEmailHarness(t)
	h.requestSignup(customerEmail)
	right := m.messages[0].Code
	wrong := "000000"
	if right == wrong {
		wrong = "111111"
	}
	for range 4 {
		assertProblem(t, h.verifySignup(customerEmail, wrong), http.StatusGone, "signup-code-invalid")
	}
	h.clock.Advance(time.Minute)
	h.requestSignup(customerEmail) // a resend does not lift it either
	right = m.messages[len(m.messages)-1].Code
	assertProblem(t, h.verifySignup(customerEmail, wrong), http.StatusGone, "signup-code-invalid") // the fifth

	assertProblem(t, h.verifySignup(customerEmail, right), http.StatusGone, "signup-code-invalid")
	h.clock.Advance(10*time.Minute - time.Second)
	assertProblem(t, h.verifySignup(customerEmail, right), http.StatusGone, "signup-code-invalid")
	h.requestSignup(customerEmail)
	right = m.messages[len(m.messages)-1].Code
	h.clock.Advance(time.Second)
	if rec := h.verifySignup(customerEmail, right); rec.Code != http.StatusOK {
		t.Errorf("after the block: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
}

func TestFourWrongVerificationCodesStillAllowTheRightOne(t *testing.T) {
	h, m := newEmailHarness(t)
	h.requestSignup(customerEmail)
	right := m.messages[0].Code
	wrong := "000000"
	if right == wrong {
		wrong = "111111"
	}
	for range 4 {
		h.verifySignup(customerEmail, wrong)
	}

	if rec := h.verifySignup(customerEmail, right); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: four wrong attempts block nothing", rec.Code)
	}
}

// customers R8, users-and-access R43: one send per 60 seconds and five per email per hour,
// counted apart from the Users'.
func TestCustomerCodeSendsAreLimitedPerEmailAndCountedApartFromUsers(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addUser(customerEmail, func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	delivery := func() string {
		t.Helper()
		rec := h.requestSignup(customerEmail)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
		}
		return decode[struct {
			CodeDelivery string `json:"codeDelivery"`
		}](t, rec).CodeDelivery
	}
	if got := delivery(); got != "sent" {
		t.Fatalf("first = %q, want sent", got)
	}
	h.clock.Advance(59 * time.Second)
	if got := delivery(); got != "suppressed" || len(m.messages) != 1 {
		t.Fatalf("within 60 seconds = %q with %d emails, want suppressed and one", got, len(m.messages))
	}
	h.clock.Advance(time.Second)
	for range 4 {
		if got := delivery(); got != "sent" {
			t.Fatalf("within the hour's five = %q, want sent", got)
		}
		h.clock.Advance(time.Minute)
	}
	if got := delivery(); got != "suppressed" || len(m.messages) != 5 {
		t.Fatalf("the sixth = %q with %d emails, want suppressed and five", got, len(m.messages))
	}
	// The same address's User still gets their First-time code: their count is their own.
	h.requestCode(customerEmail)
	if last := m.messages[len(m.messages)-1]; len(m.messages) != 6 || last.Kind != email.FirstTimeCode {
		t.Errorf("the User's code: %d emails, last %+v, want a sixth, a First-time code", len(m.messages), last)
	}
	// And the other way round: five Users' requests leave the Customers' allowance whole.
	h.clock.Advance(time.Hour)
	h2, m2 := newEmailHarness(t)
	h2.addUser(customerEmail, func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	for range 5 {
		h2.requestCode(customerEmail)
		h2.clock.Advance(time.Minute)
	}
	if rec := h2.requestSignup(customerEmail); rec.Code != http.StatusAccepted || len(m2.messages) != 6 || m2.messages[5].Kind != email.VerificationCode {
		t.Errorf("after five Users' sends: status = %d, emails = %d, want a Verification code as the sixth", rec.Code, len(m2.messages))
	}
}

// customers R8, users-and-access R43: 30 sends per IP address per hour, counted apart from the Users'.
func TestCustomerCodeSendsAreLimitedPerIPAddressAndCountedApartFromUsers(t *testing.T) {
	h, m := newEmailHarness(t)
	for i := range 30 {
		rec := h.requestSignup(fmt.Sprintf("person%d@example.com", i))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("request %d: status = %d; body %s", i, rec.Code, rec.Body)
		}
	}
	if len(m.messages) != 30 {
		t.Fatalf("emails = %d, want 30", len(m.messages))
	}

	h.requestSignup("one-more@example.com")

	if len(m.messages) != 30 {
		t.Errorf("emails = %d, want the 31st suppressed", len(m.messages))
	}
	h.addUser("somchai@example.com", func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	h.requestCode("somchai@example.com")
	if len(m.messages) != 31 {
		t.Errorf("emails = %d, want the User's code sent: the Customers' 30 are not theirs", len(m.messages))
	}
}

// customers R2, R4, R11: the rules for what a Customer chooses, and their edges.
func TestSigningUpRefusesAnInvalidNamePhoneOrPasswordAndTakesTheirEdges(t *testing.T) {
	h, m := newEmailHarness(t)
	grant := h.grantFor(m, customerEmail)
	thai := strings.Repeat("ก", 120)
	for name, body := range map[string][3]string{
		"a blank name":              {"   ", "0812345678", password},
		"a name of only a tab":      {"\t", "0812345678", password},
		"a name of 121 characters":  {strings.Repeat("ก", 121), "0812345678", password},
		"a phone not starting at 0": {"Somying", "812345678", password},
		"a phone of 8 digits":       {"Somying", "01234567", password},
		"a phone of 11 digits":      {"Somying", "08123456789", password},
		"a phone with a dash":       {"Somying", "081-234567", password},
		"a password of 5":           {"Somying", "0812345678", "12345"},
		"a password of 129":         {"Somying", "0812345678", strings.Repeat("a", 129)},
	} {
		if rec := h.completeSignup(grant, body[0], body[1], body[2]); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422; body %s", name, rec.Code, rec.Body)
		}
	}

	// A refusal used up nothing: the grant still works, at the edges.
	rec := h.completeSignup(grant, thai, "021234567", strings.Repeat("p", 128))
	if rec.Code != http.StatusCreated || decode[customerBody](t, rec).Name != thai {
		t.Fatalf("a name of 120 Thai characters, a 9-digit phone, a 128-character password: status = %d; body %s", rec.Code, rec.Body)
	}
	if rec := h.customerLogIn(customerEmail, strings.Repeat("p", 128)); rec.Code != http.StatusOK {
		t.Errorf("log in with the 128-character password: status = %d", rec.Code)
	}
	other := h.completeSignup(h.grantFor(m, "other@example.com"), "สมหญิง ใจดี", "0812345678", "123456")
	if other.Code != http.StatusCreated {
		t.Errorf("a Thai name and a 6-character password: status = %d; body %s", other.Code, other.Body)
	}
}

// customers R5: a grant works once, for ten minutes, and the sign-up fails if a Customer took the email.
func TestASignUpGrantWorksOnceAndBeforeItExpires(t *testing.T) {
	h, m := newEmailHarness(t)
	grant := h.grantFor(m, customerEmail)
	assertProblem(t, h.completeSignup("not-a-grant", "Somying", "0812345678", password), http.StatusGone, "signup-invalid")

	if rec := h.completeSignup(grant, "Somying", "0812345678", password); rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
	}
	assertProblem(t, h.completeSignup(grant, "Somying", "0812345678", password), http.StatusGone, "signup-invalid")

	late := h.grantFor(m, "late@example.com")
	h.clock.Advance(10 * time.Minute)
	assertProblem(t, h.completeSignup(late, "Late", "0812345678", password), http.StatusGone, "signup-invalid")

	taken := h.grantFor(m, "taken@example.com")
	h.addCustomer("taken@example.com")
	assertProblem(t, h.completeSignup(taken, "Taken", "0812345678", password), http.StatusConflict, "customer-email-taken")
}

// activity-log R3, R4: the sign-up is recorded with the Customer as the actor, or not at all.
func TestTheSignUpIsRecordedInTheActivityLogWithTheCustomerAsActor(t *testing.T) {
	h, m := newEmailHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")
	grant := h.grantFor(m, customerEmail)
	h.store.ActivityErr = fmt.Errorf("activity_log is full")
	if rec := h.completeSignup(grant, "Somying", "0812345678", password); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body %s", rec.Code, rec.Body)
	}
	h.store.ActivityErr = nil
	if _, err := h.store.GetCustomerByEmail(t.Context(), customerEmail); err == nil {
		t.Fatal("the Customer was saved although its entry failed")
	}

	created := decode[customerBody](t, h.completeSignup(grant, "Somying", "0812345678", password))

	page := h.activityLog("?actorKind=customer", owner)
	if page.Total != 1 || len(page.Entries) != 1 {
		t.Fatalf("entries by a Customer = %+v, want one", page)
	}
	e := page.Entries[0]
	if e.Actor.Kind != "customer" || e.Actor.ID == nil || *e.Actor.ID != created.ID || e.Actor.Name != "Somying" || e.Actor.Email != customerEmail || e.Actor.Role != "" ||
		e.Action != "created" || e.Subject.Kind != "customer" || e.Subject.ID != created.ID || e.Subject.Name != "Somying" || e.Subject.Ref != customerEmail {
		t.Errorf("entry = %+v", e)
	}
}

// customers R7 (email configuration): sign-up needs email; a Customer's login does not.
func TestSigningUpAnswers503WhenEmailIsNotConfigured(t *testing.T) {
	h := newHarness(t)
	h.addCustomer(customerEmail)

	assertProblem(t, h.requestSignup("new@example.com"), http.StatusServiceUnavailable, "email-auth-unavailable")
	assertProblem(t, h.verifySignup("new@example.com", "123456"), http.StatusServiceUnavailable, "email-auth-unavailable")
	assertProblem(t, h.completeSignup("token", "Somying", "0812345678", password), http.StatusServiceUnavailable, "email-auth-unavailable")
	if rec := h.customerLogIn(customerEmail, password); rec.Code != http.StatusOK {
		t.Errorf("log in: status = %d, want 200", rec.Code)
	}
}

// failedCustomerLogIns is every way a Storefront login can fail (customers R9).
func failedCustomerLogIns(h *harness) map[string][2]string {
	h.addCustomer("somying@example.com")
	h.addCustomer("inactive@example.com", func(c *store.Customer) { c.Status = "inactive" })
	h.addCustomer("nopassword@example.com", func(c *store.Customer) { c.PasswordHash = pgtype.Text{} })
	h.addCustomer("gone@example.com", deletedCustomer)
	h.addUser("somchai@example.com") // a User's email is not a Customer's (customers R1)
	return map[string][2]string{
		"wrong password":      {"somying@example.com", "wrong password"},
		"unknown email":       {"nobody@example.com", password},
		"inactive Customer":   {"inactive@example.com", password},
		"no password yet":     {"nopassword@example.com", "anything"},
		"Deleted Customer":    {"gone@example.com", password},
		"a User's login":      {"somchai@example.com", password},
		"an empty password":   {"somying@example.com", ""},
		"an email in caps ok": {"SOMYING@example.com", "wrong password"},
	}
}

// customers R9: one answer for every failure, whatever the reason.
func TestEveryFailedCustomerLogInGetsTheSameAnswer(t *testing.T) {
	h := newHarness(t)
	var first string
	for name, attempt := range failedCustomerLogIns(h) {
		rec := h.customerLogIn(attempt[0], attempt[1])

		assertProblem(t, rec, http.StatusUnauthorized, "login-failed")
		if got := rec.Header().Get("Set-Cookie"); got != "" {
			t.Errorf("%s: Set-Cookie = %q, want none", name, got)
		}
		if first == "" {
			first = rec.Body.String()
		}
		if rec.Body.String() != first {
			t.Errorf("%s: body = %s, want %s", name, rec.Body, first)
		}
	}
}

// customers R9: nor from the time it took.
func TestEveryFailedCustomerLogInTakesTheSameTime(t *testing.T) {
	h := newHarness(t)
	took := map[string]time.Duration{}
	attempts := failedCustomerLogIns(h)
	for name, attempt := range attempts {
		start := h.clock.Now()
		h.customerLogIn(attempt[0], attempt[1])
		took[name] = h.clock.Now().Sub(start)
	}

	want := took["wrong password"]
	if want == 0 {
		t.Fatal("a wrong password took no time: no password was verified")
	}
	for name, got := range took {
		if got != want {
			t.Errorf("%s took %v, a wrong password %v", name, got, want)
		}
	}
}

// customers R9 (Inactive): a right password is not enough.
func TestAnInactiveCustomerCannotLogInWithTheRightPassword(t *testing.T) {
	h := newHarness(t)
	h.addCustomer(customerEmail, func(c *store.Customer) { c.Status = "inactive" })

	assertProblem(t, h.customerLogIn(customerEmail, password), http.StatusUnauthorized, "login-failed")
}

// customers R9: a Customer who changed while the password was checked gets no session.
func TestACustomerMadeInactiveWhileTheirPasswordIsCheckedGetsNoSession(t *testing.T) {
	h := newHarness(t)
	c := h.addCustomer(customerEmail)
	h.whileVerifying = func() {
		c.Status = "inactive"
		h.store.SetCustomer(c)
	}

	rec := h.customerLogIn(customerEmail, password)

	assertProblem(t, rec, http.StatusUnauthorized, "login-failed")
}

// customers R10: several devices, one expiry fixed at login, logout ends only its own.
func TestACustomerHasSeveralSessionsEachEndingSevenDaysAfterLogin(t *testing.T) {
	h := newHarness(t)
	h.addCustomer(customerEmail)
	start := h.clock.Now()
	phone, laptop := h.customerSession(customerEmail), h.customerSession(customerEmail)
	if phone.Value == laptop.Value {
		t.Fatal("two logins share one session")
	}

	if rec := h.customerLogOut(phone); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: status = %d; body %s", rec.Code, rec.Body)
	}

	assertProblem(t, h.customerMe(phone), http.StatusUnauthorized, "session-ended")
	if rec := h.customerMe(laptop); rec.Code != http.StatusOK {
		t.Errorf("the other device after a logout: status = %d, want 200", rec.Code)
	}
	// The fake hasher moves the clock a little at each login, so the edges are a second wide.
	h.clock.Set(start.Add(6 * 24 * time.Hour))
	if rec := h.customerMe(laptop); rec.Code != http.StatusOK {
		t.Errorf("after 6 days: status = %d, want 200", rec.Code)
	}
	h.clock.Set(start.Add(7*24*time.Hour - time.Second))
	if rec := h.customerMe(laptop); rec.Code != http.StatusOK {
		t.Errorf("just before 7 days, though it was used a day ago: status = %d, want 200", rec.Code)
	}
	h.clock.Set(start.Add(7*24*time.Hour + time.Second))
	assertProblem(t, h.customerMe(laptop), http.StatusUnauthorized, "session-ended")
	assertProblem(t, h.customerLogOut(laptop), http.StatusUnauthorized, "session-ended")
}

func TestACustomerSessionOperationsRefuseAVisitorAndAnUnknownCookie(t *testing.T) {
	h := newHarness(t)

	for _, rec := range []*httptest.ResponseRecorder{h.customerMe(), h.customerLogOut()} {
		assertProblem(t, rec, http.StatusUnauthorized, "")
	}
	stranger := &http.Cookie{Name: "customer_session", Value: "nothing"}
	assertProblem(t, h.customerMe(stranger), http.StatusUnauthorized, "session-ended")
	assertProblem(t, h.customerLogOut(stranger), http.StatusUnauthorized, "session-ended")
}

// customers R1: a Customer's session reaches nothing of StockFlow, whichever cookie carries it.
func TestACustomerSessionIsRefusedByEveryStockFlowOperation(t *testing.T) {
	h := newHarness(t)
	h.addCustomer(customerEmail)
	cookie := h.customerSession(customerEmail)
	asUser := &http.Cookie{Name: "session", Value: cookie.Value}

	for _, op := range []struct{ method, path string }{
		{http.MethodGet, "/me"}, {http.MethodPost, "/logout"}, {http.MethodGet, "/users"}, {http.MethodGet, "/v2/activity-log"},
		{http.MethodGet, "/products"}, {http.MethodGet, "/orders"}, {http.MethodGet, "/login-log"}, {http.MethodGet, "/v2/login-log"},
		{http.MethodGet, "/stock-movements"},
	} {
		assertProblem(t, h.do(op.method, op.path, nil, asUser), http.StatusUnauthorized, "session-ended")
		// And under its own name: StockFlow reads only `session`.
		assertProblem(t, h.do(op.method, op.path, nil, cookie), http.StatusUnauthorized, "")
	}
}

// customers R1: a User's session opens no Customer-only Storefront operation.
func TestAUserSessionIsRefusedByCustomerOnlyStorefrontOperations(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com")
	user := h.session("somchai@example.com")
	asCustomer := &http.Cookie{Name: "customer_session", Value: user.Value}

	for _, rec := range []*httptest.ResponseRecorder{
		h.customerMe(user), h.customerLogOut(user),
	} {
		assertProblem(t, rec, http.StatusUnauthorized, "")
	}
	assertProblem(t, h.customerMe(asCustomer), http.StatusUnauthorized, "session-ended")
	assertProblem(t, h.customerLogOut(asCustomer), http.StatusUnauthorized, "session-ended")
	if rec := h.me(user); rec.Code != http.StatusOK {
		t.Errorf("the User's own session: status = %d, want 200", rec.Code)
	}
}

// users-and-access R55, R57: 5 failures for an email in 15 minutes block that email on the
// Storefront, and not on StockFlow; the two count apart.
func TestFiveFailedCustomerLogInsBlockTheEmailOnTheStorefrontOnly(t *testing.T) {
	h := newHarness(t)
	h.addCustomer(customerEmail)
	h.addUser(customerEmail) // the same email may be a User's too (customers R1)
	ip := "203.0.113.7"
	for i := range 4 {
		h.customerFailAt(limitsBase.Add(time.Duration(i)*time.Minute), ip, customerEmail)
	}
	if rec := h.customerLoginAt(limitsBase.Add(4*time.Minute), ip, customerEmail, password); rec.Code != http.StatusOK {
		t.Fatalf("after 4 failures: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	h.customerFailAt(limitsBase.Add(4*time.Minute), ip, customerEmail)

	assertBlocked(t, h.customerLoginAt(limitsBase.Add(16*time.Minute), ip, customerEmail, password), 3*time.Minute)
	if rec := h.loginAt(limitsBase.Add(16*time.Minute), ip, customerEmail, password); rec.Code != http.StatusOK {
		t.Errorf("the same email on StockFlow: status = %d, want 200: the apps count apart", rec.Code)
	}
	if rec := h.customerLoginAt(limitsBase.Add(19*time.Minute), ip, customerEmail, password); rec.Code != http.StatusOK {
		t.Errorf("when the block ends: status = %d, want 200", rec.Code)
	}
}

func TestFiveFailedUserLogInsDoNotBlockTheSameEmailOnTheStorefront(t *testing.T) {
	h := newHarness(t)
	h.addCustomer(customerEmail)
	h.addUser(customerEmail)
	ip := "203.0.113.7"
	for i := range 5 {
		h.failAt(limitsBase.Add(time.Duration(i)*time.Minute), ip, customerEmail)
	}

	assertBlocked(t, h.loginAt(limitsBase.Add(5*time.Minute), ip, customerEmail, password), 14*time.Minute)

	if rec := h.customerLoginAt(limitsBase.Add(5*time.Minute), ip, customerEmail, password); rec.Code != http.StatusOK {
		t.Errorf("Storefront: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
}

// users-and-access R56, R57: 50 failures from an IP address block it on the Storefront, and not on StockFlow.
func TestFiftyFailedCustomerLogInsBlockTheIPAddressOnTheStorefrontOnly(t *testing.T) {
	h := newHarness(t)
	h.addCustomer(customerEmail)
	h.addUser(customerEmail)
	ip := "203.0.113.7"
	for i := range 50 {
		h.customerFailAt(limitsBase.Add(time.Duration(i)*time.Second), ip, fmt.Sprintf("guess%d@example.com", i))
	}

	assertBlocked(t, h.customerLoginAt(limitsBase.Add(time.Minute), ip, customerEmail, password), time.Hour-time.Minute)

	if rec := h.loginAt(limitsBase.Add(time.Minute), ip, customerEmail, password); rec.Code != http.StatusOK {
		t.Errorf("StockFlow from the same address: status = %d, want 200", rec.Code)
	}
	if rec := h.customerLoginAt(limitsBase.Add(time.Minute), "203.0.113.8", customerEmail, password); rec.Code != http.StatusOK {
		t.Errorf("another address: status = %d, want 200", rec.Code)
	}
}

type storefrontLogBody struct {
	Entries []struct {
		App    string     `json:"app"`
		Email  string     `json:"email"`
		Result string     `json:"result"`
		Actor  *actorBody `json:"actor"`
		IP     string     `json:"ip"`
	} `json:"entries"`
	Total *int64 `json:"total"`
}

// users-and-access R36, R38: the Login log shows the Storefront's logins, failures, blocks and
// logouts with the Customer; the legacy log shows none of them.
func TestTheLoginLogShowsStorefrontLoginsFailuresBlocksAndLogoutsWithTheCustomer(t *testing.T) {
	h := newHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")
	c := h.addCustomer(customerEmail)
	ip := "203.0.113.7"
	for i := range 5 {
		h.customerFailAt(limitsBase.Add(time.Duration(i)*time.Second), ip, strings.ToUpper(customerEmail))
	}
	assertBlocked(t, h.customerLoginAt(limitsBase.Add(10*time.Second), ip, customerEmail, password), 15*time.Minute-6*time.Second)
	h.customerFailAt(limitsBase.Add(20*time.Minute), ip, "nobody@example.com")
	rec := h.customerLoginAt(limitsBase.Add(21*time.Minute), ip, customerEmail, password)
	if rec.Code != http.StatusOK {
		t.Fatalf("log in: status = %d; body %s", rec.Code, rec.Body)
	}
	h.clock.Set(limitsBase.Add(22 * time.Minute))
	h.customerLogOut(customerCookie(t, rec))

	log := decode[storefrontLogBody](t, h.do(http.MethodGet, "/v2/login-log?offset=0&app=storefront", nil, owner))
	legacy := decode[loginLogEntriesBody](t, h.do(http.MethodGet, "/login-log?offset=0", nil, owner))

	var results []string
	for _, e := range log.Entries {
		results = append(results, e.Result)
	}
	want := []string{"logout", "login", "failed-login", "blocked", "failed-login", "failed-login", "failed-login", "failed-login", "failed-login"}
	if strings.Join(results, ",") != strings.Join(want, ",") || *log.Total != 9 {
		t.Fatalf("results = %v (total %d), want %v", results, *log.Total, want)
	}
	for i, e := range log.Entries {
		known := i != 2 // the failure for an email that is nobody's
		if e.App != "storefront" || (e.Actor != nil) != known {
			t.Errorf("entry %d = %+v", i, e)
		}
		if known && (e.Actor.Kind != "customer" || e.Actor.ID == nil || *e.Actor.ID != c.ID || e.Actor.Name != "Somying") {
			t.Errorf("entry %d actor = %+v, want the Customer", i, e.Actor)
		}
	}
	if log.Entries[2].Email != "nobody@example.com" || log.Entries[8].Email != strings.ToUpper(customerEmail) || log.Entries[1].IP != ip {
		t.Errorf("entries keep the email as typed and the IP: %+v", log.Entries)
	}
	if len(legacy.Entries) != 1 { // only the Owner's own login, which opened the session above
		t.Errorf("the legacy log = %+v, want only StockFlow's one login", legacy.Entries)
	}
}

// users-and-access R49, customers R18 are in internal/email; this is the delivery's failure.
func TestACodeEmailThatFailsToSendIsReportedAndTheCodeStillStands(t *testing.T) {
	h, m := newEmailHarness(t)
	m.err = fmt.Errorf("provider down")

	rec := h.requestSignup(customerEmail)

	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"failed"`) {
		t.Errorf("status = %d; body %s, want 202 with failed delivery", rec.Code, rec.Body)
	}
}
