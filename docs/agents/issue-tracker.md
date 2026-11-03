# Issue tracker

Tickets for this repo live as issues in **this** repo; `stock-flow-context` creates them from its specs. The spec itself is an issue in `stock-flow-context`. The code host is GitHub; the commands are at the end.

The three remotes (`<owner>/<repo>`) are in the table in `AGENTS.md`. The CLI infers this repo from `git remote -v`; for another repo, pass `-R <owner>/<repo>`. A reference of the form `<owner>/<repo>#<number>` lives in another repo; a bare `#<number>` lives in this one.

## When a skill says "fetch the relevant ticket"

Read the ticket (command below). Then read the spec it links in `stock-flow-context` (with `-R`), and the record the ticket cites.

## When a skill says "publish to the issue tracker"

Create the issue in this repo. Never create tickets in another repo: work this change causes for `stock-flow-web` goes in the pull request's *Contract* section, and `stock-flow-context` writes the ticket from the record.

## Pull requests as a triage surface

**PRs as a request surface: no.**

## Commands

### GitHub (`gh`)

- **Read**: `gh issue view <n> --json number,title,body,labels,comments`, plus `-R <owner>/<repo>` for another repo.
- **List**: `gh issue list --state open --label ready-for-agent --json number,title,labels`.
- **Create**: `gh issue create --title "..." --body "..."` (heredoc for multi-line bodies).
- **Comment**: `gh issue comment <n> --body "..."`.
- **Close**: comment first, then `gh issue close <n>`.
- **Pull requests**: `gh pr create`, `gh pr view`, `gh pr comment`. GitHub shares one number space across issues and PRs.
