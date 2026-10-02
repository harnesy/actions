---
name: write-action
description: Write a new harnsy action in this repository (a bash, python, node or single-file Go script, or a Go command), by the actions contract. Use when asked to add, create or change an action, a dashboard button, or a script the human runs from harnsy.
---

# Write a harnsy action

1. Read `contract/contract.md` once per session, and `AGENTS.md` "Rules".
2. Pick the kind:
   - a few shell steps → `script`, lang `bash` (copy `actions/disk`);
   - Go logic, standard library only → `script`, lang `go`, one `main.go` (copy `actions/todo-count`, keep its inline helpers; it runs in the project's module, but an import of project code is not pinned, so do not);
   - a real program with packages and tests → `command` with a built binary (copy `actions/go-stats`, `cmd/go-stats`, `internal/gostats`; use `internal/action`; add it to `make build`).
3. Create `actions/<name>/action.json`: `name` = the directory; `title`; a `description` the human reads before enabling; `params` with types and defaults; `timeout`; `numbers` for the figures to keep; `marks` and `heavy` when true. No `script.body`: the script file holds it.
4. Write the script:
   - arguments in the declared order, validated, always quoted; in a command, a parameter goes in as a whole argument `{name}` (`--x={name}` stays literal: write `--x`, `{name}`);
   - `::step` before each long step, `::fail <why>` then a non-zero exit on failure;
   - the result JSON into `$ACTION_RESULT` (one line summary, figures as numbers);
   - scratch and artifacts under `$ACTION_DIR`.
5. Test it: skill `test-action`.
