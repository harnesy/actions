package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/harnesy/action-template/internal/def"
)

// project makes a project with one action: action.json and its files
// (relative to the project).
func project(t *testing.T, def string, files map[string]string) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = filepath.Join(root, "actions", "x")
	files["actions/x/action.json"] = def
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root, dir
}

func TestTryScriptResult(t *testing.T) {
	root, dir := project(t, `{"name":"x","script":{"lang":"bash"},"params":[{"name":"n","type":"int","default":2}],"numbers":[{"key":"n"}]}`,
		map[string]string{"actions/x/run.sh": `echo "::step working on $1"
printf '{"summary":"n is %s","numbers":{"n":%s},"extra":"ignored by harnsy"}' "$1" "$1" > "$ACTION_RESULT"
`})
	var out bytes.Buffer
	code, err := try(&out, root, dir, params{"n": "5"}, true)
	if err != nil || code != 0 {
		t.Fatalf("code %d %v\n%s", code, err, out.String())
	}
	for _, want := range []string{"state ok", "last step: working on 5", "summary: n is 5", "n = 5"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("no %q in\n%s", want, out.String())
		}
	}
}

// At the timeout the whole group stops, a child holding the output too, and
// try returns instead of waiting for it.
func TestTryTimeoutStopsTheGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups")
	}
	saved, savedGrace := limit, killGrace
	limit, killGrace = func(*def.Definition) time.Duration { return 300 * time.Millisecond }, 300*time.Millisecond
	t.Cleanup(func() { limit, killGrace = saved, savedGrace })
	root, dir := project(t, `{"name":"x","script":{"lang":"bash"}}`,
		map[string]string{"actions/x/run.sh": "echo '::step waiting'\nsleep 30 &\nsleep 30\n"})
	var out bytes.Buffer
	start := time.Now()
	code, err := try(&out, root, dir, params{}, true)
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("try waited %s", took)
	}
	if err != nil || code != 1 || !strings.Contains(out.String(), "state timeout") {
		t.Fatalf("code %d %v\n%s", code, err, out.String())
	}
}

func TestTryGoImportsBeyondStd(t *testing.T) {
	root, dir := project(t, `{"name":"x","script":{"lang":"go"}}`,
		map[string]string{"actions/x/main.go": "package main\n\nimport (\n\t\"fmt\"\n\t\"example.com/project/lib\"\n)\n\nfunc main() { fmt.Println(lib.X) }\n"})
	var out bytes.Buffer
	code, _ := try(&out, root, dir, params{}, false)
	if code != 1 || !strings.Contains(out.String(), `imports "example.com/project/lib"`) {
		t.Fatalf("code %d\n%s", code, out.String())
	}
}

// A command's argv[0] is found under the action's cwd, as harnsy finds it;
// an artifact outside the project and ACTION_DIR breaks the contract.
func TestTryCommandUnderCwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script as the command")
	}
	root, dir := project(t, `{"name":"x","cwd":"tools","command":{"argv":["bin/x","{what}"]},"params":[{"name":"what","type":"enum","values":["in","out"],"default":"in"}]}`,
		map[string]string{"tools/bin/x": `#!/bin/sh
if [ "$1" = in ]; then f="$ACTION_DIR/a.txt"; else f=/etc/hostname; fi
echo hi > "$ACTION_DIR/a.txt"
printf '{"artifacts":[{"path":"%s"}]}' "$f" > "$ACTION_RESULT"
`})
	var out bytes.Buffer
	if code, err := try(&out, root, dir, params{}, true); err != nil || code != 0 {
		t.Fatalf("code %d %v\n%s", code, err, out.String())
	}
	out.Reset()
	if code, _ := try(&out, root, dir, params{"what": "out"}, false); code != 1 || !strings.Contains(out.String(), "outside the project") {
		t.Fatalf("an artifact outside passed: %d\n%s", code, out.String())
	}
}
