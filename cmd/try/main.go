// Command try runs an action of this repository the way harnsy runs it, and
// checks what it reports:
//
//	go run ./cmd/try [-p name=value]... actions/<name>
//
// Like harnsy: the working directory is the action's cwd in this
// repository; the environment holds only PATH, HOME, LANG, USER, TMPDIR, the
// names the action lists in env, and ACTION_CONTRACT, ACTION_RUN_ID,
// ACTION_RESULT, ACTION_DIR; a lang go script runs as a copy outside any
// module; the run stops at its timeout. Then it prints the last ::step and
// ::fail, the exit code and the result, checked against the contract.
//
// It exits non-zero when the result breaks the contract, and with -ok when
// the run itself did not end ok either.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/harnesy/action-template/internal/action"
	"github.com/harnesy/action-template/internal/def"
)

type params map[string]string

func (p params) String() string { return fmt.Sprint(map[string]string(p)) }
func (p params) Set(s string) error {
	k, v, ok := strings.Cut(s, "=")
	if !ok {
		return errors.New("name=value")
	}
	p[k] = v
	return nil
}

func main() {
	given := params{}
	flag.Var(given, "p", "a parameter's value, name=value (repeat)")
	mustOK := flag.Bool("ok", false, "fail unless the run itself ends ok")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/try [-ok] [-p name=value]... actions/<name>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	code, err := try(flag.Arg(0), given, *mustOK)
	if err != nil {
		fmt.Fprintln(os.Stderr, "try:", err)
	}
	os.Exit(code)
}

func try(dir string, given params, mustOK bool) (int, error) {
	d, err := def.Load(dir)
	if err != nil {
		return 2, err
	}
	repo, err := os.Getwd()
	if err != nil {
		return 2, err
	}
	cwd := filepath.Join(repo, d.Cwd)
	args, err := d.Args(given, repo)
	if err != nil {
		return 2, err
	}
	timeout, _ := d.TimeoutOr()

	runDir, err := os.MkdirTemp("", "action-run-")
	if err != nil {
		return 2, err
	}
	defer os.RemoveAll(runDir)
	result := filepath.Join(runDir, "result.json")
	if err := os.WriteFile(result, nil, 0o600); err != nil {
		return 2, err
	}
	artifacts := filepath.Join(runDir, "dir")
	os.Mkdir(artifacts, 0o700)

	argv, err := program(d, runDir, args)
	if err != nil {
		return 2, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = cwd
	cmd.Env = environment(d, map[string]string{
		"ACTION_CONTRACT": action.Contract, "ACTION_RUN_ID": "1",
		"ACTION_RESULT": result, "ACTION_DIR": artifacts,
	})
	cmd.Stdin = nil
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 2, err
	}
	fmt.Printf("── %s: %s\n", d.Name, strings.Join(argv, " "))
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return 2, err
	}
	var step, fail string
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := sc.Text()
		fmt.Println("│ " + line)
		if t, ok := strings.CutPrefix(line, "::step "); ok {
			step = t
		}
		if t, ok := strings.CutPrefix(line, "::fail "); ok {
			fail = t
		}
	}
	waitErr := cmd.Wait()
	state := "ok"
	switch {
	case ctx.Err() != nil:
		state = "timeout"
	case waitErr != nil:
		state = "failed"
	}
	fmt.Printf("── state %s after %s (exit %d)\n", state, time.Since(start).Round(time.Millisecond), cmd.ProcessState.ExitCode())
	if step != "" {
		fmt.Println("   last step:", step)
	}
	if fail != "" {
		fmt.Println("   fail:", fail)
	} else if state != "ok" {
		fmt.Println("   fail: (none given: write `::fail <why>` before exiting non-zero)")
	}

	problems := checkResult(d, result, repo, artifacts)
	for _, p := range problems {
		fmt.Println("   ✗ result:", p)
	}
	switch {
	case len(problems) > 0:
		return 1, errors.New("the result breaks the contract")
	case mustOK && state != "ok":
		return 1, fmt.Errorf("the run ended %s", state)
	}
	return 0, nil
}

// program is what harnsy starts: the interpreter and a copy of the script,
// or the command's own file.
func program(d *def.Definition, runDir string, args []string) ([]string, error) {
	if d.Command != nil {
		bin, err := filepath.Abs(d.Command.Argv[0])
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(bin); err != nil {
			return nil, fmt.Errorf("%s: %w (build it first: make build)", d.Command.Argv[0], err)
		}
		return append([]string{bin}, args...), nil
	}
	copyPath := filepath.Join(runDir, "script")
	interp := map[string]string{"bash": "bash", "sh": "sh", "python": "python3", "node": "node"}
	if d.Script.Lang == "go" {
		// A copy in a directory of its own: no go.mod around it, as on harnsy.
		copyPath = filepath.Join(runDir, "go", "action.go")
		os.Mkdir(filepath.Dir(copyPath), 0o700)
		if err := os.WriteFile(copyPath, []byte(d.Script.Body), 0o600); err != nil {
			return nil, err
		}
		return append([]string{"go", "run", copyPath}, args...), nil
	}
	if err := os.WriteFile(copyPath, []byte(d.Script.Body), 0o600); err != nil {
		return nil, err
	}
	return append([]string{interp[d.Script.Lang], copyPath}, args...), nil
}

// environment is the little harnsy passes on.
func environment(d *def.Definition, own map[string]string) []string {
	var env []string
	for _, k := range append([]string{"PATH", "HOME", "LANG", "USER", "TMPDIR"}, d.Env...) {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	if os.Getenv("GOCACHE") == "" && d.Script != nil && d.Script.Lang == "go" {
		// go run needs a build cache; harnsy's HOME has one, a CI's may not.
		if c, err := os.UserCacheDir(); err == nil {
			env = append(env, "GOCACHE="+filepath.Join(c, "go-build"))
		}
	}
	for k, v := range own {
		env = append(env, k+"="+v)
	}
	return env
}

func checkResult(d *def.Definition, path, repo, runDir string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return []string{err.Error()}
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		fmt.Println("   result: none (an empty file is allowed)")
		return nil
	}
	if len(raw) > action.MaxResultBytes {
		return []string{fmt.Sprintf("%d bytes, at most %d", len(raw), action.MaxResultBytes)}
	}
	var r action.Result
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return []string{"not the result JSON: " + err.Error()}
	}
	if _, err := dec.Token(); err != io.EOF {
		return []string{"more than one JSON value"}
	}
	var problems []string
	if err := r.Check(); err != nil {
		problems = append(problems, strings.Split(err.Error(), "\n")...)
	}
	declared := map[string]bool{}
	for _, n := range d.Numbers {
		declared[n.Key] = true
	}
	fmt.Println("   summary:", r.Summary)
	for k, n := range r.Numbers {
		v, _ := json.Marshal(n)
		note := ""
		if len(d.Numbers) > 0 && !declared[k] {
			note = "  (not in the definition's numbers: shown, not kept as a series)"
		}
		fmt.Printf("   %s = %s%s\n", k, v, note)
	}
	for i, a := range r.Artifacts {
		p := a.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(repo, d.Cwd, p)
		}
		if !inside(p, repo) && !inside(p, runDir) {
			problems = append(problems, fmt.Sprintf("artifacts[%d] %s: outside the project and ACTION_DIR", i, a.Path))
		} else if _, err := os.Stat(p); err != nil {
			problems = append(problems, fmt.Sprintf("artifacts[%d] %s: %v", i, a.Path, err))
		}
	}
	return problems
}

func inside(p, root string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
