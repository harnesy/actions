---
name: propose-action
description: Propose a tested action of this repository to harnsy so the human can enable it on the dashboard. Use when an action is ready, or a changed one needs a new version.
---

# Propose a harnsy action

1. It passed skill `test-action`; for a command, `make build` ran and `argv[0]` is the fresh binary.
2. Look at what the human will see: `go run ./cmd/propose -dry-run actions/<name>`. The description says what it does and what it touches; `marks` are honest.
3. Send it: `go run ./cmd/propose -from <your agent name> -project <project> actions/<name>`
   (`-url` or `$HARNSY_URL` when the node is not on `http://127.0.0.1:7790`). You must hold a seat in a team of that project.
4. The answer is `proposed <name>: action <id>, version <n>`. Nothing runs yet: the human sees the script (or the binary's sha256), its parameters and marks, and enables it on the dashboard.
5. Report to your lead in one line: the action, its version, that it waits for the human. Do not ask the human in chat.
6. A change later (script, parameters, a rebuilt binary) is a new version: propose again; the enabled one keeps running until the human enables the new one.

Errors come as `HTTP <status> <code>: <text>`; read the code (`contract/openapi.yaml`, harnsy `docs/api.md` «Actions»).
