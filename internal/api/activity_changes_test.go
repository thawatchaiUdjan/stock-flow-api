package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/auth"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/email"
	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// entriesOf is the activity log's entries of one kind of record, oldest first.
func (h *harness) entriesOf(kind, query string, owner *http.Cookie) []activityEntry {
	h.t.Helper()
	rec := h.do(http.MethodGet, "/v2/activity-log?kind="+kind+"&limit=50"+query, nil, owner)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("activity log: status = %d; body %s", rec.Code, rec.Body)
	}
	entries := decode[activityPage](h.t, rec).Entries
	slices.Reverse(entries)
	return entries
}

func actions(entries []activityEntry) []string {
	var got []string
	for _, e := range entries {
		got = append(got, e.Action)
	}
	return got
}

// activity-log R2-R4, R6: each User change is an entry, made by whoever made it, and none
// holds a password, a code or a link.
func TestEveryUserChangeIsAnActivityLogEntryWithoutSecrets(t *testing.T) {
	h, owner := activityHarness(t)

	created := decode[createdUser](t, h.createUser(newUser("Somchai", "somchai@example.com", "staff"), owner))
	id := created.User.ID
	for _, change := range []map[string]string{{"name": "Somchai P.", "role": "admin"}, {"name": "Somchai P."}, {"status": "inactive"}, {"status": "active"}} {
		if rec := h.updateUser(id, change, owner); rec.Code != http.StatusOK {
			t.Fatalf("update %v: status = %d; body %s", change, rec.Code, rec.Body)
		}
	}
	reset := decode[setupLinkBody](t, h.resetPassword(id, owner))
	if rec := h.setPassword(reset.Token, "a brand new password"); rec.Code != http.StatusNoContent {
		t.Fatalf("set password: status = %d; body %s", rec.Code, rec.Body)
	}
	for _, rec := range []*httptest.ResponseRecorder{h.updateUser(id, map[string]string{"status": "inactive"}, owner), h.deleteUser(id, owner)} {
		if rec.Code >= 300 {
			t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
		}
	}

	raw := h.do(http.MethodGet, "/v2/activity-log?kind=user&limit=50", nil, owner).Body.String()
	entries := h.entriesOf("user", "&actorKind=user", owner)
	var subject []activityEntry
	for _, e := range entries {
		if e.Subject.ID == id {
			subject = append(subject, e)
		}
	}

	want := []string{"created", "setup-link-issued", "name-changed", "role-changed", "made-inactive", "made-active", "password-reset", "setup-link-issued", "password-set", "made-inactive", "deleted"}
	if got := actions(subject); !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	if c := subject[0].Changes; len(c) != 1 || c[0].Field != "role" || c[0].Before != nil || c[0].After != "staff" {
		t.Errorf("created changes = %+v", c)
	}
	if c := subject[2].Changes; len(c) != 1 || c[0].Before != "Somchai" || c[0].After != "Somchai P." {
		t.Errorf("name-changed changes = %+v", c)
	}
	if e := subject[0]; e.Actor.Name != "Malee" || e.Actor.Role != "owner" || e.Subject.Ref != "somchai@example.com" || e.Subject.Name != "Somchai" {
		t.Errorf("created = %+v, want the Owner as actor and the User's email as the subject's", e)
	}
	// The password the User set is theirs: actor and subject are the same person, as then.
	if e := subject[8]; e.Actor.ID == nil || *e.Actor.ID != id || e.Actor.Name != "Somchai P." || e.Actor.Role != "admin" {
		t.Errorf("password-set = %+v, want the User as actor", e)
	}
	for _, secret := range []string{created.SetupLink.Token, reset.Token, "a brand new password", "$argon2"} {
		if strings.Contains(raw, secret) {
			t.Errorf("the Activity log holds a secret: %q", secret)
		}
	}
}

// activity-log R4: a first password by an email code, and a reset by a Reset link.
func TestEmailPasswordSettingAndResetAreActivityLogEntries(t *testing.T) {
	h, m := newEmailHarness(t)
	h.addUser("owner@example.com", func(u *store.User) { u.Name = "Malee"; u.Role = "owner" })
	owner := h.session("owner@example.com")
	h.addUser("new@example.com", func(u *store.User) { u.PasswordHash.Valid = false; u.PasswordHash.String = "" })
	h.requestCode("new@example.com")
	grant := decode[verificationBody](t, h.verifyCode("new@example.com", m.messages[0].Code))
	if rec := h.do(http.MethodPost, "/v2/password-setup/password", map[string]string{"token": grant.Token, "password": "first password"}); rec.Code != http.StatusNoContent {
		t.Fatalf("set: status = %d; body %s", rec.Code, rec.Body)
	}
	h.clock.Advance(2 * time.Minute) // the same email may be sent to once a minute (users-and-access R43)
	h.requestReset("new@example.com")
	var link email.Message
	for _, msg := range m.messages {
		if msg.Kind == email.ResetLink {
			link = msg
		}
	}
	if rec := h.saveReset(resetToken(t, link), "second password"); rec.Code != http.StatusNoContent {
		t.Fatalf("reset: status = %d; body %s", rec.Code, rec.Body)
	}

	entries := h.entriesOf("user", "", owner)

	var got []string
	for _, e := range entries {
		if e.Subject.Ref == "new@example.com" {
			got = append(got, e.Action)
			if e.Actor.Email != "new@example.com" || e.Actor.Kind != "user" {
				t.Errorf("%s: actor = %+v, want the User", e.Action, e.Actor)
			}
		}
	}
	if fmt.Sprint(got) != "[password-set password-reset]" {
		t.Errorf("entries = %v, want [password-set password-reset]", got)
	}
}

// activity-log R4, R11: each Order change is an entry, and a Deleted Order is still named.
func TestEveryOrderChangeIsAnActivityLogEntry(t *testing.T) {
	h, owner := activityHarness(t)
	h.addUser("staff@example.com")
	staff := h.session("staff@example.com")
	tea := h.store.AddProduct(store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "box", Price: 4500, Stock: 20, Status: "active"})
	place := func(customer string) int64 {
		rec := h.do(http.MethodPost, "/orders", map[string]any{"customer": customer, "lines": []map[string]any{{"productId": tea.ID, "quantity": 1}}}, staff)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create order: status = %d; body %s", rec.Code, rec.Body)
		}
		return decode[struct {
			Number int64 `json:"number"`
		}](t, rec).Number
	}
	first, second, third := place("Somying"), place(""), place("Somsak")
	for _, req := range [][2]string{
		{http.MethodPost, fmt.Sprintf("/orders/%d/complete", first)},
		{http.MethodPost, fmt.Sprintf("/orders/%d/cancel", second)},
		{http.MethodDelete, fmt.Sprintf("/orders/%d", first)},
	} {
		cookie := owner
		if strings.HasSuffix(req[1], "complete") {
			cookie = staff
		}
		if rec := h.do(req[0], req[1], nil, cookie); rec.Code >= 300 {
			t.Fatalf("%s %s: status = %d; body %s", req[0], req[1], rec.Code, rec.Body)
		}
	}
	// The system cancels the third, as the Unpaid timeout of a later ticket does.
	system := store.ActivityBy{At: h.clock.Now(), Actor: store.ActivityActor{Kind: store.ActorSystem, Name: "System"}}
	if _, err := h.store.CancelOrder(context.Background(), store.FinishOrderParams{Number: third, Now: h.clock.Now}, system); err != nil {
		t.Fatal(err)
	}

	entries := h.entriesOf("order", "", owner)

	want := []string{"created", "created", "created", "completed", "cancelled", "deleted", "cancelled"}
	if got := actions(entries); !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	if e := entries[0]; e.Subject.Kind != "order" || e.Subject.Ref != fmt.Sprint(first) || e.Subject.Name != "Somying" || e.Actor.Name != "Somchai" {
		t.Errorf("created = %+v", e)
	}
	if e := entries[4]; e.Actor.Name != "Malee" || e.Subject.Ref != fmt.Sprint(second) {
		t.Errorf("cancelled by the Owner = %+v", e)
	}
	if e := entries[5]; e.Subject.Ref != fmt.Sprint(first) || e.Subject.Name != "Somying" {
		t.Errorf("deleted = %+v, want the Order still named", e)
	}
	if e := entries[6]; e.Actor.Kind != "system" || e.Actor.ID != nil || e.Subject.Ref != fmt.Sprint(third) {
		t.Errorf("cancelled by the system = %+v", e)
	}
}

// activity-log R3, R7: a refused change writes nothing, and a change is not saved without its entry.
func TestARefusedOrUnrecordedUserOrOrderChangeWritesNothing(t *testing.T) {
	h, owner := activityHarness(t)
	h.addUser("staff@example.com")
	staff := h.session("staff@example.com")
	tea := h.store.AddProduct(store.Product{Name: "Thai tea", Sku: "TEA-01", Unit: "box", Price: 4500, Stock: 20, Status: "active"})
	rec := h.do(http.MethodPost, "/orders", map[string]any{"lines": []map[string]any{{"productId": tea.ID, "quantity": 1}}}, staff)
	number := decode[struct {
		Number int64 `json:"number"`
	}](t, rec).Number
	h.do(http.MethodPost, fmt.Sprintf("/orders/%d/complete", number), nil, staff)
	before := h.activityLog("", owner).Total

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"a taken email":         h.createUser(newUser("Dup", "staff@example.com", "staff"), owner),
		"complete again":        h.do(http.MethodPost, fmt.Sprintf("/orders/%d/complete", number), nil, staff),
		"cancel a completed":    h.do(http.MethodPost, fmt.Sprintf("/orders/%d/cancel", number), nil, owner),
		"delete an Active user": h.deleteUser(2, owner),
		"no such order":         h.do(http.MethodPost, "/orders/99/complete", nil, staff),
		"update the Owner":      h.updateUser(1, map[string]string{"name": "x"}, owner),
	} {
		if rec.Code < 400 {
			t.Errorf("%s: status = %d, want a refusal", name, rec.Code)
		}
	}
	if after := h.activityLog("", owner).Total; after != before {
		t.Errorf("entries = %d, want %d: a refused change writes none", after, before)
	}

	h.store.ActivityErr = fmt.Errorf("activity_log is full")
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"create order": h.do(http.MethodPost, "/orders", map[string]any{"lines": []map[string]any{{"productId": tea.ID, "quantity": 1}}}, staff),
		"update user":  h.updateUser(2, map[string]string{"name": "Changed"}, owner),
		"delete order": h.deleteOrder(number, owner),
	} {
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500; body %s", name, rec.Code, rec.Body)
		}
	}
	h.store.ActivityErr = nil
	if got := decode[struct {
		Stock int32 `json:"stock"`
	}](t, h.do(http.MethodGet, fmt.Sprintf("/products/%d", tea.ID), nil, owner)); got.Stock != 19 {
		t.Errorf("stock = %d, want 19: the unrecorded order took none", got.Stock)
	}
	if rec := h.do(http.MethodGet, fmt.Sprintf("/orders/%d", number), nil, owner); rec.Code != http.StatusOK {
		t.Errorf("the order: status = %d, want it not Deleted", rec.Code)
	}
	if users := h.do(http.MethodGet, "/users", nil, owner).Body.String(); strings.Contains(users, "Changed") {
		t.Errorf("the User changed although its entry failed: %s", users)
	}
}

// activity-log R2, R4: the installation is the actor of the Owner and their Setup link.
func TestTheInstallationIsTheActorOfTheOwnerAndTheirSetupLink(t *testing.T) {
	h := newHarness(t)
	link, err := auth.IssueOwnerSetupLink(context.Background(), h.store, h.clock.Now(), "Malee", ownerEmail)
	if err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Minute)
	if rec := h.setPassword(link.Token, password); rec.Code != http.StatusNoContent {
		t.Fatalf("set password: status = %d; body %s", rec.Code, rec.Body)
	}
	owner := h.session(ownerEmail)

	installed := h.entriesOf("user", "&actorKind=installation", owner)
	all := h.entriesOf("user", "", owner)

	if got := actions(installed); fmt.Sprint(got) != "[created setup-link-issued]" {
		t.Fatalf("installation entries = %v, want [created setup-link-issued]", got)
	}
	for _, e := range installed {
		if e.Actor.Kind != "installation" || e.Actor.ID != nil || e.Subject.Ref != ownerEmail || e.Subject.Name != "Malee" {
			t.Errorf("%s = %+v", e.Action, e)
		}
	}
	if last := all[len(all)-1]; last.Action != "password-set" || last.Actor.Kind != "user" || last.Actor.Name != "Malee" {
		t.Errorf("last = %+v, want the Owner's password set by the Owner", last)
	}
	if secret := h.do(http.MethodGet, "/v2/activity-log?limit=50", nil, owner).Body.String(); strings.Contains(secret, link.Token) {
		t.Error("the Activity log holds the Setup link's token")
	}
}

// activity-log R10: the filters cover the new kinds and actions.
func TestTheActivityLogFiltersUserAndOrderEntries(t *testing.T) {
	h, owner := activityHarness(t)
	created := decode[createdUser](t, h.createUser(newUser("Somchai", "somchai@example.com", "staff"), owner))
	h.updateUser(created.User.ID, map[string]string{"status": "inactive"}, owner)
	h.addProduct(owner, "Thai tea", "TEA-01")

	for query, want := range map[string]int64{
		"?kind=user":                      3,
		"?kind=user&action=made-inactive": 1,
		"?kind=user&action=created":       1,
		"?action=created":                 1,
		"?kind=order":                     0,
		"?kind=product":                   1,
		"?actorKind=user":                 4,
		"?actorKind=installation":         0,
	} {
		if got := h.activityLog(query, owner).Total; got != want {
			t.Errorf("%s: total = %d, want %d", query, got, want)
		}
	}
}
