# Customers

Status: Confirmed (2026-11-20, requirements owner's task-scoped delegation in business ADR 0014)

## Purpose

People who buy from the Shop have a Customer account so they can log in to the Storefront and order. The Owner and Admins look after these accounts in StockFlow. Customers are not Users ([business ADR 0014](../business-adr/0014-customers-and-the-storefront.md)).

## Rules

- **R1** A Customer has no Role, logs in only to the Storefront, and never reaches StockFlow. A User's login never opens the Storefront. The same email may belong to a User and to a Customer.
- **R2** A Customer has a name, an email, a phone number, a status, an optional saved Delivery address (Storefront R8) and the time they joined. The name is required, up to 120 characters, in any language.
- **R3** An email belongs to one Customer only, compared without regard to letter case, and cannot change after the Customer is created. A Deleted Customer's email can be used again.
- **R4** The phone number is required: 9 or 10 digits starting with 0.
- **R5** Anyone signs up on the Storefront: they enter an email, confirm it with a Verification code, then give their name, phone number and password. The Customer exists only once all of this is done.
- **R6** Signing up with an email that already has a Customer says so and offers Log in and Forgot password; nothing is sent. Signing up with a Deleted Customer's email starts a new Customer with no link to the old one.
- **R7** The Owner and Admins create a Customer in StockFlow with a name, an email and a phone number. The Customer is emailed a Verification code at once and sets their own password through it; nobody else sets or sees it. A Customer without a password can ask for a new code from the Storefront.
- **R8** A Verification code has six digits, expires 10 minutes after it is sent, and works once; sending a new one makes the previous one stop working. Five wrong attempts block codes for that email for 10 minutes. Sends share Users & access R43's limits, counted separately for Customers.
- **R9** A Customer logs in with an email and a password on one step. A failed login shows the same message whatever the reason: a wrong email, a wrong password, an Inactive Customer, or a Customer without a password. Failed logins are limited as Users & access R55–R57 say (business ADR 0017).
- **R10** A Customer may be logged in on several devices; a session ends 7 days after login, and logging out ends it on that device only.
- **R11** A password has 8 to 128 characters (business ADR 0017).
- **R12** A Customer with a password recovers it through Forgot password: a Reset link that works once, expires 30 minutes after it is sent, and only the newest works. Saving the new password ends all their sessions and emails them that it changed. A request says the same thing whatever the email (Users & access R44).
- **R13** A Customer changes their own name, phone number and saved Delivery address. They cannot change their email, or their password while logged in; they use Forgot password.
- **R14** The Owner and Admins reach the Customers list. It shows each Customer's name, email, phone number, status, whether they have set a password, when they joined and how many Orders they have placed, newest first. It is searched by name, email or phone number and filtered by status.
- **R15** The Owner and Admins make a Customer Inactive or Active again. An Inactive Customer cannot log in or receive codes or Reset links, their sessions end at once, and their codes and links stop working. Their Pending Orders stay as they are.
- **R16** The Owner and Admins Delete a Customer, only an Inactive one. A Deleted Customer is not in the Customers list, cannot be changed or restored, and is an unknown email on the Storefront. Their Orders keep their name, phone number and Delivery address (business ADR 0013).
- **R17** Staff do not reach the Customers list. Every Role sees a Customer's name, email and phone number on that Customer's Orders (Orders R23).
- **R18** Verification code, Reset link and password-changed emails to Customers follow Users & access R49.

## Out of scope

- Signing in with Google, Facebook or LINE. Planned for later: a password is one way a Customer signs in, and these rules never require one beyond R9 (business ADR 0014).
- A Customer deleting their own account or personal data. Settle it before real Customers sign up (business ADR 0014, PDPA).
- Changing a Customer's email.
- More than one saved Delivery address.
- Customer groups, loyalty points, wish lists and reviews.

## Open questions

None. Every rule here was decided by the agent under the requirements owner's task-scoped delegation of 2026-11-20 (business ADR 0014).
