# Users & access

Status: Amended by ADR 0003 and ADR 0004

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
- **R27** A User sets their password through a Setup link, which works once and expires 24 hours after it is issued (ADR 0002).
- **R28** Creating a User issues a Setup link. The Owner sees it once, when it is issued, and passes it to that person themselves.
- **R29** The Owner can issue a new Setup link for a User at any time; any earlier link stops working at once.
- **R30** After setting a password through a Setup link, the person is sent to Login.
- **R31** Only the Owner resets a password, from the Users list. The User's password stops working, their sessions end at once, and a new Setup link is issued.
- **R32** An Inactive User's Setup link stops working, and none can be issued for them.
- **R33** The Users list shows which Users have not set a password yet.
- **R34** The Owner's password is not reset in the system. A developer issues the Owner's Setup link, at installation and whenever the Owner forgets their password. No password is kept in any repo. When the Owner sets a password through a new Setup link, all the Owner's sessions end at once (ADR 0003).
- **R35** The Owner can change the name of every User except their own (ADR 0004), and change their Role between Admin and Staff. A Role change applies at once to the sessions that User has open.
- **R36** The Login log records every successful login, failed login and logout: the time, the email entered, the User if it matches one, the result, the IP address and the browser or device.
- **R37** The Login log never records a password, whether right or wrong.
- **R38** The Login log lists the newest entries first and can be filtered by User and by result.
- **R39** Login log entries cannot be edited, and are kept indefinitely.
- **R40** Until its module is built, each page in R11 shows only its title and "Coming soon", under the same access rules.

## Out of scope

- Locking a User out after failed logins. The requirements owner left it out because this is a practice project (2026-11-06). Together with the 6-character minimum (R19), it needs revisiting before any real data goes in.
- Sending email, including sending the Setup link by email. A later feature.
- A "Forgot password" action on Login. It proves nothing without email, so it arrives with the email feature.
- A one-time code at login. A later feature.
- Changing your own password while logged in. Until it exists, a User asks the Owner for a reset.
- A log of anything other than logins and logouts, such as who changed whose Role.
- More than one Shop in a deployment.

## Open questions

| # | Question | Asked of | Since |
| --- | --- | --- | --- |
| 1 | How are repeated failed logins limited: how many failures, over what window, per email or per address, and what happens then (a wait, or a lock the Owner clears)? Today nothing limits them. Found by the [security review](../security-notes.md). | Requirements owner | 2026-11-08 |
| 2 | Should R19's 6-character minimum rise (e.g. to 8 or 10), with a maximum and a list of refused common passwords? | Requirements owner | 2026-11-08 |
