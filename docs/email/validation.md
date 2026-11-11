# Email access validation evidence

Date: 2026-11-11. Scope: API spec [#46](https://github.com/thawatchaiUdjan/stock-flow-api/issues/46), implementation tickets [#47](https://github.com/thawatchaiUdjan/stock-flow-api/issues/47) and [#48](https://github.com/thawatchaiUdjan/stock-flow-api/issues/48), merged business ADR 0011. No real credentials, mailbox delivery or web adoption were used as test evidence.

## Local checks

- `go build ./...`: passed.
- `go test ./...`: 481 fast tests passed across ten packages.
- `go vet -tags slow ./...`: passed, including compilation/analysis of slow tests.
- `gofmt -l .`: empty after formatting the local preview scratch source too.
- `go generate ./...`: generated SQL, OpenAPI and deterministic HTML/text previews successfully.
- Documentation link and staged whitespace checks passed after removing blank-line indentation in generated previews. Link checks are repeated after adding the handoff and this evidence file; final counts are recorded in the PR.

The first tracer tests failed before the relevant operations existed (404). Code-attempt and request-limit tests failed before their guards were added. Shared store contracts caught permissive fake behavior, including accepting a code as a password grant and allowing duplicate credential digests. Template tests failed before reset/completion variants existed. The final fast suite passes after all changes.

## Mutation proof

Twenty-four deliberate mutations of API, production shared policy, auth hashing and actual email-template guards were applied one at a time. Every targeted test failed with an assertion failure, not a compilation failure. Each file was restored byte-for-byte before the next case; the full fast suite then passed. Local raw logs and runner are retained under `.git/codex-email-implementation/` and are not committed or required to build the project.

| Deliberately broken behavior | Rule / source | Failing test evidence |
| --- | --- | --- |
| Code expiry; single-use code exchange | R27, R42, R48 | `TestEmailSetupCredentialsExpireAtTheirBoundaryAndWorkOnce` |
| Block enforcement; correct-code comparison | R27, R41 | `TestFiveWrongCodesBlockVerificationAcrossResendsForTenMinutes` |
| Block after four failures instead of five | R41 | `TestFourWrongCodesStillAllowTheCorrectCode` |
| Initial Owner exemption; Inactive exclusion | R32, R34, R44 | `TestRecoveryRequestsUseSetupForUnfinishedUsersAndShareCooldown` |
| Reset eligibility without an existing password | R31, R45 | Shared store contract: `Reset credential issuance requires an Active User with a password` |
| Grant expiry | R42 | `TestEmailSetupCredentialsExpireAtTheirBoundaryAndWorkOnce` |
| Credential purpose binding | R48 | Shared store contract: `email credential purposes cannot be substituted` |
| Sixty-second cooldown; five-per-email limit | R43 | `TestEmailRequestsRespectSixtySecondCooldownAndRollingHour` |
| Thirty-per-IP limit | R43 | `TestEmailRequestsLimitThirtyPerIPPerHour` |
| Ten-minute code/grant lifetimes | R27, R42 | Expiry-boundary test and `TestEmailCreatedUserVerifiesCodeSetsOwnPasswordAndLogsIn` |
| Thirty-minute Reset link lifetime | R31 | `TestResetLinksArePurposeBoundReplacedAndExpireAfterThirtyMinutes` |
| Independent HMAC secret | Technical ADR 0024 | `TestFirstTimeCodeDigestBindsSecretPurposeAndCaseInsensitiveUser` |
| Hashed grant storage | Technical ADR 0024 | First-time creation/verification/password/login tracer |
| Reject invalid credentials before password hashing | Spec #46 | `TestEmailCredentialsAreValidatedBeforeHashingAndRecheckedAfterHashing` |
| Existing six-character minimum | R19 | `TestEmailPasswordFlowsRetainSixCharacterMinimumAndDoNotConsumeOnValidationFailure` |
| Completion notification kind | R47 | `TestSelfServiceResetPreservesAccessUntilCompletionAndEndsEverySession` |
| Successful reset despite notification failure | R47 | `TestEmailFailuresPreserveCreationAndPasswordReset` |
| Actual rendered code in HTML | R49 | `TestBrevoSendsStockFlowCodeAsEscapedHTMLAndPlainText` |
| Required code-secret configuration | Spec #46 | `TestEachMissingEmailDependencyRefusesRequests/secret` |

Some rows group two mutations; the total is 24. Request-limit and code-transition policy is shared between the production store and fake, so these fast mutations affect actual production policy rather than a second copy of the rules. The fake does not claim to prove database locking, constraints or transaction rollback.

## PostgreSQL and provider boundaries

The existing shared store-port contract suite now includes email creation constraints, digest uniqueness, eligibility, code exchange/replay/attempt blocks, purpose binding, rolling limits, reset session/legacy-link invalidation, and deactivation/reactivation. It runs against the fake locally and the real store in CI.

New slow tests cover simultaneous code exchange, concurrent IP reservations, deactivation versus issuance, simultaneous Reset link completion, rollback after credential-deletion failure and old-password login racing recovery. Existing RLS tests include both new tables. Slow tests are written and compile under vet; they have not been run locally or awaited, per the repository testing policy.

The real Brevo adapter was exercised through its public Send boundary with a fake HTTP transport: endpoint/API-key protocol, actual rendered content, HTML escaping, both text parts, bounded request lifetime, cancellation, redirects and generic errors. No provider request was sent over the network. Provider acceptance and real inbox delivery remain unverified.

## Visual evidence

All three actual delivery templates were rendered at explicit 390 px and 800 px viewports using headless Edge. Layout width equalled scroll width in all six captures; the images were visually inspected. See the [preview gallery](README.md). This proves browser-preview layout, not universal email-client rendering or live inbox delivery.
