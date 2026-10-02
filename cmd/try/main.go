// Command try runs an action of this repository the way harnsy runs it, and
// checks what it reports:
//
//	go run ./cmd/try [-ok] [-p name=value]... actions/<name>
//
// Like harnsy: the working directory is the action's cwd in the project (this
// repository); the environment holds only PATH, HOME, LANG, USER, TMPDIR,
// HARNSY_HOME, the names the action lists in env, and ACTION_CONTRACT,
// ACTION_RUN_ID, ACTION_RESULT, ACTION_DIR; a script runs as a copy (lang
// go: `go run <copy>.go` from the working directory, so the project's go.mod
// applies); a command runs argv[0] found under that directory; markers count
// on standard output and error; at the timeout the whole process group gets
// SIGTERM, then SIGKILL 10 s later, and when the script exits what it left
// running is killed.
//
// Then it prints the last ::step and ::fail, the exit code and the result,
// and checks the result against the contract. It exits 1 when the result
// breaks the contract or a lang go script imports beyond the standard
// library, and with -ok also when the run did not end ok.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

// limit is the run's time limit; a test shortens it.
var limit = func(d *def.Definition) time.Duration {
	t, _ := d.TimeoutOr()
	return t
}

// killGrace is how long a stopped run gets between SIGTERM and SIGKILL.
var killGrace = 10 * time.Second

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
	repo, err := os.Getwd()
	if err == nil {
		var code int
		code, err = try(os.Stdout, repo, flag.Arg(0), given, *mustOK)
		if err == nil {
			os.Exit(code)
		}
	}
	fmt.Fprintln(os.Stderr, "try:", err)
	os.Exit(2)
}

// outcome is what try saw of one run.
type outcome struct {
	state      string // ok, failed, timeout
	exit       int
	step, fail string
	problems   []string
}

func try(w io.Writer, project, dir string, given params, mustOK bool) (int, error) {
	o, err := run(w, project, dir, given)
	if err != nil {
		return 2, err
	}
	for _, p := range o.problems {
		fmt.Fprintln(w, "   ✗", p)
	}
	switch {
	case len(o.problems) > 0:
		return 1, nil
	case mustOK && o.state != "ok":
		fmt.Fprintf(w, "   ✗ the run ended %s\n", o.state)
		return 1, nil
	}
	return 0, nil
}

// lockedWriter lets the run's stdout reader and its stderr copy share w.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func run(out io.Writer, project, dir string, given params) (*outcome, error) {
	w := &lockedWriter{w: out}
	d, err := def.Load(dir)
	if err != nil {
		return nil, err
	}
	workDir, err := d.WorkDir(project)
	if err != nil {
		return nil, err
	}
	args, err := d.Args(given, workDir)
	if err != nil {
		return nil, err
	}
	o := &outcome{}
	if d.Script != nil && d.Script.Lang == "go" {
		o.problems = append(o.problems, goImports(d.Script.Body)...)
	}

	runDir, err := os.MkdirTemp("", "action-run-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(runDir)
	result := filepath.Join(runDir, "result.json")
	if err := os.WriteFile(result, nil, 0o600); err != nil {
		return nil, err
	}
	artifacts := filepath.Join(runDir, "dir")
	if err := os.Mkdir(artifacts, 0o700); err != nil {
		return nil, err
	}
	argv, err := program(d, project, runDir, args)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = workDir
	cmd.Env = environment(d, map[string]string{
		"ACTION_CONTRACT": action.Contract, "ACTION_RUN_ID": "1",
		"ACTION_RESULT": result, "ACTION_DIR": artifacts,
	})
	setGroup(cmd)
	// One pipe for stdout and stderr, as harnsy reads them: markers count on
	// either. Our write end closes after the start, so the read ends when
	// the last process holding it does.
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = pw, pw
	fmt.Fprintf(w, "── %s: %s\n", d.Name, strings.Join(argv, " "))
	start := time.Now()
	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return nil, err
	}
	pw.Close()
	read := make(chan struct{})
	go func() { readOutput(w, pr, o); close(read) }()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()

	timeout := time.NewTimer(limit(d))
	defer timeout.Stop()
	var kill <-chan time.Time
	var waitErr error
wait:
	for {
		select {
		case waitErr = <-waited:
			break wait
		case <-timeout.C:
			o.state = "timeout"
			signalGroup(cmd.Process.Pid, false)
			kill = time.After(killGrace)
		case <-kill:
			signalGroup(cmd.Process.Pid, true)
			kill = nil
		}
	}
	// The script is done: what it left running goes too, as on harnsy; its
	// output gets a moment more to arrive.
	signalGroup(cmd.Process.Pid, true)
	select {
	case <-read:
	case <-time.After(2 * time.Second):
		pr.Close()
		<-read
	}
	pr.Close()
	o.exit = cmd.ProcessState.ExitCode()
	if o.state == "" {
		o.state = "ok"
		if waitErr != nil || o.exit != 0 {
			o.state = "failed"
		}
	}
	fmt.Fprintf(w, "── state %s after %s (exit %d)\n", o.state, time.Since(start).Round(time.Millisecond), o.exit)
	if o.step != "" {
		fmt.Fprintln(w, "   last step:", o.step)
	}
	if o.fail != "" {
		fmt.Fprintln(w, "   fail:", o.fail)
	} else if o.state == "failed" {
		fmt.Fprintln(w, "   fail: (none given: write `::fail <why>` before exiting non-zero)")
	}
	o.problems = append(o.problems, checkResult(w, d, result, project, workDir, artifacts)...)
	return o, nil
}

// readOutput copies the run's output to w and follows the markers: a line
// read in pieces of at most 64 KiB, as harnsy does, so a long line never
// stops the reading; only a piece that starts a line can be a marker.
func readOutput(w io.Writer, r io.Reader, o *outcome) {
	br := bufio.NewReaderSize(r, 64<<10)
	whole := true
	for {
		piece, err := br.ReadSlice('\n')
		if len(piece) > 0 {
			if whole {
				fmt.Fprint(w, "│ ")
			}
			_, _ = w.Write(piece)
			if whole {
				t := strings.TrimRight(string(piece), "\r\n")
				if s, ok := strings.CutPrefix(t, "::step "); ok {
					o.step = s
				}
				if s, ok := strings.CutPrefix(t, "::fail "); ok {
					o.fail = s
				}
			}
		}
		switch {
		case err == bufio.ErrBufferFull:
			whole = false
		case err != nil:
			if !whole || len(piece) > 0 && piece[len(piece)-1] != '\n' {
				fmt.Fprintln(w)
			}
			return
		default:
			whole = true
		}
	}
}

// program is what harnsy starts: the interpreter and a copy of the script,
// or the command's own file.
func program(d *def.Definition, project, runDir string, args []string) ([]string, error) {
	if d.Command != nil {
		bin, err := d.CommandFile(project)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(bin); err != nil {
			return nil, fmt.Errorf("%s: %w (build it first: make build)", d.Command.Argv[0], err)
		}
		return append([]string{bin}, args...), nil
	}
	ext := map[string]string{"bash": ".sh", "sh": ".sh", "python": ".py", "node": ".js", "go": ".go"}[d.Script.Lang]
	copyPath := filepath.Join(runDir, "script"+ext)
	if err := os.WriteFile(copyPath, []byte(d.Script.Body), 0o600); err != nil {
		return nil, err
	}
	switch d.Script.Lang {
	case "go":
		return append([]string{"go", "run", copyPath}, args...), nil
	case "python":
		return append([]string{"python3", copyPath}, args...), nil
	}
	return append([]string{d.Script.Lang, copyPath}, args...), nil
}

// goImports names every import of a lang go script beyond the standard
// library: harnsy pins the script, not the packages it imports, and runs it
// in the project's module, where such an import would build code the human
// never saw.
func goImports(body string) []string {
	f, err := parser.ParseFile(token.NewFileSet(), "action.go", body, parser.ImportsOnly)
	if err != nil {
		return []string{"lang go: " + err.Error()}
	}
	if len(f.Imports) == 0 {
		return nil
	}
	std, err := stdPackages()
	if err != nil {
		return []string{"lang go: the standard library's packages could not be listed (go list std): " + err.Error()}
	}
	var out []string
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if !std[path] {
			out = append(out, fmt.Sprintf("lang go imports %q: keep to the standard library (harnsy pins the script, not what it imports)", path))
		}
	}
	return out
}

// stdPackages lists the standard library of the go on PATH, once.
func stdPackages() (map[string]bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "go", "list", "-e", "std").Output()
	if err != nil {
		return nil, err
	}
	std := map[string]bool{}
	for _, p := range strings.Fields(string(out)) {
		std[p] = true
	}
	return std, nil
}

// environment is the little harnsy passes on.
func environment(d *def.Definition, own map[string]string) []string {
	var env []string
	for _, k := range append([]string{"PATH", "HOME", "LANG", "USER", "TMPDIR", "HARNSY_HOME"}, d.Env...) {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	for k, v := range own {
		env = append(env, k+"="+v)
	}
	return env
}

// checkResult reads the result as harnsy does (unknown fields ignored) and
// names what breaks the contract.
func checkResult(w io.Writer, d *def.Definition, path, project, workDir, runDir string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return []string{err.Error()}
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		fmt.Fprintln(w, "   result: none (an empty file is allowed)")
		return nil
	}
	if len(raw) > action.MaxResultBytes {
		return []string{fmt.Sprintf("result: %d bytes, at most %d", len(raw), action.MaxResultBytes)}
	}
	var r action.Result
	if err := json.Unmarshal(raw, &r); err != nil {
		return []string{"result: not the result JSON: " + err.Error()}
	}
	var problems []string
	if err := r.Check(); err != nil {
		for _, p := range strings.Split(err.Error(), "\n") {
			problems = append(problems, "result: "+p)
		}
	}
	kept := map[string]bool{}
	for _, n := range d.Numbers {
		kept[n.Key] = true
		if n.Of != "" {
			kept[n.Of] = true
		}
	}
	fmt.Fprintln(w, "   summary:", r.Summary)
	for k, n := range r.Numbers {
		v, _ := json.Marshal(n)
		note := ""
		if !kept[k] {
			note = "  (not in the definition's numbers: shown on the run, not kept as a series)"
		}
		fmt.Fprintf(w, "   %s = %s%s\n", k, v, note)
	}
	root, _ := filepath.EvalSymlinks(project)
	for i, a := range r.Artifacts {
		p := a.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(workDir, p)
		}
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		run, _ := filepath.EvalSymlinks(runDir)
		if !inside(p, root) && !inside(p, run) {
			problems = append(problems, fmt.Sprintf("result: artifacts[%d] %s: outside the project and ACTION_DIR", i, a.Path))
		} else if _, err := os.Stat(p); err != nil {
			problems = append(problems, fmt.Sprintf("result: artifacts[%d] %s: %v", i, a.Path, err))
		}
	}
	return problems
}

func inside(p, root string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
