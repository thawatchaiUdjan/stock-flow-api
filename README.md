# template-api

The backend for <!-- FILL: project name -->. It implements the rules recorded in [`template-context`](../template-context) and publishes the API contract that [`template-web`](../template-web) consumes.

```
<workspace>/
├── template-context/   the record: what the system must do, and why
├── template-api/       ← this repo
└── template-web/       frontend
```

Keep the three clones side by side: the agent docs refer to the siblings as `../template-context` and `../template-web`.

## First-time setup

1. Create the repo on GitHub or GitLab and add `origin`. In `docs/agents/issue-tracker.md`, keep the command section for that host and delete the other; delete the template folder of the host you don't use (`.github/` or `.gitlab/`).
2. The stack is chosen from `template-context` before the first feature ([its workflow](../template-context/docs/workflow.md#before-the-first-feature-choose-the-stack)) and arrives here as a stack ticket. Working it records the stack as the first ADRs in `docs/adr/` (language, framework, database), scaffolds the app, and fills the *Stack and commands* table in `AGENTS.md` and the sections of `CODING_STANDARDS.md`.
3. Fill the remaining placeholders. `grep -rn 'FILL' --exclude-dir=.agents .` lists every one.
4. `./scripts/install-skills.sh` installs the skills listed below.
5. Commit.

## How work arrives

The full workflow, step by step, is in `template-context`'s [docs/workflow.md](../template-context/docs/workflow.md).

Tickets are created in this project by `template-context` (`/to-tickets`), each linking its spec and the rules it implements. `/dispatch` in `template-context` starts a fresh agent here for each ready ticket; to work one by hand instead, start a session with `--add-dir ../template-context` and run `/implement <ticket>`. Either way, `/implement` builds test-first with `tdd`, runs the fast layer, and reviews the result with `code-review` before committing.

## Agent skills

From [mattpocock/skills](https://github.com/mattpocock/skills), installed by `scripts/install-skills.sh` into `.agents/skills/` (read by Codex, Cursor, Gemini CLI, Copilot and others), with symlinks in `.claude/skills/` for Claude Code. `skills-lock.json` records the source and hash of each one.

| Skill | Why it's here |
| --- | --- |
| `implement` | Builds a ticket end to end: calls `tdd`, runs the suite, then calls `code-review` |
| `tdd` | Red-green loop at the ticket's seam; `implement` calls it |
| `codebase-design` | Module and interface vocabulary; `tdd` calls it when a seam's shape is in question |
| `code-review` | Two-axis review, against `CODING_STANDARDS.md` and against the spec; `implement` calls it |
| `diagnosing-bugs` | Disciplined loop for hard bugs and performance regressions |
| `setup-matt-pocock-skills` | Re-runs tracker setup if it ever changes. User-invoked only, so it costs no context until called |

Left out on purpose:

- `domain-modeling`, `grill-with-docs`: the glossary and business decisions belong to `template-context`. Installed here, `domain-modeling` would create a second glossary. Technical ADRs are written by hand, per `docs/agents/domain.md`.
- `to-spec`, `to-tickets`: specs and tickets come from `template-context`.
- Stack-specific skills (a language's or framework's best practices): add them once the stack is chosen, and only where the agent keeps making the same mistake. Read a skill's `SKILL.md` and scripts before installing it. Install one with `npx skills add <source> --skill <name> --agent claude-code --agent universal`, then add its name to `OTHER_SKILLS` in `scripts/install-skills.sh`, or the script's check rejects it. The script refreshes only the mattpocock set; refresh the others by name with `npx skills@1.7.4 update -p <name>`.

### Updating

Skills do not update themselves. They are normally refreshed for all three repos at once from `template-context` (`scripts/update-all-skills.sh`), which runs this repo's `./scripts/install-skills.sh`; running that script here on its own refreshes only this repo. Either way, read `git diff skills-lock.json .agents/skills` before committing: a skill change changes agent behaviour, so it is reviewed like code.

Never edit a file under `.agents/skills/`: the next refresh overwrites it. Put repo-specific rules in `AGENTS.md`, `CODING_STANDARDS.md` or `docs/agents/`.
