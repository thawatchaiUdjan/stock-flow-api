# Coding standards

`code-review` checks every change against this file (its Standards axis), on top of a fixed baseline of code smells. A rule written here overrides that baseline. Skip anything a tool already enforces; name the tool instead.

<!-- FILL: complete the sections below when the stack is chosen. Leave a section out rather than writing a rule nobody follows. -->

## Formatting and linting

<!-- FILL: the formatter and linter, and the command that runs them. -->

## Naming

- Domain concepts use the terms in `stock-flow-context`'s `GLOSSARY.md`, never their `_Avoid_` synonyms.

## Structure

<!-- FILL: how the code is laid out, e.g. modules by business domain, which layer may depend on which. -->

## Errors

<!-- FILL: how errors are represented and wrapped, and how they reach the client. -->
- Every error the API can return has a code in the published error-code list.

## Comments

- A comment states *why*, or the constraint the code obeys, never *what* the next line does.
- Code that enforces a recorded rule cites it: `<module> R3` or `context ADR 0007`.

## Tests

- Tests assert observable behaviour through the public interface, not internals.
- Assert the actual value, not just "no error" or a status code.
- A test is never weakened, skipped or deleted to get green; challenge it explicitly instead.
