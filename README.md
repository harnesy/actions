# harnsy action template

Write [harnsy](https://harnsy.dev) actions here: scripts an agent proposes
and you run from the harnsy dashboard with one button, with a status, a
summary and figures for every run.

Press **Use this template**, clone your copy into (or next to) the project
the actions are for, and start from one of the three examples.

| Example | Kind | What it shows |
|---|---|---|
| [`actions/disk`](actions/disk) | bash script | the smallest action: a parameter, `::step`, `::fail`, a result with figures |
| [`actions/todo-count`](actions/todo-count) | Go, one file | a Go script harnsy runs with `go run`: standard library only |
| [`actions/go-stats`](actions/go-stats) | command (Go binary) | a program built from this module (`cmd/` + `internal/`), with tests, using the contract helper `internal/action` |

## Quick start

```sh
make check                 # gofmt, go vet, tests, build, every action run once
make try A=disk            # run an action the way harnsy runs it
make try A=disk P='-p max_used=95'
go run ./cmd/propose -dry-run actions/disk            # the proposal, printed
go run ./cmd/propose -from <agent> -project <project> actions/disk
```

`make try` gives the action what harnsy gives it (the working directory,
a trimmed environment, `ACTION_RESULT`, `ACTION_DIR`, the time limit),
prints its output and its last `::step` and `::fail`, and checks the
result against the contract. A proposal waits on the dashboard until you
enable it; nothing runs before that.

## An action

Each action is a directory in `actions/`:

```
actions/<name>/
  action.json      the definition (contract/action-definition.schema.json), without script.body
  run.sh           the script, by lang: run.sh (bash, sh), run.py, run.js, main.go
```

`cmd/propose` puts the script into `script.body` when it sends the
proposal. A command action has no script file: its `command.argv[0]` is a
file in the project.

```json
{
  "name": "disk",
  "title": "Disk",
  "script": {"lang": "bash"},
  "params": [{"name": "max_used", "type": "int", "min": 1, "max": 100, "default": 90}],
  "timeout": "1m",
  "numbers": [{"key": "free_gb", "label": "Free, GB"}, {"key": "used_pct", "of": "max_used"}]
}
```

## The contract, in short

The full text is [`contract/contract.md`](contract/contract.md) (version
1.0.0, as harnsy serves it at `/docs/actions/contract.md`), with JSON
Schemas and an OpenAPI description next to it.

- **Arguments.** One per parameter, in the declared order (`$1`, `$2`, …);
  in a command, where `{name}` stands. Never a shell string.
  `enum` the value, `int` and `item` digits, `bool` `true`/`false`,
  `commit` a full 40-character hash.
- **Environment.** Only `PATH`, `HOME`, `LANG`, `USER`, `TMPDIR`, the
  names the action lists in `env`, and `ACTION_CONTRACT` (`1`),
  `ACTION_RUN_ID`, `ACTION_RESULT` (write the result there),
  `ACTION_DIR` (a directory of the run's own).
- **Markers.** A whole line on standard output: `::step <text>` (what
  runs now) and `::fail <text>` (why it failed, one line).
- **Exit.** 0 is ok, anything else failed; past `timeout` (1m to 2h,
  default 30m) the run is stopped.
- **Result.** JSON in `$ACTION_RESULT`, up to 64 KiB, every field optional:

```json
{
  "summary": "5.1 GB free of 456 GB (99% used)",
  "numbers": {"free_gb": 5.1, "used_pct": 99},
  "artifacts": [{"path": "dist/app.tar.gz", "label": "build"}],
  "links": [{"url": "https://example.com/report/7", "label": "report"}]
}
```

  At most 8 numbers (a number or a text of up to 40 characters), 20
  artifacts inside the project or `ACTION_DIR`, 10 `http(s)` links.

## What the human approves

You enable one version of an action; harnsy pins it by sha256 and every
change is a new version that waits for you again.

- A **script** (bash, sh, python, node, go): the whole body is pinned and
  shown to you before you enable it.
- A **command**: harnsy pins the file `argv[0]` only, not what it calls.
  Propose a **built binary** (`bin/go-stats`): every rebuild changes its
  sha256, so harnsy turns the action off and waits for you to enable it
  again. Do not propose a wrapper such as `run.sh` that calls `go run
  ./cmd/x`: the Go code under it would change without asking you.

## Go

- **One file** (`lang: go`): harnsy runs `go run <copy>.go` outside any
  module, so only the standard library can be imported. Copy the few
  helper lines from [`actions/todo-count/main.go`](actions/todo-count/main.go).
- **A program** (command): `cmd/<name>/main.go` with its logic in
  `internal/<name>` and tests next to it, the contract through
  [`internal/action`](internal/action) (`FromEnv`, `Step`, `Fail`,
  `Result.Check`, `Env.Write`). `make build` puts it in `bin/`.

## Checks

`make check` is what CI runs (`.github/workflows/ci.yml`), plus
[golangci-lint](https://golangci-lint.run) (`.golangci.yml`) and
[shellcheck](https://www.shellcheck.net) on the bash scripts.
`internal/def` tests that every action in `actions/` loads, passes the
checks harnsy makes and runs with its defaults.

## For agents

[`AGENTS.md`](AGENTS.md) is the guide for coding agents in this
repository. Skills to write, test and propose an action are in
`.agents/skills/` (Codex, OpenCode) and `.claude/skills/` (Claude Code).

## Contract versions

The contract has a semantic version. `ACTION_CONTRACT` holds its major
number; a script of major 1 keeps working on every 1.x harnsy. When the
contract adds something (a minor version), update `contract/` from your
node's `/docs/actions/` files.
