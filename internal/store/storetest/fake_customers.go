package storetest

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/store"
)

// customerPhone is the customers table's CHECK on the phone number (customers R4).
var customerPhone = regexp.MustCompile(`^0[0-9]{8,9}$`)

// AddCustomer seeds a Customer, in any status, and returns it with its ID.
func (f *Fake) AddCustomer(c store.Customer) store.Customer {
	f.mu.Lock()
	defer f.mu.Unlock()
	c.ID = int64(len(f.customers) + 1)
	f.customers = append(f.customers, c)
	return c
}

// SetCustomer replaces the stored Customer with the same ID, as a change made while a request is
// under way.
func (f *Fake) SetCustomer(c store.Customer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.customers[c.ID-1] = c
}

func (f *Fake) customerByEmail(email string) (store.Customer, bool) {
	for _, c := range f.customers {
		if !c.DeletedAt.Valid && strings.EqualFold(c.Email, email) {
			return c, true
		}
	}
	return store.Customer{}, false
}

func (f *Fake) customer(id int64) (store.Customer, bool) {
	if id < 1 || id > int64(len(f.customers)) {
		return store.Customer{}, false
	}
	return f.customers[id-1], true
}

func (f *Fake) GetCustomerByEmail(_ context.Context, email string) (store.Customer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.Customer{}, f.Err
	}
	if c, ok := f.customerByEmail(email); ok {
		return c, nil
	}
	return store.Customer{}, store.ErrNotFound
}

func (f *Fake) IssueSignupCode(_ context.Context, arg store.SignupCodeParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.customerByEmail(arg.Email); ok {
		return store.ErrCustomerEmailTaken
	}
	key := strings.ToLower(arg.Email)
	row := f.signups[key] // a new code keeps the failed attempts and the block (customers R8)
	row.Email, row.Kind, row.TokenHash, row.ExpiresAt = key, store.FirstTimeCode, arg.CodeHash, arg.ExpiresAt
	f.signups[key] = row
	return nil
}

func (f *Fake) VerifySignupCode(_ context.Context, arg store.VerifyFirstTimeCodeParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return false, f.Err
	}
	key := strings.ToLower(arg.Email)
	row, ok := f.signups[key]
	if !ok {
		return false, nil
	}
	next, accepted := store.EmailCredential{
		Kind: row.Kind, TokenHash: row.TokenHash, ExpiresAt: row.ExpiresAt, FailedAttempts: row.FailedAttempts, BlockedUntil: row.BlockedUntil,
	}.VerifyCode(arg)
	row.Kind, row.TokenHash, row.ExpiresAt, row.FailedAttempts, row.BlockedUntil = next.Kind, next.TokenHash, next.ExpiresAt, next.FailedAttempts, next.BlockedUntil
	f.signups[key] = row
	return accepted, nil
}

// signupGrant is the live grant of tokenHash, and the email it belongs to.
func (f *Fake) signupGrant(arg store.SignupGrantParams) (string, bool) {
	for email, row := range f.signups {
		if row.Kind == store.SetupGrant && bytes.Equal(row.TokenHash, arg.TokenHash) && row.ExpiresAt.After(arg.At) {
			return email, true
		}
	}
	return "", false
}

func (f *Fake) ReadSignupGrant(_ context.Context, arg store.SignupGrantParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.signupGrant(arg); !ok {
		return store.ErrNotFound
	}
	return nil
}

func (f *Fake) CompleteSignup(_ context.Context, arg store.CompleteSignupParams, by store.ActivityBy) (store.Customer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.Customer{}, f.Err
	}
	email, ok := f.signupGrant(store.SignupGrantParams{TokenHash: arg.GrantHash, At: arg.At})
	if !ok {
		return store.Customer{}, store.ErrNotFound
	}
	if _, taken := f.customerByEmail(email); taken {
		return store.Customer{}, store.ErrCustomerEmailTaken
	}
	if strings.TrimSpace(arg.Name) == "" || utf8.RuneCountInString(arg.Name) > 120 {
		return store.Customer{}, errors.New("violates customers_name_check")
	}
	if !customerPhone.MatchString(arg.Phone) {
		return store.Customer{}, errors.New("violates customers_phone_check")
	}
	created := store.Customer{
		ID: int64(len(f.customers) + 1), Name: arg.Name, Email: email, Phone: arg.Phone, Status: "active", CreatedAt: arg.At,
		PasswordHash: pgtype.Text{String: arg.PasswordHash, Valid: true},
	}
	entry := store.CustomerCreated(created, by.AsCustomer(created))
	if err := f.checkAll(entry); err != nil {
		return store.Customer{}, err
	}
	f.customers = append(f.customers, created)
	delete(f.signups, email)
	f.record(entry)
	return created, nil
}

func (f *Fake) StartCustomerSession(_ context.Context, arg store.StartCustomerSessionParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return false, f.Err
	}
	arg.Entry.Result = store.ResultFailedLogin
	c, ok := f.customer(arg.Session.CustomerID)
	started := ok && c.Status == "active" && !c.DeletedAt.Valid && c.PasswordHash == arg.Session.PasswordHash
	if started {
		if _, taken := f.customerSessions[string(arg.Session.TokenHash)]; taken {
			return false, errors.New("violates customer_sessions_pkey")
		}
		arg.Entry.Result = store.ResultLogin
	}
	if err := f.addEntry(arg.Entry); err != nil {
		return false, err
	}
	if started {
		f.customerSessions[string(arg.Session.TokenHash)] = store.CustomerSession{
			TokenHash: arg.Session.TokenHash, CustomerID: arg.Session.CustomerID, ExpiresAt: arg.Session.ExpiresAt,
		}
	}
	return started, nil
}

func (f *Fake) GetCustomerSession(_ context.Context, tokenHash []byte) (store.GetCustomerSessionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.GetCustomerSessionRow{}, f.Err
	}
	session, ok := f.customerSessions[string(tokenHash)]
	if !ok {
		return store.GetCustomerSessionRow{}, store.ErrNotFound
	}
	c, _ := f.customer(session.CustomerID)
	return store.GetCustomerSessionRow{
		CustomerID: c.ID, Name: c.Name, Email: c.Email, Phone: c.Phone, ExpiresAt: session.ExpiresAt,
		AddressRecipient: c.AddressRecipient, AddressPhone: c.AddressPhone, AddressLine: c.AddressLine, AddressSubdistrict: c.AddressSubdistrict,
		AddressDistrict: c.AddressDistrict, AddressProvince: c.AddressProvince, AddressPostcode: c.AddressPostcode,
	}, nil
}

func (f *Fake) EndCustomerSession(_ context.Context, arg store.EndCustomerSessionParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.customerSessions[string(arg.TokenHash)]; !ok {
		return nil
	}
	arg.Entry.Result = store.ResultLogout
	if err := f.addEntry(arg.Entry); err != nil {
		return err
	}
	delete(f.customerSessions, string(arg.TokenHash))
	return nil
}
