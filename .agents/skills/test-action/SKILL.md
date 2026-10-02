---
name: test-action
description: Check a harnsy action of this repository before proposing it - run it the way harnsy runs it and check its markers, exit code and result against the contract. Use after writing or changing an action, or when an action fails on the dashboard.
---

# Test a harnsy action

1. `make check`: gofmt, go vet, the tests (they load every `actions/*/action.json` and run its defaults through the checks harnsy makes), the build, and every action once through `cmd/try`. Fix all of it.
2. Run yours with each meaningful parameter value:
   `make try A=<name>` and `make try A=<name> P='-p <param>=<value>'`.
   Read the output:
   - `state ok` when it should succeed; when it should fail, `state failed` with a `fail:` line that says why in words the human understands;
   - `last step` names the step it was in;
   - no `✗ result:` lines; every figure you want as a series is declared in `numbers` (else `try` notes it);
   - artifacts exist and lie in the project or `ACTION_DIR`.
3. `make try` exits non-zero only when the result breaks the contract; add `-ok` (`go run ./cmd/try -ok actions/<name>`) to fail on a failed run too.
4. Bash: `shellcheck actions/<name>/*.sh`. Go: `golangci-lint run ./...` when installed (CI runs both).
5. For a Go command: unit-test its logic in `internal/<name>`; `cmd/` stays thin.
