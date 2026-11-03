# Issue tracker

Tickets for this repo live as issues in **this** repo; `template-context` creates them from its specs. The spec itself is an issue in `template-context`. The code host is GitHub or GitLab; the commands for each are at the end.

The three remotes (`<owner>/<repo>`) are in the table in `AGENTS.md`. The CLI infers this repo from `git remote -v`; for another repo, pass `-R <owner>/<repo>`. A reference of the form `<owner>/<repo>#<number>` lives in another repo; a bare `#<number>` lives in this one.

## When a skill says "fetch the relevant ticket"

Read the ticket (command below). Then read the spec it links in `template-context` (with `-R`), and the record the ticket cites.

## When a skill says "publish to the issue tracker"

Create the issue in this repo. Never create tickets in another repo: work this change causes for `template-web` goes in the pull request's *Contract* section, and `template-context` writes the ticket from the record.

## Pull requests as a triage surface

**PRs as a request surface: no.**

## Commands

<!-- FILL: keep the section for your code host and delete the other. -->

### GitHub (`gh`)

- **Read**: `gh issue view <n> --json number,title,body,labels,comments`, plus `-R <owner>/<repo>` for another repo.
- **List**: `gh issue list --state open --label ready-for-agent --json number,title,labels`.
- **Create**: `gh issue create --title "..." --body "..."` (heredoc for multi-line bodies).
- **Comment**: `gh issue comment <n> --body "..."`.
- **Close**: comment first, then `gh issue close <n>`.
- **Pull requests**: `gh pr create`, `gh pr view`, `gh pr comment`. GitHub shares one number space across issues and PRs.

### GitLab (`glab`)

- **Read**: `glab issue view <n> --comments`, plus `-R <group>/<project>` for another repo.
- **List**: `glab issue list -O json --label ready-for-agent`.
- **Create**: `glab issue create --title "..." --description "..." --yes` (heredoc for multi-line descriptions). Without `--yes` it waits for a confirmation nobody gives.
- **Comment**: `glab issue note <n> --message "..."`.
- **Close**: note first, then `glab issue close <n>`; `close` takes no comment.
- **Merge requests**: `glab mr create --title "..." --description-file <file> --target-branch main --yes`, `glab mr view`, `glab mr note`. GitLab numbers issues and MRs separately.
