// Package storetest holds the contract suite of the store ports and an in-memory fake
// that passes it. Imported by tests only.
package storetest

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// Fake is an in-memory Store. The mutex only keeps it free of data races: what locking
// and transactions do for real is shown by the slow tests, not pretended here.
type Fake struct {
	// Err, when set, is returned by every operation, like a database that is down.
	Err error
	// LogErr, when set, is returned by every operation that writes a Login log entry, like
	// a database that can read but not write.
	LogErr error
	// ActivityErr makes writing an Activity log entry fail, and the change with it.
	ActivityErr error

	mu            sync.Mutex
	users         []store.User
	sessions      map[string]store.Session
	links         map[int64]store.SetupLink
	credentials   map[int64]store.EmailCredential
	emailRequests []store.EmailRequestParams
	loginChecks   []store.LoginCheckParams
	log           []store.ListLoginLogRow
	products      []store.Product
	activity      []store.ActivityLog
	movements     []store.StockMovement
	orders        []store.Order
	lines         []store.OrderLine
	// numbered is the last Order number handed out, as the order_numbers sequence keeps it.
	numbered int64
}

var _ Store = (*Fake)(nil)

// NewFake returns an empty Fake.
func NewFake() *Fake {
	return &Fake{sessions: map[string]store.Session{}, links: map[int64]store.SetupLink{}, credentials: map[int64]store.EmailCredential{}}
}

// AddUser seeds a User and returns it with its ID.
func (f *Fake) AddUser(user store.User) store.User {
	f.mu.Lock()
	defer f.mu.Unlock()
	user.ID = int64(len(f.users) + 1)
	f.users = append(f.users, user)
	return user
}

// SetupLinks returns what is stored, as anyone reading the database would see it.
func (f *Fake) SetupLinks() []store.SetupLink {
	f.mu.Lock()
	defer f.mu.Unlock()
	links := make([]store.SetupLink, 0, len(f.links))
	for _, link := range f.links {
		links = append(links, link)
	}
	return links
}

func (f *Fake) Ping(context.Context) error { return f.Err }

func (f *Fake) GetUserByEmail(_ context.Context, email string) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.User{}, f.Err
	}
	for _, user := range f.users {
		if !user.DeletedAt.Valid && strings.EqualFold(user.Email, email) {
			return user, nil
		}
	}
	return store.User{}, store.ErrNotFound
}

func (f *Fake) GetOwner(context.Context) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.User{}, f.Err
	}
	for _, user := range f.users {
		if user.Role == "owner" {
			return user, nil
		}
	}
	return store.User{}, store.ErrNotFound
}

func (f *Fake) CreateOwner(_ context.Context, arg store.CreateOwnerParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return 0, f.Err
	}
	if strings.TrimSpace(arg.Name) == "" {
		return 0, errors.New("violates users_name_check")
	}
	for _, user := range f.users {
		if user.Role == "owner" || (!user.DeletedAt.Valid && strings.EqualFold(user.Email, arg.Email)) {
			return 0, nil
		}
	}
	f.users = append(f.users, store.User{
		ID: int64(len(f.users) + 1), Name: arg.Name, Email: arg.Email, Role: "owner", Status: "active",
	})
	return 1, nil
}

// RecordActivity is store.Queries' RecordActivity: an entry written on the caller's transaction.
func (f *Fake) RecordActivity(_ context.Context, entry store.ActivityEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.checkActivity(entry); err != nil {
		return err
	}
	f.record(entry)
	return nil
}

// snapshot is a User as an Activity log entry names them.
func snapshot(u store.User) store.UserSnapshot {
	return store.UserSnapshot{ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role, Status: u.Status}
}

// checkAll refuses the change when any of its entries would be refused, before anything is
// kept, as the entry's failure rolls the change back (activity-log R3).
func (f *Fake) checkAll(entries ...store.ActivityEntry) error {
	for _, e := range entries {
		if err := f.checkActivity(e); err != nil {
			return err
		}
	}
	return nil
}

func (f *Fake) CreateUser(_ context.Context, arg store.CreateUserParams, by store.ActivityBy) (store.CreateUserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.CreateUserRow{}, f.Err
	}
	// Postgres checks the CHECK constraints before it looks for a conflicting email.
	switch {
	case strings.TrimSpace(arg.Name) == "":
		return store.CreateUserRow{}, errors.New("violates users_name_check")
	case arg.Role != "owner" && arg.Role != "admin" && arg.Role != "staff":
		return store.CreateUserRow{}, errors.New("violates users_role_check")
	}
	owned := false
	for _, user := range f.users {
		if !user.DeletedAt.Valid && strings.EqualFold(user.Email, arg.Email) {
			return store.CreateUserRow{}, store.ErrNotFound
		}
		owned = owned || user.Role == "owner"
	}
	if arg.Role == "owner" && owned {
		return store.CreateUserRow{}, errors.New("violates users_one_owner_key")
	}
	user := store.User{ID: int64(len(f.users) + 1), Name: arg.Name, Email: arg.Email, Role: arg.Role, Status: "active"}
	entries := []store.ActivityEntry{store.UserCreated(snapshot(user), by), store.UserSetupLinkIssued(snapshot(user), by)}
	if err := f.checkAll(entries...); err != nil {
		return store.CreateUserRow{}, err
	}
	f.users = append(f.users, user)
	f.links[user.ID] = store.SetupLink{UserID: user.ID, TokenHash: arg.TokenHash, ExpiresAt: arg.ExpiresAt}
	f.record(entries...)
	return store.CreateUserRow{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role, Status: user.Status}, nil
}

func (f *Fake) ListUsers(context.Context) ([]store.ListUsersRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	rows := make([]store.ListUsersRow, 0, len(f.users))
	for _, user := range f.users {
		if user.DeletedAt.Valid {
			continue
		}
		rows = append(rows, store.ListUsersRow{
			ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role, Status: user.Status,
			PasswordSet: user.PasswordHash.Valid,
		})
	}
	return rows, nil
}

func (f *Fake) UpdateUser(_ context.Context, arg store.ChangeUserParams, by store.ActivityBy) (store.ChangeUserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.ChangeUserRow{}, f.Err
	}
	target := f.userRef(arg.ID)
	if target == nil {
		return store.ChangeUserRow{}, store.ErrNotFound
	}
	if target.Role == "owner" {
		return store.ChangeUserRow{}, store.ErrTargetIsOwner
	}
	changed := *target
	if arg.Name != "" {
		changed.Name = arg.Name
	}
	if arg.Role != "" {
		changed.Role = arg.Role
	}
	if arg.Status != "" {
		changed.Status = arg.Status
	}
	switch {
	case strings.TrimSpace(changed.Name) == "":
		return store.ChangeUserRow{}, errors.New("violates users_name_check")
	case changed.Role != "admin" && changed.Role != "staff" && changed.Role != "owner":
		return store.ChangeUserRow{}, errors.New("violates users_role_check")
	case changed.Status != "active" && changed.Status != "inactive":
		return store.ChangeUserRow{}, errors.New("violates users_status_check")
	case changed.Role == "owner" && slices.ContainsFunc(f.users, func(u store.User) bool { return u.Role == "owner" }):
		return store.ChangeUserRow{}, errors.New("violates users_one_owner_key")
	}
	entries := store.UserEdited(snapshot(*target), snapshot(changed), by)
	if err := f.checkAll(entries...); err != nil {
		return store.ChangeUserRow{}, err
	}
	*target = changed
	f.record(entries...)
	if changed.Status == "inactive" {
		f.endSessions(changed.ID)
		delete(f.links, changed.ID)
		delete(f.credentials, changed.ID)
	}
	return store.ChangeUserRow{
		ID: changed.ID, Name: changed.Name, Email: changed.Email, Role: changed.Role, Status: changed.Status,
		PasswordSet: changed.PasswordHash.Valid,
	}, nil
}

func (f *Fake) ResetPassword(_ context.Context, arg store.IssueSetupLinkParams, by store.ActivityBy) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	target := f.userRef(arg.UserID)
	switch {
	case target == nil:
		return store.ErrNotFound
	case target.Role == "owner":
		return store.ErrTargetIsOwner
	case target.Status == "inactive":
		return store.ErrUserInactive
	}
	entries := []store.ActivityEntry{store.UserPasswordReset(snapshot(*target), by), store.UserSetupLinkIssued(snapshot(*target), by)}
	if err := f.checkAll(entries...); err != nil {
		return err
	}
	if err := f.issue(arg); err != nil {
		return err
	}
	target.PasswordHash = pgtype.Text{}
	delete(f.credentials, arg.UserID)
	f.endSessions(arg.UserID)
	f.record(entries...)
	return nil
}

func (f *Fake) CreateSession(_ context.Context, arg store.CreateSessionParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return 0, f.Err
	}
	user, ok := f.user(arg.UserID)
	if !ok || user.Status != "active" || user.PasswordHash != arg.PasswordHash {
		return 0, nil
	}
	if _, ok := f.sessions[string(arg.TokenHash)]; ok {
		return 0, errors.New("violates sessions_pkey")
	}
	f.sessions[string(arg.TokenHash)] = store.Session{TokenHash: arg.TokenHash, UserID: arg.UserID, ExpiresAt: arg.ExpiresAt}
	return 1, nil
}

// StartSession keeps the entry and the session together: with a refused session the entry
// is still written, as a failed login. Like the real one it is one transaction, so an entry
// that Postgres would refuse (a User that does not exist, an unknown result) starts no
// session either.
func (f *Fake) StartSession(_ context.Context, arg store.StartSessionParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return false, f.Err
	}
	arg.Entry.Result = store.ResultFailedLogin
	user, ok := f.user(arg.Session.UserID)
	started := ok && user.Status == "active" && user.PasswordHash == arg.Session.PasswordHash
	if started {
		if _, taken := f.sessions[string(arg.Session.TokenHash)]; taken {
			return false, errors.New("violates sessions_pkey")
		}
		arg.Entry.Result = store.ResultLogin
	}
	if err := f.addEntry(arg.Entry); err != nil {
		return false, err
	}
	if started {
		f.sessions[string(arg.Session.TokenHash)] = store.Session{
			TokenHash: arg.Session.TokenHash, UserID: arg.Session.UserID, ExpiresAt: arg.Session.ExpiresAt,
		}
	}
	return started, nil
}

func (f *Fake) EndSession(_ context.Context, arg store.EndSessionParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.sessions[string(arg.TokenHash)]; !ok {
		return nil
	}
	arg.Entry.Result = store.ResultLogout
	if err := f.addEntry(arg.Entry); err != nil {
		return err
	}
	delete(f.sessions, string(arg.TokenHash))
	return nil
}

func (f *Fake) AddLoginLogEntry(_ context.Context, arg store.AddLoginLogEntryParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	return f.addEntry(arg)
}

func (f *Fake) addEntry(arg store.AddLoginLogEntryParams) error {
	if f.LogErr != nil {
		return f.LogErr
	}
	if arg.Result != store.ResultLogin && arg.Result != store.ResultFailedLogin && arg.Result != store.ResultLogout && arg.Result != store.ResultBlocked {
		return errors.New("violates login_log_result_check")
	}
	app := store.AppStockFlow // the column's default
	if arg.App.Valid {
		app = arg.App.String
	}
	if app != store.AppStockFlow && app != store.AppStorefront {
		return errors.New("violates login_log_app_check")
	}
	if arg.CustomerID.Valid && (app != store.AppStorefront || arg.UserID.Valid) {
		return errors.New("violates login_log_customer_check")
	}
	entry := store.ListLoginLogRow{
		ID: int64(len(f.log) + 1), At: arg.At, Email: arg.Email, UserID: arg.UserID,
		Result: arg.Result, Ip: arg.Ip, UserAgent: arg.UserAgent, App: app, CustomerID: arg.CustomerID,
	}
	if arg.UserID.Valid {
		user, ok := f.user(arg.UserID.Int64)
		if !ok {
			return errors.New("violates login_log_user_id_fkey")
		}
		entry.UserName = pgtype.Text{String: user.Name, Valid: true}
	}
	f.log = append(f.log, entry)
	return nil
}

// LoginBlockedUntil feeds the newest failures to the same rule the database uses.
func (f *Fake) LoginBlockedUntil(_ context.Context, arg store.LoginBlockParams) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return time.Time{}, f.Err
	}
	var byEmail, byIP []time.Time
	for _, row := range f.log {
		if row.Result != store.ResultFailedLogin || row.App != arg.App {
			continue
		}
		if strings.EqualFold(row.Email, arg.Email) {
			byEmail = append(byEmail, row.At)
		}
		if row.Ip == arg.IP {
			byIP = append(byIP, row.At)
		}
	}
	newestFirst := func(times []time.Time) []time.Time {
		slices.SortStableFunc(times, func(a, b time.Time) int { return b.Compare(a) })
		return times[:min(len(times), store.IPFailureLimit)]
	}
	byEmail, byIP = newestFirst(byEmail), newestFirst(byIP)
	return store.BlockedUntil(byEmail[:min(len(byEmail), store.EmailFailureLimit)], byIP, arg.At), nil
}

// navigation-and-lists R6: the fake applies the same filters before counting or paging.
func matchesLoginLog(row store.ListLoginLogRow, arg store.CountLoginLogParams) bool {
	return (!arg.UserID.Valid || row.UserID == arg.UserID) && (!arg.Result.Valid || row.Result == arg.Result.String) &&
		(!arg.App.Valid || row.App == arg.App.String)
}

func (f *Fake) CountLoginLog(_ context.Context, arg store.CountLoginLogParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return 0, f.Err
	}
	var total int64
	for _, row := range f.log {
		if matchesLoginLog(row, arg) {
			total++
		}
	}
	return total, nil
}

func (f *Fake) ListLoginLog(_ context.Context, arg store.ListLoginLogParams) ([]store.ListLoginLogRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	var rows []store.ListLoginLogRow
	var skipped int32
	for i := len(f.log) - 1; i >= 0 && len(rows) < int(arg.RowLimit); i-- {
		row := f.log[i]
		if !matchesLoginLog(row, store.CountLoginLogParams{UserID: arg.UserID, Result: arg.Result, App: arg.App}) || (arg.Before.Valid && row.ID >= arg.Before.Int64) {
			continue
		}
		if skipped < arg.RowOffset {
			skipped++
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (f *Fake) GetSession(_ context.Context, tokenHash []byte) (store.GetSessionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.GetSessionRow{}, f.Err
	}
	session, ok := f.sessions[string(tokenHash)]
	if !ok {
		return store.GetSessionRow{}, store.ErrNotFound
	}
	user, _ := f.user(session.UserID)
	return store.GetSessionRow{UserID: session.UserID, Name: user.Name, Email: user.Email, Role: user.Role, ExpiresAt: session.ExpiresAt}, nil
}

func (f *Fake) IssueSetupLink(_ context.Context, arg store.IssueSetupLinkParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.user(arg.UserID); !ok {
		return errors.New("violates setup_links_user_id_fkey")
	}
	return f.issue(arg)
}

func (f *Fake) issue(arg store.IssueSetupLinkParams) error {
	for userID, link := range f.links {
		if userID != arg.UserID && string(link.TokenHash) == string(arg.TokenHash) {
			return errors.New("violates setup_links_token_hash_key")
		}
	}
	f.links[arg.UserID] = store.SetupLink(arg)
	return nil
}

func (f *Fake) GetSetupLinkUser(_ context.Context, arg store.GetSetupLinkUserParams) (store.GetSetupLinkUserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.GetSetupLinkUserRow{}, f.Err
	}
	for _, link := range f.links {
		if string(link.TokenHash) == string(arg.TokenHash) && link.ExpiresAt.After(arg.At) {
			user, _ := f.user(link.UserID)
			return store.GetSetupLinkUserRow{Name: user.Name, Email: user.Email}, nil
		}
	}
	return store.GetSetupLinkUserRow{}, store.ErrNotFound
}

func (f *Fake) SetPassword(_ context.Context, arg store.SetPasswordParams, by store.ActivityBy) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return 0, f.Err
	}
	for userID, link := range f.links {
		if string(link.TokenHash) == string(arg.TokenHash) && link.ExpiresAt.After(arg.At) {
			for i := range f.users {
				if f.users[i].ID == userID {
					who := snapshot(f.users[i])
					entry := store.UserPasswordSet(who, by.AsUser(who))
					if err := f.checkActivity(entry); err != nil {
						return 0, err
					}
					f.record(entry)
				}
			}
			delete(f.links, userID)
			delete(f.credentials, userID)
			for i := range f.users {
				if f.users[i].ID != userID {
					continue
				}
				f.users[i].PasswordHash = pgtype.Text{String: arg.PasswordHash, Valid: true}
				if f.users[i].Role == "owner" {
					f.endSessions(userID) // users-and-access R34, context ADR 0003.
				}
			}
			return 1, nil
		}
	}
	return 0, nil
}

func (f *Fake) endSessions(userID int64) {
	maps.DeleteFunc(f.sessions, func(_ string, session store.Session) bool { return session.UserID == userID })
}

func (f *Fake) user(id int64) (store.User, bool) {
	if user := f.userRef(id); user != nil {
		return *user, true
	}
	return store.User{}, false
}

// userRef is the User of id, unless Deleted: what a lookup by id sees.
func (f *Fake) userRef(id int64) *store.User {
	if u := f.userRow(id); u != nil && !u.DeletedAt.Valid {
		return u
	}
	return nil
}

// userRow is the User of id, Deleted or not: what a join sees, so that an Order still names
// the User who made it.
func (f *Fake) userRow(id int64) *store.User {
	for i := range f.users {
		if f.users[i].ID == id {
			return &f.users[i]
		}
	}
	return nil
}
