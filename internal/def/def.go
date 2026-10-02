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
)

// Definition is an action's definition (contract 1.0.0).
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

// Check finds what harnsy would refuse in the definition (a subset of
// contract/action-definition.schema.json, with the reasons spelled out).
func (d *Definition) Check() error {
	var errs []error
	if !namePattern.MatchString(d.Name) {
		errs = append(errs, fmt.Errorf("name %q: lowercase letters, digits and -, up to 40", d.Name))
	}
	if (d.Script == nil) == (d.Command == nil) {
		errs = append(errs, errors.New("exactly one of script and command"))
	}
	if d.Script != nil && (len(d.Script.Body) == 0 || len(d.Script.Body) > 65536) {
		errs = append(errs, errors.New("script.body: 1 to 65536 bytes"))
	}
	if d.Command != nil {
		if n := len(d.Command.Argv); n == 0 || n > 32 {
			errs = append(errs, errors.New("command.argv: 1 to 32 items"))
		} else if a0 := d.Command.Argv[0]; filepath.IsAbs(a0) || !strings.Contains(a0, "/") || strings.HasPrefix(filepath.Clean(a0), "..") {
			errs = append(errs, fmt.Errorf("command.argv[0] %q: a relative path inside the project, with a slash (bin/tool)", a0))
		}
	}
	if len(d.Params) > 8 {
		errs = append(errs, errors.New("params: at most 8"))
	}
	for _, p := range d.Params {
		if !paramPattern.MatchString(p.Name) {
			errs = append(errs, fmt.Errorf("param %q: [a-z][a-z0-9_]{0,31}", p.Name))
		}
		switch p.Type {
		case "enum":
			if len(p.Values) == 0 {
				errs = append(errs, fmt.Errorf("param %s: an enum needs values", p.Name))
			}
			for _, v := range p.Values {
				if v == "" || strings.HasPrefix(v, "-") {
					errs = append(errs, fmt.Errorf("param %s: value %q may not be empty or start with -", p.Name, v))
				}
			}
		case "int", "bool", "commit", "item":
		default:
			errs = append(errs, fmt.Errorf("param %s: type %q is enum, int, bool, commit or item", p.Name, p.Type))
		}
	}
	for _, e := range d.Env {
		if !envPattern.MatchString(e) || reservedEnv.MatchString(e) {
			errs = append(errs, fmt.Errorf("env %q: a variable name, not HARNSY_*, CCBUS_* or ACTION_*", e))
		}
	}
	if _, err := d.TimeoutOr(); err != nil {
		errs = append(errs, err)
	}
	if len(d.Numbers) > 8 {
		errs = append(errs, errors.New("numbers: at most 8"))
	}
	return errors.Join(errs...)
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
// default) into arguments as harnsy passes them: script parameters in order,
// a command's {name} places filled. repo is where a commit is resolved.
func (d *Definition) Args(given map[string]string, repo string) ([]string, error) {
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
			v = fmt.Sprint(p.Default)
		}
		v, err := p.value(v, repo)
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
		for name, v := range values {
			a = strings.ReplaceAll(a, "{"+name+"}", v)
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

// value checks v as harnsy does and gives the argument.
func (p Param) value(v, repo string) (string, error) {
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
		if v != "true" && v != "false" {
			return "", fmt.Errorf("parameter %s: true or false, not %q", p.Name, v)
		}
		return v, nil
	case "commit":
		out, err := exec.Command("git", "-C", repo, "rev-parse", "--verify", v+"^{commit}").Output()
		if err != nil {
			return "", fmt.Errorf("parameter %s: %q is not a commit here", p.Name, v)
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", fmt.Errorf("parameter %s: unknown type %q", p.Name, p.Type)
}
