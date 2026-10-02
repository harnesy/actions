# harnsy actions contract, version 1.1.0

An **action** is a script an agent writes and the human runs from the
harnsy dashboard. This contract is what a script and a client of the HTTP
API may rely on. harnsy serves it at `/docs/actions/contract.md`, with
machine-readable parts next to it:

| File | What |
|---|---|
| [`action-definition.schema.json`](action-definition.schema.json) | an action's definition, as proposed (`POST /api/actions`) |
| [`action-result.schema.json`](action-result.schema.json) | the file a script writes to `$ACTION_RESULT` |
| [`action-run.schema.json`](action-run.schema.json) | a run, as `GET /api/action-runs/<id>` shows it |
| [`openapi.yaml`](openapi.yaml) | the HTTP API of actions (OpenAPI 3.1) |

## Versions

The contract has a semantic version, `MAJOR.MINOR.PATCH`. A script reads
the one it runs under in `$ACTION_CONTRACT` (the major number, e.g. `1`);
the API answers `"contract": "1.0.0"` in `GET /api/actions`.

- **PATCH**: wording, examples. Nothing a script or client sees changes.
- **MINOR**: something is added: an optional field, a variable, a marker,
  a parameter type, an endpoint. Scripts and clients of the same major keep
  working; they ignore what they do not know.
- **MAJOR**: something a script or client relies on changes or goes. harnsy
  then runs scripts of the previous major unchanged for at least one more
  release, and says so in its release notes.

| Version | What changed |
|---|---|
| 1.1.0 | `HARNSY_HOME` reaches the script; a command's file is shown (`GET /api/actions/{id}/file`) and enabled by its sha256 (`file_sha256`); Linux only (`os_unsupported`); a script body counts characters; links with a login or a secret are dropped; the result file must be a regular file |
| 1.0.0 | first version |

## 1. Script ↔ harnsy

### How it starts

- **Where it runs.** Linux nodes for now; elsewhere the API answers `501
  os_unsupported`.
- **Who.** The user harnsy runs as, on the node, with no terminal and no
  standard input.
- **Where.** The working directory is the action's `cwd` inside the
  project's directory.
- **What.** harnsy runs a copy of the enabled version: `bash <copy>`, `sh
  <copy>`, `python3 <copy>`, `node <copy>` or `go run <copy>.go`, by
  `script.lang`; or, for a `command`, the project's file itself. The human
  reads that file before enabling it and its sha256 is pinned; harnsy
  checks it again before every run. Only that file is pinned, not what it
  calls.
- **Arguments.** Each parameter is one argument, in the declared order
  (`$1`, `$2`, …); in a `command`, where `{name}` stands. Never a shell
  string.
  - `enum`: the value chosen.
  - `int`, `item`: decimal digits.
  - `bool`: `true` or `false`.
  - `commit`: the full 40-character hash the asked ref resolved to.
- **Environment.** Only `PATH`, `HOME`, `LANG`, `USER`, `TMPDIR`,
  `HARNSY_HOME` (where the node keeps its data, so the harnsy CLI reaches
  this node), the names listed in `env` (when set on the node), and:

| Variable | What |
|---|---|
| `ACTION_CONTRACT` | the contract's major version, `1` |
| `ACTION_RUN_ID` | the run's id |
| `ACTION_RESULT` | a file, empty at start: write the result there |
| `ACTION_DIR` | a directory of this run's own, for artifacts and scratch |

  Never another `HARNSY_*`, `CCBUS_*`, nor harnsy's tokens or keys. The
  script can still read what the node's user can.

### While it runs

- **Output.** Standard output and error both go to the run's log, up to
  20 MiB (then the log says it stops there; the script runs on). Secrets
  are cut out when the log is read.
- **Markers.** A whole line on standard output:
  - `::step <text>`: the step running now; the dashboard shows the latest.
  - `::fail <text>`: why it failed, in one line; the latest counts.

  Each text is cut to 200 characters.
- **Time.** Up to the action's `timeout` (1m to 2h, default 30m). Then, or
  when the human stops the run, harnsy sends SIGTERM to the script's whole
  process group, and SIGKILL 10 s later.

### When it ends

- **Exit code.** 0 means `ok`, anything else `failed`. A run stopped at its
  timeout is `timeout`; one the human stopped is `cancelled`; one whose
  runner went without a word is `lost`.
- **Result.** JSON in `$ACTION_RESULT` (a regular file), up to 64 KiB, by
  [`action-result.schema.json`](action-result.schema.json). Optional: an
  empty file is no result. A file that is not JSON, or too big, is
  ignored and the run says why in `fail`.
  - `summary`: one line (300 characters kept).
  - `numbers`: at most 8, a number or a short string (40 characters). Keys
    that the definition names in `numbers` are kept as a series.
  - `artifacts`: up to 20 files inside the project's directory or
    `$ACTION_DIR`; harnsy records each one's size and sha256.
  - `links`: up to 10 `http` or `https` URLs, shown, never opened by harnsy;
    one with a login (`user:pass@`) or a secret in it is dropped.

### Example (bash)

```bash
#!/usr/bin/env bash
set -euo pipefail
commit=$1
echo "::step build ${commit:0:7}"
go build -o "$ACTION_DIR/app" .
size=$(stat -c %s "$ACTION_DIR/app")
printf '{"summary":"built %s","numbers":{"size_b":%s},"artifacts":[{"path":"%s","label":"app"}]}\n' \
  "${commit:0:7}" "$size" "$ACTION_DIR/app" > "$ACTION_RESULT"
```

## 2. The definition

By [`action-definition.schema.json`](action-definition.schema.json). An
agent proposes it; the human enables one version, pinned by the sha256 of
the whole definition; any change is a new version that waits for the
human. `script.body` counts characters (65536 at most). Who may do what,
the bell and the errors: `docs/api.md` «Actions» (served at
`/docs/api.md`).

## 3. The HTTP API

By [`openapi.yaml`](openapi.yaml), on the node's private port
(`http://127.0.0.1:7788` by default) and, for agents, its agent port. A
write needs JSON (`Content-Type: application/json`); the human's moves
(enable, run, stop, the switches) are the dashboard's own requests.
