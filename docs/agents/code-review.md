# Local code-review override

Read this before every `code-review`, including review invoked by `/implement`. This is the maintained repository override for the [installed skill](../../.agents/skills/code-review/SKILL.md). It takes precedence over conflicting defaults there; retain its Standards/Spec axes and smell baseline. [Skill maintenance](../../README.md#updating) keeps overrides outside installed files so reinstalling cannot erase them.

## Trigger and settings

Run review automatically after implementation and required checks, before PR completion. Unless the owner explicitly overrides settings, request **each** Standards and Spec reviewer with `model: "gpt-6.1-sol"` and `reasoning_effort: "medium"`. For Codex `spawn_agent`, set `fork_turns: "none"` so explicit settings are supported and implementation history is not inherited. Use equivalent supported model/effort and fresh-context controls in other hosts. Reviewers are read-only; the implementer alone applies fixes.

Inspect actual tool capabilities first. Record the requested settings, accepted spawn parameters and any effective-setting metadata returned. An accepted request confirms the request, not necessarily the effective backend configuration; if tooling does not expose that, report effective settings as unconfirmed. Never silently inherit settings or substitute another model for review.

If explicit settings cannot be requested, report the limitation and supply the filled [manual-review prompt](../session-guide.md#manual-review) for a fresh Sol/medium session. If subagents remain unavailable in that session, perform the two axes as separate read-only passes and disclose reduced isolation. Do not send the same handoff again. Keep pending review visible until the fallback is completed; do not claim independent review passed.

## Pin the actual state

1. Resolve the supplied comparison point once. By default, use the merge-base with `main` and record its full SHA. Record the reviewed `HEAD` SHA and commit list (`git log <base>..HEAD --oneline`). Confirm the base resolves and the in-scope diff is nonempty. Do not let moving refs change a review underneath its reviewers.
2. Inspect `git status --short`, `git diff --name-status`, `git diff --cached --name-status` and `git ls-files --others --exclude-standard`. Identify task paths and unrelated dirty paths explicitly. Ask only if ownership is unclear. Never stage, restore or clean unrelated work.
3. For a clean committed review, pass `git diff <base-sha> <reviewed-head-sha> -- <scope paths>`. For pre-commit review, include the net tracked diff from the base to the working tree (`git diff <base-sha> -- <scope paths>`), the staged diff (`git diff --cached <base-sha> -- <scope paths>`) and every in-scope untracked new implementation file by content. A `base...HEAD` diff alone omits them. Review staged and working versions where they differ, so cancellation between staged and unstaged edits cannot hide a planned commit.
4. Identify the exact dirty state with a manifest of paths, staged blob IDs (`git ls-files --stage -- <scope paths>`), working/untracked content hashes (`git hash-object -- <each path>`) and deletions. Save the manifest and diff artifacts in Git's local metadata directory or another non-committed scratch location; include staged file contents if needed. Exclude unrelated dirty files and secrets. Freeze edits until both reviewers finish; alternatively provide an immutable snapshot. Reviewers may inspect relevant unchanged code and dependencies read-only.
5. Before accepting reports and again before PR completion, compare current state with the captured state. A commit containing exactly reviewed content can be mapped to that snapshot; record the mapping. New or changed content needs affected-axis re-review. An empty committed diff is not an empty review if in-scope staged, unstaged or new files exist.

## Focused dispatch

Send each reviewer only the repository path/instructions, task and owner scope, pinned base and reviewed commit, exact diff commands, commit list, snapshot manifest/new-file contents or locations, exclusions and that axis's sources. Do not send the whole implementation chat, the implementer's conclusions or the other reviewer's findings. Let reviewers inspect relevant code/dependencies themselves. Run both axes independently in parallel when supported.

Standards sources include `AGENTS.md`, `CODING_STANDARDS.md`, applicable workflow/testing/domain guidance and any `CONTRIBUTING.md`. Include the installed skill's full smell baseline in the dispatch, or explicitly direct the reviewer to read that baseline at its local path. Repo standards override smell heuristics. Distinguish hard breaches from judgement calls and skip tooling-enforced formatting/lint findings.

Spec sources come from ticket references in commits, a supplied spec/acceptance path, matching local records, or the owner's acceptance conditions captured on the API ticket. Fetch full ticket bodies/comments and linked specs as [tracker guidance](issue-tracker.md) requires. If none exists, ask where it is; skip Spec only when no spec/acceptance source exists, and state why. Missing commit issue references alone do not justify skipping available acceptance conditions.

Use this dispatch shape, filling all fields:

```text
Read-only [Standards or Spec] review. Repo: [absolute path]. Read AGENTS.md
and docs/agents/code-review.md. Task/owner scope: [ticket and authority].
Base SHA: [sha]. Reviewed HEAD: [sha]. Commit list: [list or none].
Diff commands: [exact commands]. Snapshot: [manifest/artifact paths, new files,
staged differences and hashes, or clean]. Exclude unrelated paths: [list or none].
Sources: [axis-specific paths/issue URLs; Standards also reads full smell
baseline in .agents/skills/code-review/SKILL.md]. Inspect relevant dependencies.
Do not edit, commit, install, message externally or merge.
Standards: cite file/hunk and documented rule for each breach; separate smell
judgement calls. Spec: cite missing/partial, unrequested or incorrect behaviour
against exact acceptance/rule text. Report only this axis, under 400 words,
including reviewed state and any evidence limitations. Do not infer effective
settings from this prompt; report tooling evidence or unconfirmed.
```

## Repairs and completion

Keep concise reports under separate Standards and Spec headings, with counts and worst issue per axis. Apply fixes in the implementation session, run affected checks, then return affected hunks and rules to the relevant reviewer. If the reviewer is no longer available, dispatch a fresh reviewer with the same explicit settings and focused context. Avoid full repeated reviews unless scope changes. Use the same settings/fallback rules for repair review and report requested/effective evidence.

Record the final reviewed commit/snapshot, excluded paths, findings fixed or retained with reasons, checks and settings evidence in the ticket/PR. Preserve existing security, record confirmation, contract publication and verification rules. Review does not authorise merging. Escalate model/effort only for a specific stated reason or owner override; do not create an agent solely to format a short PR body.
