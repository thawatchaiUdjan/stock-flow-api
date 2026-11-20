package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// DeliveryAddress is a Customer's saved Delivery address (storefront R7, customers R2).
type DeliveryAddress struct {
	Recipient   string `json:"recipient"`
	Phone       string `json:"phone"`
	Line        string `json:"line"`
	Subdistrict string `json:"subdistrict"`
	District    string `json:"district"`
	Province    string `json:"province"`
	Postcode    string `json:"postcode"`
}

// DeliveryAddress is the Customer's saved address, or nil when they have none.
func (c Customer) DeliveryAddress() *DeliveryAddress {
	if !c.AddressRecipient.Valid {
		return nil
	}
	return &DeliveryAddress{
		Recipient: c.AddressRecipient.String, Phone: c.AddressPhone.String, Line: c.AddressLine.String,
		Subdistrict: c.AddressSubdistrict.String, District: c.AddressDistrict.String,
		Province: c.AddressProvince.String, Postcode: c.AddressPostcode.String,
	}
}

// Customer is the Customer a session belongs to, as far as a session row knows them.
func (r GetCustomerSessionRow) Customer() Customer {
	return Customer{
		ID: r.CustomerID, Name: r.Name, Email: r.Email, Phone: r.Phone,
		AddressRecipient: r.AddressRecipient, AddressPhone: r.AddressPhone, AddressLine: r.AddressLine,
		AddressSubdistrict: r.AddressSubdistrict, AddressDistrict: r.AddressDistrict,
		AddressProvince: r.AddressProvince, AddressPostcode: r.AddressPostcode,
	}
}

func holdCustomer(ctx context.Context, q *Queries, id int64) (Customer, error) {
	c, err := q.LockCustomer(ctx, id)
	if err != nil {
		return Customer{}, fmt.Errorf("lock Customer: %w", err)
	}
	return c, nil
}

// IssueCustomerCredential stores a Verification code or Reset link for the Customer with this
// email, replacing their earlier one (customers R8, R12). Eligibility is checked under the
// Customer's lock, so a concurrent deactivation or password change leaves no usable
// instructions (customers R15). It answers ErrNotFound for no such Customer or one who may
// not have it: an Inactive one, a Reset link for one without a password, a code for one with.
func (s *Store) IssueCustomerCredential(ctx context.Context, arg EmailCredentialParams) (issued Customer, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		c, err := q.GetCustomerByEmail(ctx, arg.Email)
		if err != nil {
			return fmt.Errorf("find Customer: %w", err)
		}
		if c, err = holdCustomer(ctx, q, c.ID); err != nil {
			return err
		}
		if !c.CanUseEmailCredential(arg.Kind) {
			return ErrNotFound
		}
		if err := q.IssueCustomerCredential(ctx, IssueCustomerCredentialParams{
			CustomerID: c.ID, Kind: arg.Kind, TokenHash: arg.TokenHash, ExpiresAt: arg.ExpiresAt,
		}); err != nil {
			return fmt.Errorf("issue Customer credential: %w", err)
		}
		issued = c
		return nil
	})
	return issued, err
}

// VerifyCustomerCode exchanges the Customer's code for a password grant, consuming the code
// (customers R7, R8). The rules are the Users' (VerifyCode). It reports whether the code was right.
func (s *Store) VerifyCustomerCode(ctx context.Context, arg VerifyFirstTimeCodeParams) (verified bool, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		c, err := q.GetCustomerByEmail(ctx, arg.Email)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find Customer: %w", err)
		}
		if c, err = holdCustomer(ctx, q, c.ID); err != nil {
			return err
		}
		row, err := q.GetCustomerCredential(ctx, c.ID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read Customer code: %w", err)
		}
		if !c.CanUseEmailCredential(FirstTimeCode) {
			return nil
		}
		next, accepted := EmailCredential{
			Kind: row.Kind, TokenHash: row.TokenHash, ExpiresAt: row.ExpiresAt, FailedAttempts: row.FailedAttempts, BlockedUntil: row.BlockedUntil,
		}.VerifyCode(arg)
		if accepted {
			if err := q.IssueCustomerCredential(ctx, IssueCustomerCredentialParams{
				CustomerID: c.ID, Kind: next.Kind, TokenHash: next.TokenHash, ExpiresAt: next.ExpiresAt,
			}); err != nil {
				return fmt.Errorf("exchange code for password grant: %w", err)
			}
		}
		if err := q.SetCustomerCodeAttempts(ctx, SetCustomerCodeAttemptsParams{
			CustomerID: c.ID, FailedAttempts: next.FailedAttempts, BlockedUntil: next.BlockedUntil,
		}); err != nil {
			return fmt.Errorf("save Customer code attempts: %w", err)
		}
		verified = accepted
		return nil
	})
	return verified && err == nil, err
}

// ReadCustomerCredential answers ErrNotFound unless the grant or link is live for an eligible
// Customer. It lets the API refuse a bad one before it hashes a password; the change itself
// is checked again under the lock.
func (s *Store) ReadCustomerCredential(ctx context.Context, arg EmailPasswordParams) error {
	if _, err := s.GetCustomerCredentialUser(ctx, GetCustomerCredentialUserParams{Kind: arg.Kind, TokenHash: arg.TokenHash, At: arg.At}); err != nil {
		return fmt.Errorf("read Customer credential: %w", err)
	}
	return nil
}

// SetCustomerPassword consumes a grant or Reset link, sets the password and ends all the
// Customer's sessions and outstanding credentials, in one transaction (customers R7, R12;
// users-and-access R48). The entry, a first password set or a reset by the Customer
// themselves, is written with it (activity-log R4). It answers ErrNotFound when the
// credential does not work.
func (s *Store) SetCustomerPassword(ctx context.Context, arg EmailPasswordParams, by ActivityBy) (updated Customer, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		c, err := q.GetCustomerCredentialUser(ctx, GetCustomerCredentialUserParams{Kind: arg.Kind, TokenHash: arg.TokenHash, At: arg.At})
		if err != nil {
			return fmt.Errorf("find password credential: %w", err)
		}
		if c, err = holdCustomer(ctx, q, c.ID); err != nil {
			return err
		}
		row, err := q.GetCustomerCredential(ctx, c.ID)
		if err != nil {
			return fmt.Errorf("read password credential: %w", err)
		}
		if !(EmailCredential{Kind: row.Kind, TokenHash: row.TokenHash, ExpiresAt: row.ExpiresAt}).AllowsCustomerPassword(c, arg) {
			return ErrNotFound
		}
		if updated, err = q.SetCustomerPassword(ctx, SetCustomerPasswordParams{ID: c.ID, PasswordHash: pgtype.Text{String: arg.PasswordHash, Valid: true}}); err != nil {
			return fmt.Errorf("replace Customer password: %w", err)
		}
		if err := q.DeleteCustomerSessions(ctx, c.ID); err != nil {
			return fmt.Errorf("end Customer sessions: %w", err)
		}
		if err := q.DeleteCustomerCredential(ctx, c.ID); err != nil {
			return fmt.Errorf("consume Customer credential: %w", err)
		}
		if arg.Kind == ResetLink {
			return q.addActivity(ctx, CustomerPasswordReset(updated, by.AsCustomer(updated)))
		}
		return q.addActivity(ctx, CustomerPasswordSet(updated, by.AsCustomer(updated)))
	})
	return updated, err
}

// ChangeProfileParams is what a Customer changes of themselves (customers R13): nil leaves
// a field as it is. The email and password are not here: neither can be changed this way.
type ChangeProfileParams struct {
	ID      int64
	Name    *string
	Phone   *string
	Address *DeliveryAddress
}

// UpdateProfile changes what arg names and writes an entry for each change, with its value
// before and after, in the same transaction (customers R13, activity-log R4). It answers
// ErrNotFound for no such Customer.
func (s *Store) UpdateProfile(ctx context.Context, arg ChangeProfileParams, by ActivityBy) (updated Customer, err error) {
	err = InTx(ctx, s.pool, func(q *Queries) error {
		before, err := holdCustomer(ctx, q, arg.ID)
		if err != nil {
			return err
		}
		p := UpdateCustomerProfileParams{
			ID: before.ID, Name: before.Name, Phone: before.Phone,
			AddressRecipient: before.AddressRecipient, AddressPhone: before.AddressPhone, AddressLine: before.AddressLine,
			AddressSubdistrict: before.AddressSubdistrict, AddressDistrict: before.AddressDistrict,
			AddressProvince: before.AddressProvince, AddressPostcode: before.AddressPostcode,
		}
		if arg.Name != nil {
			p.Name = *arg.Name
		}
		if arg.Phone != nil {
			p.Phone = *arg.Phone
		}
		if a := arg.Address; a != nil {
			text := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }
			p.AddressRecipient, p.AddressPhone, p.AddressLine = text(a.Recipient), text(a.Phone), text(a.Line)
			p.AddressSubdistrict, p.AddressDistrict, p.AddressProvince, p.AddressPostcode = text(a.Subdistrict), text(a.District), text(a.Province), text(a.Postcode)
		}
		if updated, err = q.UpdateCustomerProfile(ctx, p); err != nil {
			return fmt.Errorf("update Customer profile: %w", err)
		}
		for _, e := range CustomerEdited(before, updated, by.AsCustomer(updated)) {
			if err := q.addActivity(ctx, e); err != nil {
				return err
			}
		}
		return nil
	})
	return updated, err
}
