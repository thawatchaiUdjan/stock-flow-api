# Testing: what runs where

Tests come in two layers, and they run in different places, because waiting on slow tests is where agents lose the most time.

| Layer | What it touches | Speed | Runs locally | Runs in CI |
| --- | --- | --- | --- | --- |
| **Fast** | Business logic through in-memory fakes of the ports (store, mailer, clock) | seconds for the whole layer | **all of it**, as often as you like | all of it |
| **Slow** | A real database or other real dependency | seconds to minutes per package | **no** (see below) | all of it, every pipeline; nobody waits for it |

<!-- FILL: the commands for each layer, once the stack is chosen: run the fast layer, run one slow test by name, run everything. -->

## Overrides

- **This overrides `implement`'s "the full test suite once at the end".** Locally, "the full suite" means the whole fast layer. The slow layer runs only in CI, and **nobody waits for it**: a pull request can be merged before its pipeline finishes. When the user reports a red pipeline, the fix comes back as work (`template-context`'s `docs/workflow.md`).
- You still **write** slow tests where only the real dependency can show the behaviour (below); you just don't wait for them. Write them so they pass first time on a clean environment: unique data, no ordering assumptions.
- **Until CI exists, nothing runs the slow layer.** In that case, run it locally once before opening the pull request, and say in it that you did.

## Keep the fakes honest

A fast layer is only as good as its fakes. A fake more permissive than the real dependency hides exactly the defects a real one would catch (a missing uniqueness or concurrency guard, an ordering that only holds by accident).

- Write **one contract suite per port** and run it against both the fake and the real implementation. When the real one fails it, the fake was lying.
- Behaviour that only a real dependency can show (locking, races, ordering, transactions) gets a slow test, and the fake must not pretend to provide it.

## Keep the slow layer cheap

- **One shared test database, not one container per test package.** Migrate a template database once and clone it per package; many containers at once overload the machine.
- **Cheap per-test setup.** Roll back a transaction per test rather than truncating every table; use the cheapest settings of deliberately slow algorithms (password hashing) in tests. Per-test setup, more than container start-up, decides how long the slow layer takes.
- **Unique data per test** (generated names, keys), so tests never collide on a shared database.

## Running long tests as an agent

- Start a test run expected to exceed a minute in the background, and read its result when it finishes, rather than polling it in a loop.
- Set the command timeout above the slowest package; a run killed by a timeout proves nothing and has to be repeated.
- At most two test runs at once on one machine: they finish sooner one after another than all together.
