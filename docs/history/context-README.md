# stock-flow-context

The record for stock-flow: requirements, business decisions and shared vocabulary. No code. Two sibling repos implement it:

```
<workspace>/
├── stock-flow-context/   ← this repo: what the system must do, and why
├── stock-flow-api/       backend: implements the record, publishes the API contract
└── stock-flow-web/       frontend: implements the record, consumes the contract
```

Keep the three clones side by side: the agent docs refer to the siblings as `../stock-flow-api` and `../stock-flow-web`.

## Project goal

A practice project for frontend development.

## First-time setup

1. Create the three repos on one code host (GitHub or GitLab), under one owner, and add each one's `origin`. In `docs/agents/issue-tracker.md` and the two siblings' copies, keep the command section for that host and delete the other. In each repo, delete the template folder of the host you don't use (`.github/` or `.gitlab/`).
2. Workers are started through Orca orchestration; `scripts/dispatch.sh` is the fallback without Orca (set its `AGENT`, `claude` or `codex`). The workers' allowed commands are added once the stack is chosen. Fill the other placeholders. `grep -rn 'FILL' --exclude-dir=.agents .` lists every one. If you rename the repos, also rename them in `REPOS` in `scripts/update-all-skills.sh` and `scripts/dispatch.sh`.
3. `./scripts/install-skills.sh` installs the skills listed below.
4. Create the `ready-for-agent` label in all three repos (`gh label create ready-for-agent`, or `glab label create --name ready-for-agent`, with `-R <owner>/<repo>` for each sibling). `/to-spec` and `/to-tickets` apply it.
5. Commit. Then choose the stack, before the first feature: [docs/workflow.md](https://github.com/thawatchaiUdjan/stock-flow-context/blob/4c81bd45ff27cca31bd911e5e429b46c273265d2/docs/workflow.md#before-the-first-feature-choose-the-stack).

There is no need to run `/setup-matt-pocock-skills`: the files it would write (`docs/agents/*.md` and the `## Agent skills` block in `AGENTS.md`) are already here, adapted for three repos. Run it only to switch issue trackers, and only once `origin` is reachable, since it stops if `gh repo view` / `glab repo view` fails.

## How work flows

Once per project: `grill the stack → stack tickets ✋ → stack PRs ✋ → plan the design in stock-flow-web ✋`. Then for every feature: session A `grill → record PR ✋ → /to-spec → /to-tickets ✋`, which starts session B `/dispatch → PRs ✋`. Step by step, with what you type and what the agent does: [docs/workflow.md](https://github.com/thawatchaiUdjan/stock-flow-context/blob/4c81bd45ff27cca31bd911e5e429b46c273265d2/docs/workflow.md). In a session here, ask **"what's next?"** at any point.

## Agent skills

From [mattpocock/skills](https://github.com/mattpocock/skills), installed by `scripts/install-skills.sh` into `.agents/skills/` (read by Codex, Cursor, Gemini CLI, Copilot and others), with symlinks in `.claude/skills/` for Claude Code. `skills-lock.json` records the source and hash of each one.

| Skill | Why it's here |
| --- | --- |
| `grill-with-docs` | The main job of this repo: interview the owner and write the record while doing it |
| `grilling`, `domain-modeling` | `grill-with-docs` calls both; it cannot run without them. `domain-modeling` also writes ADRs and glossary terms |
| `to-questionnaire` | Questions whose answer must come from someone who isn't in the session |
| `research` | Facts that can be looked up instead of asked |
| `to-spec` | Turns a confirmed requirement into a spec |
| `to-tickets` | Hands the spec to the implementing repos as tickets |
| `writing-for-agents` | This repo's output is read by agents: `AGENTS.md`, tickets, the docs in `docs/agents/` |
| `setup-matt-pocock-skills` | Re-runs tracker setup if it ever changes. User-invoked only, so it costs no context until called |
| `dispatch` | Written in this repo (`OTHER_SKILLS` in the install script): the coordinator: checks each ticket is ready, starts a supervised worker for it in the implementing repo through Orca orchestration (`scripts/dispatch.sh` without Orca), relays questions and reports what came back. No upstream skill hands work to another repo |

Left out on purpose: `tdd`, `implement`, `code-review`, `diagnosing-bugs` (there is no code here), and any overlapping methodology such as superpowers (two process frameworks compete over the same steps).

### Updating

Skills do not update themselves. To refresh, run `./scripts/update-all-skills.sh` here: it refreshes all three repos (each with its own `scripts/install-skills.sh`), so they never drift onto different versions, and reports what changed in each. Then read each repo's `git diff skills-lock.json .agents/skills` and commit it in that repo: a skill change changes agent behaviour, so it is reviewed like code.

Never edit a file under `.agents/skills/`: the next refresh overwrites it. Put repo-specific rules in `AGENTS.md` or `docs/agents/`.

A skill from another source, or one written here, goes in `OTHER_SKILLS` in `scripts/install-skills.sh`, or the script's check rejects it. The script refreshes only the mattpocock set; refresh the others by name with `npx skills@1.7.4 update -p <name>`.
