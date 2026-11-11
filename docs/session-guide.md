# Sessions and reviewer settings

Use this guide with [AGENTS.md](../AGENTS.md), [the API workflow](workflow.md) and [testing policy](agents/testing.md). It guides available host tools; it installs no automatic workers, worker service or required coordinator. Do not launch extra implementation sessions unless requested. Work only in this API repository. Follow branch/worktree rules and use one writer per checkout; reviewers are read-only.

## Keep useful context together

Keep requirements discussions, `/grill-with-docs`, `/to-spec` and `/to-tickets` for one feature in the same session. A record PR still needs owner merge authorisation and must merge before dependent specs/implementation treat new rules as authoritative. Keep small fixes, relevant short discussions, a ticket's tests, review repairs and PR creation together too.

Consider a fresh implementation session after long planning or for an independent substantial ticket only when a compact handoff can replace irrelevant history. A new session is optional. Return to the original session when unresolved planning context is still useful. Before handing off, record important chat decisions, authorisation, progress, blockers and check/review evidence in the local record, API ticket or PR. Chat alone is not the durable record. Keep business decisions in the existing requirements/ADR process; do not turn a handoff into a new source of product rules.

Changing model/effort and opening a new session are separate choices. Use supported host settings to change model/effort within the current session when useful. A prompt requesting a model does not change active settings. Report requested and effective settings separately; mark effective settings unconfirmed unless tooling exposes them. In Codex CLI use `/model`; in supported desktop hosts use the model/reasoning picker. Check the host's actual controls first. [Official OpenAI model guidance](https://learn.chatgpt.com/docs/models) describes those controls and availability; checked 2026-11-11.

## Starting recommendations

These are repository starting points, not guarantees of quality, speed or savings. Recheck the host's available models and supported effort levels at task start. The current Codex delegation tool advertises all four IDs below; that is availability evidence, not proof of the active model. If unavailable, explain the missing choice and proposed available substitute. For implementation, use an available model suited to the same role and disclose the substitution; review has the stricter fallback below.

| Work | Requested model | Effort |
| --- | --- | --- |
| Requirements, planning and normal feature implementation | GPT-6.1 Sol (`gpt-6.1-sol`) | `medium` |
| Small, clear fixes with known scope | GPT-6.1 Terra (`gpt-6.1-terra`) | `medium` |
| Demonstrated difficult diagnosis: unresolved evidence, concurrency or repeated failed hypotheses | GPT-6.1 Sol (`gpt-6.1-sol`) | `high` |
| Diagnosis still blocked for a specific reason after that | Consider GPT-6 Astra (`gpt-6-astra`) | `high` |
| Already separate routine formatting task, optionally | GPT-6 Luna (`gpt-6-luna`) | `low` |
| Each Standards and Spec reviewer, unless owner overrides | GPT-6.1 Sol (`gpt-6.1-sol`) | `medium` |

Do not create a session or agent merely to format a short PR body. Escalate only with a named difficulty and the evidence the stronger setting should resolve. A host may expose different effort labels; verify their mapping instead of assuming equivalence.

## Stage boundaries and authority

At every boundary (planning, record PR, spec, tickets, implementation, checks/review, repairs, PR and handoff/resume), give:

```text
Next action: <concrete next step, or the exact owner decision needed>
Session: stay | open new | return <session reference>
Model and effort: <requested choice; effective setting and evidence, or unconfirmed>
Paste this prompt: <filled continuation if a new session, return, or manual review is needed; otherwise "Not needed; continuing here.">
```

Fill this block with actual paths, issue/PR links, commits and remaining work. Persist decisions/progress in the appropriate local record, ticket or PR as work advances. In Autopilot, continue the already authorised stages without asking the owner to type each slash command. Stop only dependent work for unresolved decisions or real blockers. In Approval, retain the owner's stage gates: present the stage result and obtain the required approval before advancing. If the inherited gate is unclear, ask rather than convert modes. Both modes require requirements confirmation, merged authoritative record where needed, and explicit merge authorisation. Neither inherits historical automatic-merge grants.

## Implementation and review

Use `/implement` with `tdd` at the ticket's agreed seam, from local requirements, glossary and ADRs. Preserve security/transaction guarantees, generated contract publication, compatibility and web adoption checks in [the workflow](workflow.md). For an API validation fix, use existing HTTP tests through `api.New` and `httptest` when that seam can prove the rule. Do not invent a new field limit or security policy.

Run `go build ./...`, `go test ./...`, `go vet -tags slow ./...`, `gofmt -l .`, `go generate ./...` and `bash scripts/check-links.sh` as required locally. Confirm generation leaves SQL and OpenAPI current. Follow [testing policy](agents/testing.md): slow tests stay in CI, nobody waits for them, except the existing pre-CI fallback. Documentation-only work uses meaningful link, provenance and workflow checks instead of artificial product tests; the current workflow and definition of done still require the listed repo checks.

After implementation and required checks, automatically invoke `code-review` before PR completion. Load [the local review override](agents/code-review.md) as well as the installed skill. It defines explicit reviewer settings, exact reviewed-state capture (including new untracked implementation files), focused context, read-only review and targeted repair review. Keep Standards and Spec reports separate. Skip Spec only when no specification or acceptance source exists; a documentation task's owner acceptance conditions count.

If explicit reviewer settings are unsupported, report that and provide a filled handoff for a fresh Sol/medium review session. Do not silently inherit host defaults. If subagents remain unavailable there, review Standards and Spec as separate passes and disclose reduced isolation. Finish there without another identical handoff loop. Report requested settings and whether tooling confirms effective settings.

## Copyable prompts

Replace bracketed fields with actual values before use. The agent supplies a fully filled prompt when handing work off. Each continuation retains the existing mode; selecting a model is still a separate host action.

### Start a feature

```text
[Autopilot or Approval, matching the owner's choice]: plan [feature] in
C:/ProjectsDev/projects/stock-flow/stock-flow-api. Read AGENTS.md,
docs/workflow.md, docs/session-guide.md and applicable local skill instructions.
Keep requirements, /to-spec and /to-tickets in this session. Record decisions
locally. Scope and delegation: [owner's exact authority and unresolved decisions].
Use host settings for Sol/medium where available; report actual evidence.
Preserve requirements confirmation, Approval gates if selected, and explicit
merge permission. Prepare a branch/PR; stop before merging.
```

### Implement a ready ticket

```text
Continue [existing Autopilot or Approval] for API ticket [URL], spec [URL],
merged record [commit/PR and rule IDs]. Repository:
C:/ProjectsDev/projects/stock-flow/stock-flow-api. Read AGENTS.md, workflow,
session guide, testing policy and invoked skills. Authority: [owner scope/gates].
Branch/checkpoint: [branch and commit; dirty paths or clean state].
Implement only [acceptance criteria] using tdd at [agreed seam]. Retain [exclusions].
Use Sol/medium for normal feature work, or Terra/medium for a small clear fix,
through supported host controls; report requested/effective settings.
Keep tests, review fixes and PR creation together. Run required checks and
automatic Standards/Spec review with the local override before completing PR.
Continue authorised stages, preserve current mode/gates, stop before merging.
```

### API example: scoped validation bug

```text
Continue the ticket's existing workflow mode and owner gates. In stock-flow-api,
fix only the validation mismatch named by the ticket, against its exact merged
rule ID and existing published request/error semantics. Read the local record
and flag conflicts before editing. Use Terra/medium if the cause is clear.
At the agreed HTTP seam, write a failing httptest through api.New for the
recorded invalid case and a valid boundary case. Preserve Problem Details,
error-code enum, status declarations and compatibility. Prove the guard matters
by removing it once, restore it, then run required checks and automatic review.
Keep review repairs and PR creation here. Do not decide unanswered password
limits, change contracts implicitly, or merge without explicit authorisation.
```

This is a process example, not an authorised bug or a new validation requirement. A real ticket must supply the rule, input, expected error/status and test seam.

### Manual review

```text
Continue [existing mode] as a read-only reviewer in a fresh session. Select
gpt-6.1-sol / medium using host settings and report effective-setting evidence.
Repo: [absolute API path]. Read AGENTS.md, docs/agents/code-review.md and
the installed code-review skill. Task/authority: [ticket URL and owner scope].
Base: [pinned SHA]. Reviewed commit: [SHA]. Commits: [exact list or none].
Diff command: [exact command]. Reviewed dirty/new paths and snapshot hashes:
[manifest location/details, or clean]. Exclude: [unrelated dirty paths or none].
Standards sources: [paths]. Spec/acceptance sources: [paths/issue URLs].
Run independent Standards and Spec reviews with explicit Sol/medium settings
when supported. If subagents remain unavailable, perform two separate passes
here and disclose reduced isolation; do not repeat this handoff.
Inspect relevant code/dependencies. No edits, commits, installs or merges.
Report each axis under 400 words, with file/hunk, source rule and finding.
Return results to [original ticket/PR/session reference; let implementer post].
```

### Resume

```text
Resume [existing Autopilot or Approval; preserve exact gates] in [API path].
Read AGENTS.md, workflow, session guide and applicable skills.
Task/spec/record: [links, merged commits and rule IDs]. Authority: [scope/gates].
Checkpoint: [branch, HEAD, staged/unstaged/untracked paths and exclusions].
Decisions/progress: [durable record/ticket/PR links]. Checks: [commands/results].
Review: [base, reviewed commit/snapshot, per-axis results, settings evidence].
Remaining: [specific work/blockers]. Next action: [concrete step].
Use [model/effort] via supported host settings; report effective evidence.
Refresh affected review after changes. Continue only authorised stages; preserve
requirements confirmation, Approval gates and explicit merge permission.
```

## Compare usage per completed ticket

Cached input reuses an eligible matching prompt prefix; new context adds input that must be processed. A long session can reuse context, but irrelevant history still competes for attention. Cache reuse depends on model/settings and matching boundaries, so neither staying nor restarting guarantees a cheaper run. See [OpenAI prompt caching](https://developers.openai.com/api/docs/guides/prompt-caching).

Higher reasoning effort can add latency and reasoning usage; reasoning tokens count as API output usage even when not shown as answer text. See [OpenAI reasoning documentation](https://developers.openai.com/api/docs/guides/reasoning). A fresh session can reduce irrelevant history while repeating instruction loading, exploration and tool setup. A short handoff helps only if it preserves enough evidence to avoid rediscovery and repairs.

Compare total usage per completed ticket: implementation, both reviewers, targeted re-review, repairs, repeated setup and tool work. Record model/effort, elapsed time, input/cache/output/reasoning usage where exposed, and completion quality; mark unavailable metrics unknown. Use actual API rates for API-key billing. Subscription allowances/credits are a separate system; API token prices do not estimate included subscription tasks. See [OpenAI subscription usage guidance](https://learn.chatgpt.com/docs/pricing). No fixed savings percentage is promised.
