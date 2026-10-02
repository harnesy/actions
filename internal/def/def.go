// Package def reads an action of this repository: actions/<name>/action.json
// (a definition by contract/action-definition.schema.json, without
// script.body) and the script next to it, and turns parameter values into
// the arguments harnsy would pass.
package def

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Definition is an action's definition (contract 1.1.0).
type Definition struct {
	From        string    `json:"from,omitempty"`
	Project     string    `json:"project,omitempty"`
	Name        string    `json:"name"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	Script      *Script   `json:"script,omitempty"`
	Command     *Command  `json:"command,omitempty"`
	Cwd         string    `json:"cwd,omitempty"`
	Params      []Param   `json:"params,omitempty"`
	Env         []string  `json:"env,omitempty"`
	Timeout     string    `json:"timeout,omitempty"`
	Heavy       bool      `json:"heavy,omitempty"`
	Marks       []string  `json:"marks,omitempty"`
	Pin         bool      `json:"pin,omitempty"`
	Numbers     []NumberD `json:"numbers,omitempty"`

	// Dir is the action's directory in this repository; not sent.
	Dir string `json:"-"`
}

type Script struct {
	Lang string `json:"lang"`
	Body string `json:"body,omitempty"`
}

type Command struct {
	Argv []string `json:"argv"`
}

type Param struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Values  []string `json:"values,omitempty"`
	Min     *int     `json:"min,omitempty"`
	Max     *int     `json:"max,omitempty"`
	Default any      `json:"default,omitempty"`
	Label   string   `json:"label,omitempty"`
}

type NumberD struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
	Of    string `json:"of,omitempty"`
}

// ScriptFiles is the file next to action.json that holds a script, by lang.
var ScriptFiles = map[string]string{"bash": "run.sh", "sh": "run.sh", "python": "run.py", "node": "run.js", "go": "main.go"}

var (
	namePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	paramPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	envPattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	reservedEnv  = regexp.MustCompile(`(?i)^(harnsy|ccbus|action)_`)
)

// Load reads actions/<name> (or a directory path) and fills script.body
// from the script file.
func Load(dir string) (*Definition, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "action.json"))
	if err != nil {
		return nil, err
	}
	var d Definition
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("%s/action.json: %w", dir, err)
	}
	d.Dir = dir
	if d.Script != nil && d.Script.Body == "" {
		file, ok := ScriptFiles[d.Script.Lang]
		if !ok {
			return nil, fmt.Errorf("script.lang %q: bash, sh, python, node or go", d.Script.Lang)
		}
		body, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return nil, fmt.Errorf("the script of %s: %w", d.Name, err)
		}
		d.Script.Body = string(body)
	}
	return &d, d.Check()
}

// Check finds what harnsy refuses when the definition is proposed (400
// invalid): the rules of contract/action-definition.schema.json and of
// harnsy's own check, each with its reason.
func (d *Definition) Check() error {
	var errs []error
	bad := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	if !namePattern.MatchString(strings.TrimSpace(d.Name)) {
		bad("name %q: a-z, 0-9 and -, 1 to 40, starting with a letter or digit", d.Name)
	}
	if utf8.RuneCountInString(strings.TrimSpace(d.Title)) > 80 {
		bad("title: up to 80 characters")
	}
	if utf8.RuneCountInString(d.Description) > 2000 {
		bad("description: up to 2000 characters")
	}
	switch {
	case (d.Script == nil) == (d.Command == nil):
		bad("give either script {lang, body} or command {argv}")
	case d.Script != nil:
		if _, ok := ScriptFiles[d.Script.Lang]; !ok {
			bad("script.lang %q: bash, sh, python, node or go", d.Script.Lang)
		}
		if strings.TrimSpace(d.Script.Body) == "" || !utf8.ValidString(d.Script.Body) || utf8.RuneCountInString(d.Script.Body) > 65536 {
			bad("script.body: UTF-8 text, not blank, up to 65536 characters")
		}
	default:
		if n := len(d.Command.Argv); n == 0 || n > 32 {
			bad("command.argv: 1 to 32 items")
		} else if a0 := d.Command.Argv[0]; a0 == "" || filepath.IsAbs(a0) || strings.Contains(a0, "{") || !strings.ContainsAny(a0, `/\`) {
			bad("command.argv[0] %q: a file of the project, as a relative path like bin/tool", a0)
		}
		for _, x := range d.Command.Argv {
			if utf8.RuneCountInString(x) > 500 || strings.ContainsRune(x, 0) {
				bad("command.argv: items up to 500 characters, no NUL")
			}
		}
	}
	if cwd := d.CwdOr(); filepath.IsAbs(cwd) || strings.HasPrefix(filepath.Clean(cwd), "..") {
		bad("cwd %q: a directory inside the project, relative", d.Cwd)
	}
	if len(d.Params) > 8 {
		bad("params: at most 8")
	}
	seen := map[string]bool{}
	for i, p := range d.Params {
		if !paramPattern.MatchString(p.Name) || seen[p.Name] {
			bad("params[%d] %q: a-z, 0-9 and _, starting with a letter, unique", i, p.Name)
		}
		seen[p.Name] = true
		if utf8.RuneCountInString(p.Label) > 80 {
			bad("param %s: label up to 80 characters", p.Name)
		}
		switch p.Type {
		case "enum":
			if len(p.Values) == 0 || len(p.Values) > 20 {
				bad("param %s: an enum has 1 to 20 values", p.Name)
			}
		case "int", "bool", "commit", "item":
		default:
			bad("param %s: type %q is enum, int, bool, commit or item", p.Name, p.Type)
		}
		for _, v := range p.Values {
			if v == "" || len(v) > 80 || strings.HasPrefix(v, "-") {
				bad("param %s: value %q: not empty, up to 80 characters, not starting with -", p.Name, v)
			}
		}
	}
	if d.Command != nil && len(d.Command.Argv) > 0 {
		for _, a := range d.Command.Argv[1:] {
			if n, ok := wholeParam(a); ok && !seen[n] {
				bad("command.argv: %s names no parameter", a)
			}
		}
	}
	if len(d.Env) > 32 {
		bad("env: at most 32 names")
	}
	envSeen := map[string]bool{}
	for _, e := range d.Env {
		if !envPattern.MatchString(e) || reservedEnv.MatchString(e) || envSeen[e] {
			bad("env %q: a variable name, once, never HARNSY_*, CCBUS_* or ACTION_*", e)
		}
		envSeen[e] = true
	}
	if _, err := d.TimeoutOr(); err != nil {
		errs = append(errs, err)
	}
	for _, m := range d.Marks {
		if m != "restarts_node" && m != "deletes" && m != "network" {
			bad("marks: restarts_node, deletes or network, not %q", m)
		}
	}
	if len(d.Numbers) > 8 {
		bad("numbers: at most 8")
	}
	numSeen := map[string]bool{}
	for i, n := range d.Numbers {
		if !paramPattern.MatchString(n.Key) || n.Of != "" && !paramPattern.MatchString(n.Of) || numSeen[n.Key] {
			bad("numbers[%d]: key and of are a-z, 0-9 and _, starting with a letter; keys unique", i)
		}
		numSeen[n.Key] = true
		if utf8.RuneCountInString(n.Label) > 80 {
			bad("numbers[%d]: label up to 80 characters", i)
		}
	}
	return errors.Join(errs...)
}

// CwdOr is the action's working directory, relative to the project: cwd,
// "." when empty.
func (d *Definition) CwdOr() string {
	if d.Cwd == "" {
		return "."
	}
	return d.Cwd
}

// WorkDir is the action's working directory: cwd under the project's directory,
// symlinks resolved; an error when it leaves the project (harnsy: 409
// cwd_outside).
func (d *Definition) WorkDir(project string) (string, error) {
	root, err := realDir(project)
	if err != nil {
		return "", err
	}
	dir, err := realDir(filepath.Join(root, d.CwdOr()))
	if err != nil || !under(dir, root) {
		return "", fmt.Errorf("cwd %q is outside the project", d.Cwd)
	}
	return dir, nil
}

// CommandFile is a command's file, as harnsy finds it: argv[0] under the
// action's working directory, symlinks resolved, inside the project. harnsy
// pins this file's sha256, and only it.
func (d *Definition) CommandFile(project string) (string, error) {
	dir, err := d.WorkDir(project)
	if err != nil {
		return "", err
	}
	root, _ := realDir(project)
	path := filepath.Join(dir, d.Command.Argv[0])
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if !under(filepath.Dir(path), root) {
		return "", fmt.Errorf("%s is outside the project", d.Command.Argv[0])
	}
	return path, nil
}

func realDir(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

func under(p, root string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// wholeParam: a is a whole {name} argument, the only place harnsy puts a
// parameter in a command; "--level={level}" stays as written.
func wholeParam(a string) (string, bool) {
	n, ok := strings.CutPrefix(a, "{")
	if !ok || !strings.HasSuffix(n, "}") {
		return "", false
	}
	return strings.TrimSuffix(n, "}"), true
}

// TimeoutOr is the run's time limit: timeout, 30m when empty.
func (d *Definition) TimeoutOr() (time.Duration, error) {
	if d.Timeout == "" {
		return 30 * time.Minute, nil
	}
	t, err := time.ParseDuration(d.Timeout)
	if err != nil || t < time.Minute || t > 2*time.Hour {
		return 0, fmt.Errorf("timeout %q: a duration from 1m to 2h", d.Timeout)
	}
	return t, nil
}

// Args turns the values given (by parameter name; the rest take their
// default) into arguments as harnsy passes them: script parameters in order;
// in a command, each argument that is a whole {name} replaced by that value,
// every other argument as written. dir is the action's working directory,
// where a commit is resolved.
func (d *Definition) Args(given map[string]string, dir string) ([]string, error) {
	values := map[string]string{}
	for k := range given {
		if d.param(k) == nil {
			return nil, fmt.Errorf("no parameter %q", k)
		}
	}
	var ordered []string
	for _, p := range d.Params {
		v, ok := given[p.Name]
		if !ok {
			if p.Default == nil {
				return nil, fmt.Errorf("parameter %s: no value and no default", p.Name)
			}
			v = defaultString(p.Default)
		}
		v, err := p.value(v, dir)
		if err != nil {
			return nil, err
		}
		values[p.Name] = v
		ordered = append(ordered, v)
	}
	if d.Command == nil {
		return ordered, nil
	}
	var out []string
	for _, a := range d.Command.Argv[1:] {
		if n, ok := wholeParam(a); ok {
			a = values[n]
		}
		out = append(out, a)
	}
	return out, nil
}

func (d *Definition) param(name string) *Param {
	for i := range d.Params {
		if d.Params[i].Name == name {
			return &d.Params[i]
		}
	}
	return nil
}

// defaultString is a default as text: a JSON number 1000000 is "1000000",
// not "1e+06".
func defaultString(v any) string {
	if f, ok := v.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

var refPattern = regexp.MustCompile(`^[A-Za-z0-9._/][A-Za-z0-9._/~^-]{0,99}$`)

// value checks v as harnsy does (trimmed first) and gives the argument.
func (p Param) value(v, dir string) (string, error) {
	v = strings.TrimSpace(v)
	switch p.Type {
	case "enum":
		for _, ok := range p.Values {
			if v == ok {
				return v, nil
			}
		}
		return "", fmt.Errorf("parameter %s: %q is not one of %v", p.Name, v, p.Values)
	case "int", "item":
		n, err := strconv.Atoi(v)
		if err != nil || (p.Type == "item" && n < 1) {
			return "", fmt.Errorf("parameter %s: %q is not a whole number", p.Name, v)
		}
		if p.Min != nil && n < *p.Min || p.Max != nil && n > *p.Max {
			return "", fmt.Errorf("parameter %s: %d is out of its range", p.Name, n)
		}
		return strconv.Itoa(n), nil
	case "bool":
		b, err := strconv.ParseBool(v) // as harnsy: 1, t, true, TRUE, 0, f, false…
		if err != nil {
			return "", fmt.Errorf("parameter %s: true or false, not %q", p.Name, v)
		}
		return strconv.FormatBool(b), nil
	case "commit":
		if !refPattern.MatchString(v) || strings.Contains(v, "..") {
			return "", fmt.Errorf("parameter %s: %q is not a branch, tag or commit", p.Name, v)
		}
		cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "--end-of-options", v+"^{commit}")
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("parameter %s: %q is not a commit here", p.Name, v)
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", fmt.Errorf("parameter %s: unknown type %q", p.Name, p.Type)
}
