# Users & access

Status: Amended by business ADR 0003, business ADR 0004 and [business ADR 0011](../business-adr/0011-email-first-time-setup-and-self-service-recovery.md)

## Purpose

Everyone who works in the Shop uses this module to log in. It decides who has a User, what each Role may reach, and where each User lands after logging in.

## Rules

- **R1** The system serves one Shop; all data in it belongs to that Shop.
- **R2** A User logs in with an email and a password.
- **R3** An email belongs to one User only, and is compared without regard to letter case.
- **R4** Nobody can sign up on their own.
- **R5** A failed login shows the same message, whatever the reason: a wrong email, a wrong password, an Inactive User, or a User with no password yet.
- **R6** A User may be logged in on several devices at once.
- **R7** A logged-in User can log out, which ends the session on that device only.
- **R8** There are three Roles: Owner, Admin and Staff. Every User has exactly one.
- **R9** There is exactly one Owner, created when the system is installed. No other Owner can be created.
- **R10** Only the Owner creates Users, and chooses Admin or Staff for each.
- **R11** Each Role reaches these pages; a page not yet built follows this table once it is:

  | Page | Owner | Admin | Staff |
  | --- | --- | --- | --- |
  | Dashboard | Yes | Yes | No |
  | Product List | Yes | Yes | Yes |
  | Product Form | Yes | Yes | No |
  | Stock Movements | Yes | Yes | Yes |
  | Orders List | Yes | Yes | Yes |
  | Create Order | Yes | Yes | Yes |
  | Reports | Yes | Yes | No |
  | Users list and form | Yes | No | No |
  | Login log | Yes | No | No |

- **R12** After logging in, the Owner and Admins land on the Dashboard, and Staff on the Orders List.
- **R13** What a Role may do within a page, such as which actions it sees, is stated by the module that owns the page.
- **R14** A User has a name, an email, a Role and a status. The name is required and may be in any language.
- **R15** A User's email cannot change after the User is created.
- **R16** A User is Active or Inactive, and only the Owner sets this. An Inactive User cannot log in, and their sessions end at once.
- **R17** A User is never deleted, so that what they did stays attributed to them.
- **R18** The Owner's own status, Role and name cannot be changed in the system (ADR 0004).
- **R19** A password has at least 6 characters.
- **R20** Every User sets their own password, the first time and after a reset. Nobody else sets or sees it.
- **R21** A session ends 7 days after login, however much it is used.
- **R22** A page the User's Role cannot reach does not appear in their navigation. Opened directly, it says they have no access and links to their landing page.
- **R23** The API refuses whatever the User's Role cannot reach, whatever the web app shows.
- **R24** A visitor who is not logged in is sent to Login from any page. After logging in they return to that page, or to their landing page if their Role cannot reach it.
- **R25** A logged-in User who opens Login is sent to their landing page.
- **R26** A User whose session has ended is sent to Login with a message saying so.
- **R27** An Admin or Staff User sets their first password after verifying a First-time code emailed to their registered address. The code contains six digits and expires 10 minutes after issue. Subsequent logins use email and password without a code (business ADR 0011).
- **R28** Creating an Admin or Staff User automatically emails their First-time code. The Owner does not receive a Setup link or code to pass on (business ADR 0011).
- **R29** An Active Admin or Staff User without a password can request or resend a First-time code through password setup on Login. Issuing a new code invalidates the previous code; the Owner cannot issue password-reset instructions for another User (business ADR 0011).
- **R30** After first setting or resetting a password, the person is sent to Login. Neither flow logs them in automatically (business ADR 0011).
- **R31** An Active User with a password, including the Owner, requests their own Reset link through Forgot password on Login. It works once, expires 30 minutes after issue, and only the newest link works. Saving the new password ends all their sessions at once. There is no Owner-triggered password-reset action (business ADR 0011).
- **R32** An Inactive User cannot receive password-setup or recovery instructions. Making a User Inactive invalidates their codes, verification grants, Reset links and any Setup link. Reactivation requires fresh instructions; old credentials do not revive (business ADR 0011).
- **R33** The Users list shows which Users have not set a password yet.
- **R34** At installation only, a developer issues the first Owner's Setup link: single-use and valid for 24 hours. Initial Owner setup sends no email or First-time code, including through public requests before installation setup completes. Afterwards, the Owner uses Forgot password under R31. Setting the Owner's password through the installation link ends all Owner sessions. No password is kept in any repo (business ADR 0003 and business ADR 0011).
- **R35** The Owner can change the name of every User except their own (ADR 0004), and change their Role between Admin and Staff. A Role change applies at once to the sessions that User has open.
- **R36** The Login log records every successful login, failed login and logout: the time, the email entered, the User if it matches one, the result, the IP address and the browser or device.
- **R37** The Login log never records a password, whether right or wrong.
- **R38** The Login log lists the newest entries first and can be filtered by User and by result.
- **R39** Login log entries cannot be edited, and are kept indefinitely.
- **R40** Until its module is built, each page in R11 shows only its title and "Coming soon", under the same access rules.
- **R41** Five incorrect First-time code attempts block code verification for that email for 10 minutes. Resending does not bypass the block or reset its failed-attempt allowance. This does not lock ordinary password login (business ADR 0011).
- **R42** Successfully verifying a First-time code grants 10 minutes to set the first password. This verification grant is single-use and does not create a logged-in session (business ADR 0011).
- **R43** First-time code and Reset link email requests share limits: one send per 60 seconds, five per email per hour, and 30 per IP address per hour. Exact windows, counter semantics and retry handling are delegated technical details within these limits (business ADR 0011).
- **R44** Public setup and recovery email requests return the same eligibility-neutral message. They do not reveal whether the email matches a User, their status, or whether they have set a password. Unknown and Inactive Users receive nothing (business ADR 0011).
- **R45** A recovery request for an eligible Active Admin or Staff User without a password sends a First-time code instead of a Reset link. The initial Owner email exemption in R34 takes precedence (business ADR 0011).
- **R46** If sending the initial email fails, the newly created User remains without a password and can retry through password setup. The Owner sees creation success and email-sending failure separately; public requests retain the generic response (business ADR 0011).
- **R47** Requesting a Reset link does not change the current password or end sessions. After a successful reset, the system emails a completion notification; notification failure does not undo the password change (business ADR 0011).
- **R48** Password setup and recovery codes, links and verification grants work once. Successful password setup or reset invalidates all outstanding setup and recovery credentials for that User (business ADR 0011).
- **R49** First-time code, Reset link and reset-completion emails use English, StockFlow branding, responsive HTML and a plain-text alternative. API owns the email templates and follows the existing web design system, adapted to email clients (business ADR 0011).

## Out of scope

- Locking a User out after failed logins. The requirements owner left it out because this is a practice project (2026-11-06). Together with the 6-character minimum (R19), it needs revisiting before any real data goes in.
- A one-time code on ordinary password logins after first-time setup.
- Changing your own password while logged in. Users recover through Forgot password on Login.
- Changing a User's email or recovering lost mailbox access inside the application. Email remains immutable (R15); lost-mailbox cases require developer assistance outside the app.
- A log of anything other than logins and logouts, such as who changed whose Role.
- More than one Shop in a deployment.

## Open questions

| # | Question | Asked of | Since |
| --- | --- | --- | --- |
| 1 | How are repeated failed logins limited: how many failures, over what window, per email or per address, and what happens then (a wait, or a lock the Owner clears)? Today nothing limits them. Found by the [security review](../security-notes.md). | Requirements owner | 2026-11-08 |
| 2 | Should R19's 6-character minimum rise (e.g. to 8 or 10), with a maximum and a list of refused common passwords? | Requirements owner | 2026-11-08 |

The owner confirmed the email setup/recovery design on 2026-11-11 in business ADR 0011, retaining R15 and R19 unchanged. The two questions above concern broader policy and remain outside this feature. The record becomes authoritative on merge; runtime rollout and web adoption are separate work.
