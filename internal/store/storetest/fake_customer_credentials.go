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

var postcode = regexp.MustCompile(`^[0-9]{5}$`)

// blank and within are the customers table's CHECKs on a text column.
func blank(s string) bool { return strings.TrimSpace(s) == "" }
func within(s string, n int) bool {
	return !blank(s) && utf8.RuneCountInString(s) <= n
}

// checkAddress is the customers_address_check constraint (storefront R7).
func checkAddress(a store.DeliveryAddress) error {
	if !within(a.Recipient, 120) || !customerPhone.MatchString(a.Phone) || !within(a.Line, 200) || !within(a.Subdistrict, 80) ||
		!within(a.District, 80) || blank(a.Province) || !postcode.MatchString(a.Postcode) {
		return errors.New("violates customers_address_check")
	}
	return nil
}

func (f *Fake) customerByID(id int64) (int, bool) {
	if id < 1 || id > int64(len(f.customers)) {
		return 0, false
	}
	return int(id - 1), true
}

func (f *Fake) IssueCustomerCredential(_ context.Context, arg store.EmailCredentialParams) (store.Customer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.Customer{}, f.Err
	}
	c, ok := f.customerByEmail(arg.Email)
	if !ok || !c.CanUseEmailCredential(arg.Kind) {
		return store.Customer{}, store.ErrNotFound
	}
	row := f.customerCredentials[c.ID] // a new one keeps the failed attempts and the block (customers R8)
	row.Kind, row.TokenHash, row.ExpiresAt = arg.Kind, arg.TokenHash, arg.ExpiresAt
	f.customerCredentials[c.ID] = row
	return c, nil
}

func (f *Fake) VerifyCustomerCode(_ context.Context, arg store.VerifyFirstTimeCodeParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return false, f.Err
	}
	c, ok := f.customerByEmail(arg.Email)
	if !ok {
		return false, nil
	}
	row, ok := f.customerCredentials[c.ID]
	if !ok || !c.CanUseEmailCredential(store.FirstTimeCode) {
		return false, nil
	}
	next, accepted := row.VerifyCode(arg)
	f.customerCredentials[c.ID] = next
	return accepted, nil
}

// customerForCredential is the eligible Customer whose live credential is this.
func (f *Fake) customerForCredential(arg store.EmailPasswordParams) (store.Customer, bool) {
	for id, row := range f.customerCredentials {
		i, _ := f.customerByID(id)
		c := f.customers[i]
		if row.Kind == arg.Kind && bytes.Equal(row.TokenHash, arg.TokenHash) && row.ExpiresAt.After(arg.At) && row.AllowsCustomerPassword(c, arg) {
			return c, true
		}
	}
	return store.Customer{}, false
}

func (f *Fake) ReadCustomerCredential(_ context.Context, arg store.EmailPasswordParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.customerForCredential(arg); !ok {
		return store.ErrNotFound
	}
	return nil
}

func (f *Fake) SetCustomerPassword(_ context.Context, arg store.EmailPasswordParams, by store.ActivityBy) (store.Customer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.Customer{}, f.Err
	}
	c, ok := f.customerForCredential(arg)
	if !ok {
		return store.Customer{}, store.ErrNotFound
	}
	c.PasswordHash = pgtype.Text{String: arg.PasswordHash, Valid: true}
	entry := store.CustomerPasswordSet(c, by.AsCustomer(c))
	if arg.Kind == store.ResetLink {
		entry = store.CustomerPasswordReset(c, by.AsCustomer(c))
	}
	if err := f.checkAll(entry); err != nil {
		return store.Customer{}, err
	}
	i, _ := f.customerByID(c.ID)
	f.customers[i] = c
	for hash, session := range f.customerSessions {
		if session.CustomerID == c.ID {
			delete(f.customerSessions, hash)
		}
	}
	delete(f.customerCredentials, c.ID)
	f.record(entry)
	return c, nil
}

func (f *Fake) UpdateProfile(_ context.Context, arg store.ChangeProfileParams, by store.ActivityBy) (store.Customer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return store.Customer{}, f.Err
	}
	i, ok := f.customerByID(arg.ID)
	if !ok || f.customers[i].DeletedAt.Valid {
		return store.Customer{}, store.ErrNotFound
	}
	before := f.customers[i]
	after := before
	if arg.Name != nil {
		after.Name = *arg.Name
	}
	if arg.Phone != nil {
		after.Phone = *arg.Phone
	}
	if !within(after.Name, 120) {
		return store.Customer{}, errors.New("violates customers_name_check")
	}
	if !customerPhone.MatchString(after.Phone) {
		return store.Customer{}, errors.New("violates customers_phone_check")
	}
	if a := arg.Address; a != nil {
		if err := checkAddress(*a); err != nil {
			return store.Customer{}, err
		}
		text := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }
		after.AddressRecipient, after.AddressPhone, after.AddressLine = text(a.Recipient), text(a.Phone), text(a.Line)
		after.AddressSubdistrict, after.AddressDistrict, after.AddressProvince, after.AddressPostcode = text(a.Subdistrict), text(a.District), text(a.Province), text(a.Postcode)
	}
	entries := store.CustomerEdited(before, after, by.AsCustomer(after))
	if err := f.checkAll(entries...); err != nil {
		return store.Customer{}, err
	}
	f.customers[i] = after
	f.record(entries...)
	return after, nil
}
