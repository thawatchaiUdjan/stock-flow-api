package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

type logEntry struct {
	ID    int64     `json:"id"`
	At    time.Time `json:"at"`
	Email string    `json:"email"`
	User  *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"user"`
	Result    string `json:"result"`
	IP        string `json:"ip"`
	UserAgent string `json:"userAgent"`
}

type logPage struct {
	Entries    []logEntry `json:"entries"`
	NextBefore *int64     `json:"nextBefore"`
}

// from sends a request as a visitor with these headers and this address.
func (h *harness) from(remoteAddr string, header http.Header, method, path string, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.RemoteAddr = remoteAddr
	req.Header.Set("Content-Type", "application/json")
	for name, values := range header {
		req.Header[name] = values
	}
	for _, cookie := range cookies {
		req.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func (h *harness) loginLog(query string, owner *http.Cookie) logPage {
	h.t.Helper()
	rec := h.do(http.MethodGet, "/login-log"+query, nil, owner)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("list login log %q: status = %d; body %s", query, rec.Code, rec.Body)
	}
	return decode[logPage](h.t, rec)
}

// results are the entries' results, newest first.
func results(entries []logEntry) []string {
	var got []string
	for _, e := range entries {
		got = append(got, e.Result)
	}
	return got
}

// users-and-access R36: every successful login writes one entry with the time, the email
// entered, the User, the result, the IP address and the browser or device.
func TestASuccessfulLogInWritesOneEntry(t *testing.T) {
	h := newHarness(t)
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	somchai := h.addUser("Somchai@Example.com")
	header := http.Header{"User-Agent": {"Mozilla/5.0 (iPhone)"}, "X-Vercel-Forwarded-For": {"203.0.113.7"}}
	body := fmt.Sprintf(`{"email":"sOMCHAI@example.com","password":%q}`, password)
	at := h.clock.Now()

	rec := h.from("10.0.0.1:1234", header, http.MethodPost, "/login", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("log in: status = %d; body %s", rec.Code, rec.Body)
	}
	owner := h.session(ownerSessionEmail)

	var got []logEntry
	for _, e := range h.loginLog("", owner).Entries {
		if e.Email != ownerSessionEmail {
			got = append(got, e)
		}
	}
	if len(got) != 1 {
		t.Fatalf("entries for somchai = %+v, want exactly one", got)
	}
	e := got[0]
	if e.Result != "login" || e.Email != "sOMCHAI@example.com" || e.IP != "203.0.113.7" || e.UserAgent != "Mozilla/5.0 (iPhone)" {
		t.Errorf("entry = %+v", e)
	}
	if e.User == nil || e.User.ID != somchai.ID || e.User.Name != "Somchai" {
		t.Errorf("entry user = %+v, want Somchai", e.User)
	}
	if e.At.Before(at) || e.At.After(h.clock.Now()) {
		t.Errorf("entry time = %v, want between %v and %v", e.At, at, h.clock.Now())
	}
}

// users-and-access R36: a failed login is recorded whatever made it fail, with the User
// when the email matches one.
func TestEveryFailedLogInIsRecorded(t *testing.T) {
	h := newHarness(t)
	failures := failedLogIns(h)
	owner, _ := h.ownerAnd("other@example.com")

	for name, attempt := range failures {
		if rec := h.logIn(attempt[0], attempt[1]); rec.Code != http.StatusUnauthorized && rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: status = %d, want a failure", name, rec.Code)
		}
	}

	byEmail := map[string]logEntry{}
	for _, e := range h.loginLog("?result=failed-login", owner).Entries {
		byEmail[e.Email] = e
	}
	// Four causes of 401: "not an email" is a 401 as well, as an email that matches nobody.
	for name, attempt := range failures {
		e, ok := byEmail[attempt[0]]
		if !ok {
			t.Errorf("%s: no failed-login entry for %q", name, attempt[0])
			continue
		}
		wantUser := name == "wrong password" || name == "inactive user" || name == "no password yet"
		if (e.User != nil) != wantUser {
			t.Errorf("%s: user = %+v, want a User: %v", name, e.User, wantUser)
		}
	}
}

// users-and-access R36: a guess at an email nobody has is kept, as typed, with no User.
func TestAFailedLogInWithAnEmailThatMatchesNoUserIsRecordedWithNoUser(t *testing.T) {
	h := newHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")

	h.logIn("Admin@Shop.example", "guess")

	page := h.loginLog("?result=failed-login", owner)
	if len(page.Entries) != 1 {
		t.Fatalf("entries = %+v, want one", page.Entries)
	}
	if e := page.Entries[0]; e.Email != "Admin@Shop.example" || e.User != nil {
		t.Errorf("entry = %+v, want the email as typed and no User", e)
	}
}

// A failure found after the password was right still counts as a failed login
// (users-and-access R16, R31).
func TestALogInRefusedWhileThePasswordWasCheckedIsRecordedAsFailed(t *testing.T) {
	h := newHarness(t)
	owner, somchai := h.ownerAnd("somchai@example.com")
	h.whileVerifying = func() { h.updateUser(somchai.ID, inactive, owner) }

	h.logIn("somchai@example.com", password)

	got := h.loginLog("?userId="+fmt.Sprint(somchai.ID), owner).Entries
	if r := results(got); !slices.Equal(r, []string{"failed-login"}) {
		t.Errorf("results = %v, want [failed-login]", r)
	}
}

// users-and-access R36: a logout is recorded with the User who logged out.
func TestALogOutWritesOneEntry(t *testing.T) {
	h := newHarness(t)
	owner, somchai := h.ownerAnd("somchai@example.com")
	laptop := h.session("somchai@example.com")

	rec := h.from("192.0.2.9:1", http.Header{"User-Agent": {"Firefox"}, "X-Forwarded-For": {"198.51.100.4"}},
		http.MethodPost, "/logout", "", laptop)
	if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Fatalf("log out: status = %d; body %s", rec.Code, rec.Body)
	}

	got := h.loginLog("?result=logout", owner).Entries
	if len(got) != 1 {
		t.Fatalf("logout entries = %+v, want one", got)
	}
	e := got[0]
	if e.Email != "somchai@example.com" || e.IP != "198.51.100.4" || e.UserAgent != "Firefox" || e.User == nil || e.User.ID != somchai.ID {
		t.Errorf("entry = %+v", e)
	}
}

// A request that was no logout, because the session was not live, writes nothing.
func TestALogOutWithoutALiveSessionWritesNoEntry(t *testing.T) {
	h := newHarness(t)
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	h.addUser("somchai@example.com")
	laptop := h.session("somchai@example.com")
	h.clock.Advance(8 * 24 * time.Hour)
	owner := h.session(ownerSessionEmail)

	h.logOut(laptop)
	h.logOut()

	if got := h.loginLog("?result=logout", owner).Entries; len(got) != 0 {
		t.Errorf("logout entries = %+v, want none", got)
	}
}

// users-and-access R37: not the right password, and not the wrong one.
func TestNoPasswordReachesTheLog(t *testing.T) {
	h := newHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")
	const wrong = "hunter2-Wrong-Guess"
	h.logIn("somchai@example.com", wrong)
	h.logIn("nobody@example.com", wrong)
	h.logIn("somchai@example.com", password)
	h.logOut(h.session("somchai@example.com"))

	rec := h.do(http.MethodGet, "/login-log", nil, owner)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
	}
	for _, secret := range []string{wrong, password} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("the log contains the password %q: %s", secret, rec.Body)
		}
	}
}

// users-and-access R38: newest first, and one page at a time.
func TestTheLogComesNewestFirstOnePageAtATime(t *testing.T) {
	h := newHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com") // the Owner has logged in once
	h.logIn("somchai@example.com", "wrong")
	h.logIn("nobody@example.com", "wrong")
	h.logOut(owner)
	owner = h.session(ownerSessionEmail)

	first := h.loginLog("?limit=2", owner)
	if r := results(first.Entries); !slices.Equal(r, []string{"login", "logout"}) {
		t.Fatalf("first page = %v, want [login logout]", r)
	}
	if first.NextBefore == nil {
		t.Fatal("first page has no nextBefore")
	}
	second := h.loginLog(fmt.Sprintf("?limit=2&before=%d", *first.NextBefore), owner)
	if r := results(second.Entries); !slices.Equal(r, []string{"failed-login", "failed-login"}) {
		t.Errorf("second page = %v, want two failed-login", r)
	}
	if got := second.Entries[0].Email; got != "nobody@example.com" {
		t.Errorf("second page starts with %q, want the newer failure", got)
	}
	last := h.loginLog(fmt.Sprintf("?limit=2&before=%d", *second.NextBefore), owner)
	if r := results(last.Entries); !slices.Equal(r, []string{"login"}) {
		t.Errorf("last page = %v, want the Owner's first login", r)
	}
	if last.NextBefore != nil {
		t.Errorf("last page nextBefore = %d, want null", *last.NextBefore)
	}
	all := append(append(first.Entries, second.Entries...), last.Entries...)
	if !slices.IsSortedFunc(all, func(a, b logEntry) int { return int(b.ID - a.ID) }) {
		t.Errorf("entries are not newest first: %+v", all)
	}
}

// A page is exactly full and nothing follows: no next page to chase.
func TestAFullLastPageHasNoNextPage(t *testing.T) {
	h := newHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")
	h.logIn("nobody@example.com", "wrong")

	page := h.loginLog("?limit=2", owner)

	if len(page.Entries) != 2 || page.NextBefore != nil {
		t.Errorf("page = %+v, want two entries and no next page", page)
	}
}

func TestTheLogHoldsAPageOfFiftyByDefault(t *testing.T) {
	h := newHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")
	for range 60 {
		h.logIn("nobody@example.com", "wrong")
	}

	page := h.loginLog("", owner)

	if len(page.Entries) != 50 || page.NextBefore == nil {
		t.Errorf("entries = %d, nextBefore = %v, want 50 and a next page", len(page.Entries), page.NextBefore)
	}
}

// users-and-access R38: filtered by User and by result.
func TestTheLogFiltersByUserAndByResult(t *testing.T) {
	h := newHarness(t)
	owner, somchai := h.ownerAnd("somchai@example.com")
	malee := h.addUser("malee@example.com")
	h.logIn("somchai@example.com", "wrong")
	h.logIn("malee@example.com", "wrong")
	h.logIn("malee@example.com", "wrong")
	h.logIn("nobody@example.com", "wrong")
	h.logOut(h.session("malee@example.com"))

	cases := []struct {
		query string
		want  []string
	}{
		{fmt.Sprintf("?userId=%d", malee.ID), []string{"logout", "login", "failed-login", "failed-login"}},
		{fmt.Sprintf("?userId=%d", somchai.ID), []string{"failed-login"}},
		{"?result=failed-login", []string{"failed-login", "failed-login", "failed-login", "failed-login"}},
		{"?result=logout", []string{"logout"}},
		{fmt.Sprintf("?userId=%d&result=failed-login", malee.ID), []string{"failed-login", "failed-login"}},
		{"?userId=9999", nil},
	}
	for _, c := range cases {
		got := results(h.loginLog(c.query, owner).Entries)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: results = %v, want %v", c.query, got, c.want)
		}
	}
	// Every entry of a User filter belongs to that User.
	for _, e := range h.loginLog(fmt.Sprintf("?userId=%d", malee.ID), owner).Entries {
		if e.User == nil || e.User.ID != malee.ID {
			t.Errorf("entry %+v is not Malee's", e)
		}
	}
}

func TestTheLogRefusesAFilterItDoesNotKnow(t *testing.T) {
	h := newHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")

	for _, query := range []string{"?result=success", "?userId=0", "?limit=0", "?limit=101"} {
		rec := h.do(http.MethodGet, "/login-log"+query, nil, owner)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422", query, rec.Code)
		}
	}
}

// users-and-access R11, R23: only the Owner reads it.
func TestOnlyTheOwnerReadsTheLog(t *testing.T) {
	h := newHarness(t)
	h.addUser(ownerSessionEmail, func(u *store.User) { u.Role = "owner" })
	h.addUser("admin@example.com", func(u *store.User) { u.Role = "admin" })
	h.addUser("staff@example.com")
	admin, staff := h.session("admin@example.com"), h.session("staff@example.com")

	assertProblem(t, h.do(http.MethodGet, "/login-log", nil, &http.Cookie{Name: "session", Value: "gone"}),
		http.StatusUnauthorized, "session-ended")
	if rec := h.do(http.MethodGet, "/login-log", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("no session: status = %d, want 401", rec.Code)
	}
	if rec := h.do(http.MethodGet, "/login-log", nil, admin); rec.Code != http.StatusForbidden {
		t.Errorf("Admin: status = %d, want 403", rec.Code)
	}
	if rec := h.do(http.MethodGet, "/login-log", nil, staff); rec.Code != http.StatusForbidden {
		t.Errorf("Staff: status = %d, want 403", rec.Code)
	}
	if rec := h.do(http.MethodGet, "/login-log", nil, h.session(ownerSessionEmail)); rec.Code != http.StatusOK {
		t.Errorf("Owner: status = %d, want 200", rec.Code)
	}
}

// A refused visitor hears the same whatever they asked for: the access check comes first.
func TestAnAdminIsRefusedBeforeTheirFilterIsLookedAt(t *testing.T) {
	h := newHarness(t)
	h.addUser("admin@example.com", func(u *store.User) { u.Role = "admin" })
	admin := h.session("admin@example.com")

	rec := h.do(http.MethodGet, "/login-log?result=nonsense", nil, admin)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// ADR 0022: the visitor's address, not the proxy's.
func TestTheStoredIPIsTheVisitors(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		header     http.Header
		want       string
	}{
		{"Vercel's own header first", "10.0.0.1:1", http.Header{
			"X-Vercel-Forwarded-For": {"203.0.113.7"}, "X-Forwarded-For": {"198.51.100.1"}}, "203.0.113.7"},
		{"the first of X-Forwarded-For", "10.0.0.1:1", http.Header{"X-Forwarded-For": {"198.51.100.1, 10.9.9.9"}}, "198.51.100.1"},
		{"X-Real-Ip", "10.0.0.1:1", http.Header{"X-Real-Ip": {"198.51.100.2"}}, "198.51.100.2"},
		{"an IPv6 address", "10.0.0.1:1", http.Header{"X-Vercel-Forwarded-For": {"2001:db8::1"}}, "2001:db8::1"},
		{"the connection when no header names one", "192.0.2.50:4711", nil, "192.0.2.50"},
		{"a header that is no address is passed over", "192.0.2.50:4711", http.Header{
			"X-Vercel-Forwarded-For": {"<script>"}, "X-Forwarded-For": {"198.51.100.3"}}, "198.51.100.3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			owner, _ := h.ownerAnd("somchai@example.com")

			h.from(c.remoteAddr, c.header, http.MethodPost, "/login", `{"email":"nobody@example.com","password":"x"}`)

			got := h.loginLog("?result=failed-login", owner).Entries
			if len(got) != 1 || got[0].IP != c.want {
				t.Errorf("entries = %+v, want one with IP %q", got, c.want)
			}
		})
	}
}

// Anyone can attempt a login and entries are kept indefinitely (users-and-access R39), so
// what one entry holds is bounded.
func TestAnEntryHoldsABoundedEmailAndBrowser(t *testing.T) {
	h := newHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")
	long := strings.Repeat("ก", 5000)

	h.from("192.0.2.1:1", http.Header{"User-Agent": {strings.Repeat("a", 5000)}}, http.MethodPost, "/login",
		fmt.Sprintf(`{"email":%q,"password":"x"}`, long))

	got := h.loginLog("?result=failed-login", owner).Entries
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1", len(got))
	}
	if n := len([]rune(got[0].Email)); n != 320 {
		t.Errorf("email has %d characters, want 320", n)
	}
	if n := len(got[0].UserAgent); n != 512 {
		t.Errorf("browser has %d characters, want 512", n)
	}
}

// A login that cannot be recorded is not let through, nor passed off as a wrong password.
func TestALogInIsRefusedWhenItCannotBeRecorded(t *testing.T) {
	h := newHarness(t)
	h.addUser("somchai@example.com")
	h.store.LogErr = fmt.Errorf("disk full")

	rec := h.logIn("somchai@example.com", password)

	assertProblem(t, rec, http.StatusInternalServerError, "")
	if len(rec.Result().Cookies()) != 0 {
		t.Errorf("cookies = %v, want none", rec.Result().Cookies())
	}
}

// users-and-access R39: nothing in the API edits or deletes an entry, or writes one.
func TestTheLogHasOnlyAReadOperation(t *testing.T) {
	for id, op := range operations(t) {
		if strings.Contains(op.Path, "login-log") && (op.Method != http.MethodGet || id != "list-login-log") {
			t.Errorf("%s %s (%s): only list-login-log may reach the log", op.Method, op.Path, id)
		}
	}
}

// users-and-access R36: "the User if it matches one". A client generated from the
// contract must not be told the User is always there, so the response to the most common
// failed-login entry (an email that matches no User) has to satisfy the published schema.
func TestAFailedLogInWithAnUnknownEmailIsAnswerableByThePublishedSchema(t *testing.T) {
	h := newHarness(t)
	owner, _ := h.ownerAnd("somchai@example.com")
	h.logIn("nobody@shop.example", "guess")

	rec := h.do(http.MethodGet, "/login-log?result=failed-login", nil, owner)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
	}
	var body any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), `"user":null`) {
		t.Fatalf("the response has no entry with a null user, so it proves nothing: %s", rec.Body)
	}
	openapi := h.contract.OpenAPI()
	schema := openapi.Paths["/login-log"].Get.Responses["200"].Content["application/json"].Schema
	res := &huma.ValidateResult{}
	huma.Validate(openapi.Components.Schemas, schema, huma.NewPathBuffer([]byte(""), 0), huma.ModeReadFromServer, body, res)
	if len(res.Errors) != 0 {
		t.Errorf("the response does not satisfy the published schema: %v\n%s", res.Errors, rec.Body)
	}
}
