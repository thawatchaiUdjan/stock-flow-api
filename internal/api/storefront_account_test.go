package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/email"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

func noPasswordYet(c *store.Customer) { c.PasswordHash = pgtype.Text{} }

func inactiveCustomer(c *store.Customer) { c.Status = "inactive" }

func (h *harness) post(path string, body map[string]string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, path, body)
}

func (h *harness) requestSetup(address string) *httptest.ResponseRecorder {
	return h.post("/v2/storefront/password-setup/request", map[string]string{"email": address})
}

func (h *harness) requestCustomerReset(address string) *httptest.ResponseRecorder {
	return h.post("/v2/storefront/password-reset/request", map[string]string{"email": address})
}

func (h *harness) verifySetup(address, code string) *httptest.ResponseRecorder {
	return h.post("/v2/storefront/password-setup/verify", map[string]string{"email": address, "code": code})
}

func (h *harness) setCustomerPassword(token, pw string) *httptest.ResponseRecorder {
	return h.post("/v2/storefront/password-setup/password", map[string]string{"token": token, "password": pw})
}

func (h *harness) resetCustomerPassword(token, pw string) *httptest.ResponseRecorder {
	return h.post("/v2/storefront/password-reset/password", map[string]string{"token": token, "password": pw})
}

// resetToken is the token in the Reset link the mailbox's last message holds.
func customerResetToken(t *testing.T, m *mailbox) string {
	t.Helper()
	last := m.messages[len(m.messages)-1]
	if last.Kind != email.ResetLink {
		t.Fatalf("last email = %+v, want a Reset link", last)
	}
	_, fragment, _ := strings.Cut(last.Link, "#token=")
	token, err := url.QueryUnescape(fragment)
	if err != nil || token == "" {
		t.Fatalf("no token in %q", last.Link)
	}
	return token
}

const newPassword = "a brand new one"

// customers R7, R8, R11: a Customer without a password gets a code, then sets one.
func TestACustomerWithoutAPasswordSetsOneWithAVerificationCode(t *testing.T) {
	h, m := newEmailHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")
	c := h.addCustomer("Somying@Example.com", noPasswordYet)
	assertProblem(t, h.customerLogIn("somying@example.com", password), http.StatusUnauthorized, "login-failed")

	rec := h.requestSetup("somying@example.com")

	if rec.Code != http.StatusAccepted || len(m.messages) != 1 {
		t.Fatalf("request: status = %d, emails = %d; body %s", rec.Code, len(m.messages), rec.Body)
	}
	sent := m.messages[0]
	if sent.Kind != email.VerificationCode || sent.To != "Somying@Example.com" || sent.Name != "Somying" || !sixDigits(sent.Code) ||
		sent.Link != "https://stock-flow.example/customer/password-setup" || !sent.ExpiresAt.Equal(h.clock.Now().Add(10*time.Minute)) {
		t.Errorf("email = %+v", sent)
	}
	verified := h.verifySetup("somying@example.com", sent.Code)
	if verified.Code != http.StatusOK {
		t.Fatalf("verify: status = %d; body %s", verified.Code, verified.Body)
	}
	grant := decode[verificationBody](t, verified).Token

	done := h.setCustomerPassword(grant, newPassword)

	if done.Code != http.StatusNoContent || len(done.Result().Cookies()) != 0 {
		t.Fatalf("set: status = %d, cookies %v; body %s", done.Code, done.Result().Cookies(), done.Body)
	}
	if rec := h.customerLogIn("somying@example.com", newPassword); rec.Code != http.StatusOK {
		t.Errorf("log in with the new password: status = %d", rec.Code)
	}
	assertProblem(t, h.setCustomerPassword(grant, newPassword), http.StatusGone, "customer-password-setup-invalid")
	page := h.activityLog("?actorKind=customer", owner)
	if len(page.Entries) != 1 || page.Entries[0].Action != "password-set" || page.Entries[0].Actor.Kind != "customer" ||
		*page.Entries[0].Actor.ID != c.ID || page.Entries[0].Subject.Kind != "customer" || len(page.Entries[0].Changes) != 0 {
		t.Errorf("entries = %+v, want one password-set by the Customer with no field", page.Entries)
	}
	if body := fmt.Sprintf("%+v", page.Entries); strings.Contains(body, grant) || strings.Contains(body, sent.Code) || strings.Contains(body, newPassword) {
		t.Errorf("an entry holds a secret: %s", body)
	}
}

// customers R7, R15, users-and-access R44: one answer, and nothing sent, for any email that is not eligible.
func TestPasswordSetupAndRecoveryRequestsAnswerTheSameForEveryEmailAndSendOnlyToEligibleOnes(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addCustomer("none@example.com", noPasswordYet)
	h.addCustomer("has@example.com")
	h.addCustomer("inactive@example.com", noPasswordYet, inactiveCustomer)
	h.addCustomer("inactive-pw@example.com", inactiveCustomer)
	h.addCustomer("gone@example.com", noPasswordYet, deletedCustomer)
	for _, request := range []struct {
		name  string
		do    func(string) *httptest.ResponseRecorder
		sends map[string]bool
	}{
		{"setup", h.requestSetup, map[string]bool{"none@example.com": true}},
		{"recovery", h.requestCustomerReset, map[string]bool{"none@example.com": true, "has@example.com": true}},
	} {
		var first string
		for _, address := range []string{"none@example.com", "has@example.com", "inactive@example.com", "inactive-pw@example.com", "gone@example.com", "nobody@example.com"} {
			h.clock.Advance(2 * time.Hour) // far apart: no limit is in play
			before := len(m.messages)

			rec := request.do(address)

			if rec.Code != http.StatusAccepted {
				t.Fatalf("%s %s: status = %d; body %s", request.name, address, rec.Code, rec.Body)
			}
			if first == "" {
				first = rec.Body.String()
			}
			if rec.Body.String() != first {
				t.Errorf("%s %s: body = %s, want %s", request.name, address, rec.Body, first)
			}
			if sent := len(m.messages) - before; (sent == 1) != request.sends[address] || sent > 1 {
				t.Errorf("%s %s: sent %d emails, want sent = %v (customers R15)", request.name, address, sent, request.sends[address])
			}
		}
	}
}

// customers R8: setup codes follow sign-up's rules: once, ten minutes, newest only, five wrong block.
func TestASetupCodeWorksOnceForTenMinutesOnlyTheNewestAndFiveWrongBlock(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addCustomer(customerEmail, noPasswordYet)
	h.requestSetup(customerEmail)
	h.clock.Advance(time.Minute)
	h.requestSetup(customerEmail)
	old, newest := m.messages[0].Code, m.messages[1].Code
	if old == newest {
		t.Skip("two random codes were equal")
	}
	assertProblem(t, h.verifySetup(customerEmail, old), http.StatusGone, "signup-code-invalid")
	h.clock.Set(m.messages[1].ExpiresAt.Add(-time.Second))
	if rec := h.verifySetup(customerEmail, newest); rec.Code != http.StatusOK {
		t.Fatalf("the newest just before it expires: status = %d; body %s", rec.Code, rec.Body)
	}
	assertProblem(t, h.verifySetup(customerEmail, newest), http.StatusGone, "signup-code-invalid")

	h.clock.Advance(time.Hour)
	h.requestSetup(customerEmail)
	right := m.messages[len(m.messages)-1].Code
	wrong := "000000"
	if right == wrong {
		wrong = "111111"
	}
	for range 5 {
		assertProblem(t, h.verifySetup(customerEmail, wrong), http.StatusGone, "signup-code-invalid")
	}
	assertProblem(t, h.verifySetup(customerEmail, right), http.StatusGone, "signup-code-invalid")
	h.clock.Advance(10 * time.Minute)
	h.requestSetup(customerEmail)
	if rec := h.verifySetup(customerEmail, m.messages[len(m.messages)-1].Code); rec.Code != http.StatusOK {
		t.Errorf("after the block: status = %d, want 200; body %s", rec.Code, rec.Body)
	}
}

// customers R7: a code is for the Customer it was sent to: not another's, and not a sign-up's.
func TestACodeForOneEmailDoesNotWorkForAnother(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addCustomer("one@example.com", noPasswordYet)
	h.addCustomer("two@example.com", noPasswordYet)
	h.requestSetup("one@example.com")

	assertProblem(t, h.verifySetup("two@example.com", m.messages[0].Code), http.StatusGone, "signup-code-invalid")
}

// customers R11: 6 to 128 characters, and a grant that failed validation is not used up.
func TestAPasswordIsSixToOneHundredTwentyEightCharacters(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addCustomer(customerEmail, noPasswordYet)
	h.requestSetup(customerEmail)
	grant := decode[verificationBody](t, h.verifySetup(customerEmail, m.messages[0].Code)).Token

	for name, pw := range map[string]string{"5": "12345", "129": strings.Repeat("a", 129)} {
		if rec := h.setCustomerPassword(grant, pw); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("a password of %s: status = %d, want 422", name, rec.Code)
		}
	}
	if rec := h.setCustomerPassword(grant, strings.Repeat("a", 128)); rec.Code != http.StatusNoContent {
		t.Fatalf("a password of 128: status = %d; body %s", rec.Code, rec.Body)
	}
	if rec := h.customerLogIn(customerEmail, strings.Repeat("a", 128)); rec.Code != http.StatusOK {
		t.Errorf("log in: status = %d", rec.Code)
	}
}

func TestAPasswordSetupGrantExpiresTenMinutesAfterItIsGiven(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addCustomer(customerEmail, noPasswordYet)
	h.requestSetup(customerEmail)
	grant := decode[verificationBody](t, h.verifySetup(customerEmail, m.messages[0].Code)).Token
	h.clock.Advance(10 * time.Minute)

	assertProblem(t, h.setCustomerPassword(grant, newPassword), http.StatusGone, "customer-password-setup-invalid")
	assertProblem(t, h.setCustomerPassword("not-a-grant", newPassword), http.StatusGone, "customer-password-setup-invalid")
}

// customers R12, R18, activity-log R4: a Reset link, once, 30 minutes, newest only; it ends every
// session, emails the Customer, and is recorded.
func TestAResetLinkSetsANewPasswordOnceEndsEverySessionAndEmailsTheCustomer(t *testing.T) {
	h, m := newEmailHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")
	h.addCustomer(customerEmail)
	other := h.addCustomer("other@example.com")
	phone, laptop := h.customerSession(customerEmail), h.customerSession(customerEmail)
	theirs := h.customerSession("other@example.com")
	h.clock.Advance(time.Hour)

	if rec := h.requestCustomerReset("SOMYING@example.com"); rec.Code != http.StatusAccepted {
		t.Fatalf("request: status = %d; body %s", rec.Code, rec.Body)
	}
	first := customerResetToken(t, m)
	h.clock.Advance(time.Minute)
	h.requestCustomerReset(customerEmail)
	link := customerResetToken(t, m)
	if link == first {
		t.Fatal("two requests made one link")
	}
	sent := m.messages[len(m.messages)-1]
	if sent.To != customerEmail || sent.Name != "Somying" || !strings.HasPrefix(sent.Link, "https://stock-flow.example/customer/reset-password#token=") ||
		!sent.ExpiresAt.Equal(h.clock.Now().Add(30*time.Minute)) {
		t.Errorf("email = %+v", sent)
	}
	assertProblem(t, h.resetCustomerPassword(first, newPassword), http.StatusGone, "customer-password-reset-invalid")
	// A request does not change the password or end a session (users-and-access R47).
	if rec := h.customerMe(phone); rec.Code != http.StatusOK {
		t.Errorf("a session after the request: status = %d, want 200", rec.Code)
	}
	assertProblem(t, h.setCustomerPassword(link, newPassword), http.StatusGone, "customer-password-setup-invalid")
	before := len(m.messages)

	done := h.resetCustomerPassword(link, newPassword)

	if done.Code != http.StatusNoContent || len(done.Result().Cookies()) != 0 {
		t.Fatalf("reset: status = %d; body %s", done.Code, done.Body)
	}
	if len(m.messages) != before+1 || m.messages[before].Kind != email.ResetComplete || m.messages[before].To != customerEmail ||
		m.messages[before].Link != "https://stock-flow.example/customer/login" {
		t.Errorf("emails after = %+v, want a password-changed notice", m.messages[before:])
	}
	assertProblem(t, h.customerLogIn(customerEmail, password), http.StatusUnauthorized, "login-failed")
	if rec := h.customerLogIn(customerEmail, newPassword); rec.Code != http.StatusOK {
		t.Errorf("log in with the new password: status = %d", rec.Code)
	}
	for name, cookie := range map[string]*http.Cookie{"phone": phone, "laptop": laptop} {
		assertProblem(t, h.customerMe(cookie), http.StatusUnauthorized, "session-ended")
		_ = name
	}
	if rec := h.customerMe(theirs); rec.Code != http.StatusOK {
		t.Errorf("another Customer's session: status = %d, want 200", rec.Code)
	}
	assertProblem(t, h.resetCustomerPassword(link, "yet another one"), http.StatusGone, "customer-password-reset-invalid")
	page := h.activityLog("?actorKind=customer", owner)
	if len(page.Entries) != 1 || page.Entries[0].Action != "password-reset" || page.Entries[0].Actor.Name != "Somying" || len(page.Entries[0].Changes) != 0 {
		t.Errorf("entries = %+v, want one password-reset by the Customer", page.Entries)
	}
	_ = other
}

// customers R12: the link expires 30 minutes after it is sent.
func TestAResetLinkExpiresThirtyMinutesAfterItIsSent(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addCustomer(customerEmail)
	h.requestCustomerReset(customerEmail)
	link := customerResetToken(t, m)
	h.clock.Advance(30*time.Minute - time.Second)
	if rec := h.resetCustomerPassword(link, "123456"); rec.Code != http.StatusNoContent {
		t.Fatalf("just before: status = %d; body %s", rec.Code, rec.Body)
	}
	h.clock.Advance(time.Hour)
	h.requestCustomerReset(customerEmail)
	late := customerResetToken(t, m)
	h.clock.Advance(30 * time.Minute)

	assertProblem(t, h.resetCustomerPassword(late, newPassword), http.StatusGone, "customer-password-reset-invalid")
}

// customers R12, users-and-access R47: a failed notice does not undo the change.
func TestAFailedPasswordChangedNoticeDoesNotUndoTheReset(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addCustomer(customerEmail)
	h.requestCustomerReset(customerEmail)
	link := customerResetToken(t, m)
	m.err = fmt.Errorf("provider down")

	if rec := h.resetCustomerPassword(link, newPassword); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
	}

	m.err = nil
	if rec := h.customerLogIn(customerEmail, newPassword); rec.Code != http.StatusOK {
		t.Errorf("log in: status = %d", rec.Code)
	}
}

// customers R12 (Forgot password): a Customer without a password is sent a code instead.
func TestForgotPasswordSendsACodeToACustomerWithoutAPassword(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addCustomer(customerEmail, noPasswordYet)

	h.requestCustomerReset(customerEmail)

	if len(m.messages) != 1 || m.messages[0].Kind != email.VerificationCode {
		t.Errorf("emails = %+v, want a Verification code", m.messages)
	}
}

// customers R15: a link or code stops working when the Customer is made Inactive.
func TestAResetLinkAndAGrantStopWorkingForACustomerMadeInactive(t *testing.T) {
	h, m := newEmailHarness(t)
	c := h.addCustomer(customerEmail)
	h.requestCustomerReset(customerEmail)
	link := customerResetToken(t, m)
	c.Status = "inactive"
	h.store.SetCustomer(c)

	assertProblem(t, h.resetCustomerPassword(link, newPassword), http.StatusGone, "customer-password-reset-invalid")

	d := h.addCustomer("other@example.com", noPasswordYet)
	h.requestSetup("other@example.com")
	code := m.messages[len(m.messages)-1].Code
	d.Status = "inactive"
	h.store.SetCustomer(d)
	assertProblem(t, h.verifySetup("other@example.com", code), http.StatusGone, "signup-code-invalid")
}

// users-and-access R57: Forgot password is not blocked by failed logins.
func TestForgotPasswordIsNotBlockedByFailedLogins(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addCustomer(customerEmail)
	for i := range 5 {
		h.customerFailAt(limitsBase.Add(time.Duration(i)*time.Second), "203.0.113.7", customerEmail)
	}
	assertBlocked(t, h.customerLoginAt(limitsBase.Add(10*time.Second), "203.0.113.7", customerEmail, password), 15*time.Minute-6*time.Second)

	h.requestCustomerReset(customerEmail)

	if len(m.messages) != 1 || m.messages[0].Kind != email.ResetLink {
		t.Errorf("emails = %+v, want a Reset link", m.messages)
	}
}

func TestCustomerPasswordOperationsAnswer503WhenEmailIsNotConfigured(t *testing.T) {
	h := newHarness(t)

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"request setup": h.requestSetup("a@example.com"), "verify": h.verifySetup("a@example.com", "123456"),
		"set": h.setCustomerPassword("t", newPassword), "request reset": h.requestCustomerReset("a@example.com"),
		"reset": h.resetCustomerPassword("t", newPassword),
	} {
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, want 503", name, rec.Code)
		}
	}
}

func (h *harness) patchMe(body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPatch, "/v2/storefront/me", body, cookies...)
}

func validAddress() map[string]string {
	return map[string]string{
		"recipient": "Somying Jaidee", "phone": "0812345678", "line": "99/1 Rama IV Road", "subdistrict": "Silom",
		"district": "Bang Rak", "province": "Bangkok", "postcode": "10500",
	}
}

type profileBody struct {
	ID              int64             `json:"id"`
	Name            string            `json:"name"`
	Email           string            `json:"email"`
	Phone           string            `json:"phone"`
	DeliveryAddress map[string]string `json:"deliveryAddress"`
}

// customers R13, activity-log R4: a Customer changes their own name, phone and saved Delivery address.
func TestACustomerChangesTheirNamePhoneAndDeliveryAddressAndEachIsRecorded(t *testing.T) {
	h := newHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")
	c := h.addCustomer("somying@example.com")
	cookie := h.customerSession("somying@example.com")
	if got := decode[profileBody](t, h.customerMe(cookie)); got.DeliveryAddress != nil {
		t.Fatalf("a new Customer's address = %+v, want none", got.DeliveryAddress)
	}

	rec := h.patchMe(map[string]any{"name": "Somying J.", "phone": "021234567", "deliveryAddress": validAddress()}, cookie)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
	}
	want := profileBody{ID: c.ID, Name: "Somying J.", Email: "somying@example.com", Phone: "021234567", DeliveryAddress: validAddress()}
	if got := decode[profileBody](t, rec); !reflect.DeepEqual(got, want) {
		t.Errorf("answer = %+v, want %+v", got, want)
	}
	if got := decode[profileBody](t, h.customerMe(cookie)); !reflect.DeepEqual(got, want) {
		t.Errorf("me = %+v, want %+v", got, want)
	}
	page := h.activityLog("?actorKind=customer", owner)
	byAction := map[string]activityEntry{}
	for _, e := range page.Entries {
		byAction[e.Action] = e
	}
	if len(page.Entries) != 3 {
		t.Fatalf("entries = %+v, want three", page.Entries)
	}
	for action, w := range map[string][2]any{
		"name-changed": {"Somying", "Somying J."}, "phone-changed": {"0812345678", "021234567"},
	} {
		e := byAction[action]
		if len(e.Changes) != 1 || e.Changes[0].Before != w[0] || e.Changes[0].After != w[1] || e.Actor.Kind != "customer" || *e.Actor.ID != c.ID || e.Subject.Ref != "somying@example.com" {
			t.Errorf("%s = %+v", action, e)
		}
	}
	e := byAction["delivery-address-changed"]
	after, _ := e.Changes[0].After.(map[string]any)
	if len(e.Changes) != 1 || e.Changes[0].Before != nil || after["postcode"] != "10500" || after["province"] != "Bangkok" {
		t.Errorf("delivery-address-changed = %+v", e)
	}

	// A change that changes nothing is recorded as nothing; a later address shows the one before.
	h.patchMe(map[string]any{"name": "Somying J."}, cookie)
	next := validAddress()
	next["postcode"] = "10110"
	h.patchMe(map[string]any{"deliveryAddress": next}, cookie)
	page = h.activityLog("?actorKind=customer", owner)
	if len(page.Entries) != 4 || page.Entries[0].Action != "delivery-address-changed" || page.Entries[0].Changes[0].Before.(map[string]any)["postcode"] != "10500" {
		t.Errorf("entries = %+v, want one more: the address, from 10500", page.Entries)
	}
}

// customers R13: the email and password are not the Customer's to change here.
func TestACustomerCannotChangeTheirEmailOrPasswordWhileLoggedIn(t *testing.T) {
	h := newHarness(t)
	h.addCustomer("somying@example.com")
	cookie := h.customerSession("somying@example.com")

	for name, body := range map[string]map[string]any{
		"email": {"email": "new@example.com"}, "password": {"password": "something else"}, "status": {"status": "inactive"},
	} {
		if rec := h.patchMe(body, cookie); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422; body %s", name, rec.Code, rec.Body)
		}
	}

	if got := decode[profileBody](t, h.customerMe(cookie)); got.Email != "somying@example.com" {
		t.Errorf("email = %q", got.Email)
	}
	if rec := h.customerLogIn("somying@example.com", password); rec.Code != http.StatusOK {
		t.Errorf("the old password: status = %d, want it unchanged", rec.Code)
	}
}

// customers R4, R2, storefront R7: what a profile may hold, and its edges.
func TestAProfileIsValidatedAndTakesTheEdges(t *testing.T) {
	h := newHarness(t)
	h.addCustomer("somying@example.com")
	cookie := h.customerSession("somying@example.com")
	thai := func(n int) string { return strings.Repeat("ก", n) }
	with := func(field, value string) map[string]any {
		a := validAddress()
		a[field] = value
		return map[string]any{"deliveryAddress": a}
	}
	missing := validAddress()
	delete(missing, "line")

	for name, body := range map[string]any{
		"a blank name":                 map[string]any{"name": "  "},
		"a name of 121":                map[string]any{"name": thai(121)},
		"a phone starting at 8":        map[string]any{"phone": "812345678"},
		"a phone of 11 digits":         map[string]any{"phone": "08123456789"},
		"a blank recipient":            with("recipient", " "),
		"a recipient of 121":           with("recipient", thai(121)),
		"a recipient phone of 8":       with("phone", "01234567"),
		"a line of 201":                with("line", thai(201)),
		"a sub-district of 81":         with("subdistrict", thai(81)),
		"a district of 81":             with("district", thai(81)),
		"a province that is not one":   with("province", "Atlantis"),
		"a province in the wrong case": with("province", "bangkok"),
		"a postcode of 4 digits":       with("postcode", "1050"),
		"a postcode of 6 digits":       with("postcode", "105000"),
		"a postcode with a letter":     with("postcode", "1050x"),
		"an address without a line":    map[string]any{"deliveryAddress": missing},
	} {
		if rec := h.patchMe(body, cookie); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422; body %s", name, rec.Code, rec.Body)
		}
	}

	edge := validAddress()
	edge["recipient"], edge["line"], edge["subdistrict"], edge["district"], edge["province"] = thai(120), thai(200), thai(80), thai(80), "Yasothon"
	if rec := h.patchMe(map[string]any{"name": thai(120), "phone": "021234567", "deliveryAddress": edge}, cookie); rec.Code != http.StatusOK {
		t.Errorf("the longest of each: status = %d; body %s", rec.Code, rec.Body)
	}
	if rec := h.patchMe(map[string]any{}, cookie); rec.Code != http.StatusOK {
		t.Errorf("nothing to change: status = %d, want 200", rec.Code)
	}
}

// storefront R7: all 77 provinces, and only they, are published and accepted.
func TestAllSeventySevenProvincesAreAcceptedAndPublished(t *testing.T) {
	h := newHarness(t)
	h.addCustomer("somying@example.com")
	cookie := h.customerSession("somying@example.com")
	enum := h.contract.OpenAPI().Components.Schemas.Map()["DeliveryAddress"].Properties["province"].Enum
	seen := map[string]bool{}
	for _, p := range enum {
		seen[p.(string)] = true
		a := validAddress()
		a["province"] = p.(string)
		if rec := h.patchMe(map[string]any{"deliveryAddress": a}, cookie); rec.Code != http.StatusOK {
			t.Errorf("%v: status = %d; body %s", p, rec.Code, rec.Body)
		}
	}
	if len(enum) != 77 || len(seen) != 77 {
		t.Errorf("published provinces = %d (%d distinct), want 77", len(enum), len(seen))
	}
	for _, name := range []string{"Bangkok", "Chiang Mai", "Phuket", "Bueng Kan", "Yasothon", "Nakhon Si Thammarat"} {
		if !seen[name] {
			t.Errorf("%s is not published", name)
		}
	}
}

func TestChangingAProfileNeedsACustomerSession(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com")
	user := h.session("somchai@example.com")

	assertProblem(t, h.patchMe(map[string]any{"name": "x"}), http.StatusUnauthorized, "")
	assertProblem(t, h.patchMe(map[string]any{"name": "x"}, user), http.StatusUnauthorized, "")
	assertProblem(t, h.patchMe(map[string]any{"name": "x"}, &http.Cookie{Name: "customer_session", Value: user.Value}), http.StatusUnauthorized, "session-ended")
}

// activity-log R3: a profile change and its entry are saved together.
func TestAProfileChangeWhoseEntryFailsIsNotSaved(t *testing.T) {
	h := newHarness(t)
	h.addCustomer("somying@example.com")
	cookie := h.customerSession("somying@example.com")
	h.store.ActivityErr = fmt.Errorf("activity_log is full")

	rec := h.patchMe(map[string]any{"name": "Changed"}, cookie)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body %s", rec.Code, rec.Body)
	}
	h.store.ActivityErr = nil
	if got := decode[profileBody](t, h.customerMe(cookie)); got.Name != "Somying" {
		t.Errorf("name = %q, want it unchanged", got.Name)
	}
}

// customers R8, users-and-access R43: setup and recovery sends count with sign-up's, apart from the Users'.
func TestCustomerSetupAndRecoverySendsAreCountedApartFromUsers(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addUser(customerEmail, func(u *store.User) { u.PasswordHash = pgtype.Text{} })
	h.addCustomer(customerEmail, noPasswordYet)
	for range 5 {
		h.requestCode(customerEmail)
		h.clock.Advance(time.Minute)
	}
	if len(m.messages) != 5 {
		t.Fatalf("the User's emails = %d, want five", len(m.messages))
	}

	h.requestSetup(customerEmail)

	if len(m.messages) != 6 || m.messages[5].Kind != email.VerificationCode {
		t.Fatalf("emails = %d, want the Customer's code as a sixth: the Users' five are not theirs", len(m.messages))
	}
	h.clock.Advance(time.Minute)
	for range 4 {
		h.requestSetup(customerEmail)
		h.clock.Advance(time.Minute)
	}
	h.requestCustomerReset(customerEmail)
	if len(m.messages) != 10 {
		t.Errorf("emails = %d, want the Customer's fifth and no sixth", len(m.messages))
	}
}
