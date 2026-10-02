# Agents: writing harnsy actions here

An action is a script you write and the human runs from the harnsy
dashboard. You propose it; the human enables it; only the enabled version
runs. The contract is `contract/contract.md` (1.0.0): read it before your
first action.

## Layout

- `actions/<name>/action.json`: the definition (`contract/action-definition.schema.json`) without `script.body`.
- Next to it the script: `run.sh` (bash, sh), `run.py`, `run.js`, `main.go` (lang go).
- A command action: `command.argv[0]` is a built file in the project (`bin/<name>`); its code in `cmd/<name>` + `internal/<name>`, the contract through `internal/action`.

## Rules

- One action, one job. `name` is its directory: `[a-z0-9-]`, up to 40.
- Parameters arrive as arguments, in order. Never build a shell string from them; quote every use (`"$1"`).
- Report: `::step <text>` while working, `::fail <text>` before a non-zero exit, a JSON result in `$ACTION_RESULT` (summary, up to 8 numbers, artifacts, links). Declare the figures you want kept as series in `numbers`.
- Scratch files and artifacts go to `$ACTION_DIR`, not the project.
- No secrets in output or the result. Ask for a variable by name in `env`; never `HARNSY_*`, `CCBUS_*`, `ACTION_*`.
- Mark what the human must know in `marks`: `restarts_node`, `deletes`, `network`. Set `heavy` for builds and long scans.
- lang go: standard library only (harnsy runs `go run <copy>.go` outside any module).
- A command is pinned by `argv[0]` alone: propose a built binary, never a wrapper script that calls other code.

## Before you propose

```sh
make check                       # gofmt, vet, tests, build, every action once
make try A=<name> P='-p k=v'     # run it as harnsy does; read the result check
go run ./cmd/propose -dry-run actions/<name>
```

Then `go run ./cmd/propose -from <your agent name> -project <project> actions/<name>`,
and tell your lead the action waits for the human. Do not ask the human in
chat to enable it; it is on their dashboard.

Skills with the steps: `.agents/skills/` (write-action, test-action, propose-action).
