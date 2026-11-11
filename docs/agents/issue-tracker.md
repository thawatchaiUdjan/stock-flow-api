# Issue tracker

GitHub issues in `thawatchaiUdjan/stock-flow-api` hold this repo's specs, implementation tickets and questions. Requirements and decisions live in this checkout. `stock-flow-context` issues and PRs remain historical evidence; no context issue or approval is required for new work.

## Fetch and publish

When a skill says to fetch a ticket, read its full body and comments, linked API spec, and cited local rules/ADRs. For legacy context specs, preserve the original link and carry necessary acceptance criteria into an API issue before resuming work. Do not assume a historical delegation still applies.

When a skill says to publish, use this repo. `/to-spec` publishes a spec linked to the merged record PR and rule IDs. `/to-tickets` publishes approved API implementation slices linked to that spec. Each ticket records owner authorisation, scope, acceptance criteria, agreed test seam, contract impact and actual blockers. Link child tickets from the spec using GitHub sub-issues when available, or issue links otherwise. Readiness requires confirmed, merged rules and satisfied blockers.

Triage vocabulary: `needs-triage` (evaluate), `needs-info` (awaiting facts/decisions), `ready-for-agent` (specified and unblocked), `ready-for-human` (requires human implementation), `wontfix` (not planned). These map directly to the five canonical skill labels. A skill's default ready label never overrides record confirmation or a real blocker. Create missing labels before applying them.

PRs are review surfaces, not intake requests. Record PRs settle local rules/decisions; implementation PRs close tickets after checks and review. Both fill every section of `.github/pull_request_template.md`. No merge without explicit owner authorisation.

## Direct API/web requests

The owner authorises necessary task-related issue requests and replies directly between API and web. No extra handoff approval, context repo, middleman or coordinator is needed. Unrelated external messages still need authorisation.

When a task needs web work, create a request in `thawatchaiUdjan/stock-flow-web` and comment its link on the API issue. Include:

- Originating API issue, owner-authorised task scope, behaviour needed and relevant local rules.
- Contract operations/errors, compatibility or versioning needs, and publication commit when available.
- Acceptance evidence needed, which API work depends on the reply, and what can continue independently.

Let web own its planning, decisions and implementation. Reply directly to incoming web issues with an API issue link and disposition. A request does not approve new business rules. Route such decisions to the requirements owner and local record PR. Block only dependent work; keep independent tickets moving.

After a relevant contract is published, notify web on the linked request with its exact API commit and generated contract link. Web pins an API commit and tests integration against it; it never follows API `main` automatically. Record web's reply with its pin and test result before claiming adoption. Historical completion claims are not confirmation of the current pin.

## GitHub commands

Use `-R thawatchaiUdjan/stock-flow-api` for API and `-R thawatchaiUdjan/stock-flow-web` for web. Bare `#n` means this repo; qualify cross-repo references. For multiline bodies, write a UTF-8 file and use `--body-file`.

- Read: `gh issue view <n> --json number,title,body,labels,comments`.
- List: `gh issue list --state open --label ready-for-agent --json number,title,labels`.
- Create: `gh issue create --title "..." --body-file <path>`.
- Reply/link: `gh issue comment <n> --body-file <path>`.
- Update a spec: `gh issue edit <n> --body-file <path>`; preserve existing decisions and links.
- Close: comment with evidence first, then `gh issue close <n>`.
- Labels: `gh label list`; create missing canonical labels with `gh label create <name> --description "..." --color <hex>`.
- Pull requests: `gh pr create --body-file <path>`, `gh pr view`, `gh pr comment <n> --body-file <path>`.

GitHub shares issue and PR numbers. `git remote -v` supplies this repo's host; never infer another repo from a bare number.
