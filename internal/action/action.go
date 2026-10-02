// Package action is the script side of the harnsy actions contract 1.x
// (contract/contract.md): the environment harnsy sets, the ::step and
// ::fail markers, and the result file.
//
// A program built from this module runs as a harnsy "command" action; a
// single-file action (lang go) cannot import it and carries a copy of the
// few lines it needs (actions/todo-count/main.go).
package action

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Contract is the major version of the contract this package speaks.
const Contract = "1"

// Limits of the result file, from contract 1.0.0.
const (
	MaxResultBytes = 64 << 10
	MaxSummary     = 300
	MaxNumbers     = 8
	MaxNumberText  = 40
	MaxArtifacts   = 20
	MaxLinks       = 10
	MaxLabel       = 80
	MaxURL         = 500
	MaxMarker      = 200
)

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// Env is what harnsy tells a run through its environment.
type Env struct {
	Contract string // ACTION_CONTRACT, the major version: "1"
	RunID    string // ACTION_RUN_ID
	Result   string // ACTION_RESULT: the file the result goes to
	Dir      string // ACTION_DIR: this run's own directory
}

// FromEnv reads the run's environment. Outside harnsy (no ACTION_RESULT)
// it returns an error: run the action with `make try` to get one.
func FromEnv() (Env, error) {
	e := Env{
		Contract: os.Getenv("ACTION_CONTRACT"),
		RunID:    os.Getenv("ACTION_RUN_ID"),
		Result:   os.Getenv("ACTION_RESULT"),
		Dir:      os.Getenv("ACTION_DIR"),
	}
	if e.Result == "" {
		return e, errors.New("ACTION_RESULT is not set: this runs under harnsy, or under `make try`")
	}
	if e.Contract != "" && e.Contract != Contract {
		return e, fmt.Errorf("harnsy runs contract %s, this program speaks %s", e.Contract, Contract)
	}
	return e, nil
}

// out is where markers go; a test replaces it.
var out io.Writer = os.Stdout

// Step tells the dashboard what runs now (::step).
func Step(format string, args ...any) { marker("step", fmt.Sprintf(format, args...)) }

// Fail says in one line why the run failed (::fail); the latest counts. It
// does not end the program: exit non-zero for that.
func Fail(format string, args ...any) { marker("fail", fmt.Sprintf(format, args...)) }

func marker(kind, text string) {
	text = strings.Join(strings.Fields(text), " ") // one line
	fmt.Fprintf(out, "::%s %s\n", kind, cut(text, MaxMarker))
}

// Result is what a run reports: all optional.
type Result struct {
	Summary   string            `json:"summary,omitempty"`
	Numbers   map[string]Number `json:"numbers,omitempty"`
	Artifacts []Artifact        `json:"artifacts,omitempty"`
	Links     []Link            `json:"links,omitempty"`
}

// Number is a figure: a number, or a short text such as "1.4 GB".
type Number struct {
	value any
}

// Num is a numeric figure.
func Num(v float64) Number { return Number{v} }

// Text is a figure as text, up to 40 characters.
func Text(s string) Number { return Number{s} }

func (n Number) MarshalJSON() ([]byte, error) { return json.Marshal(n.value) }

func (n *Number) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch v.(type) {
	case float64, string:
		n.value = v
		return nil
	}
	return fmt.Errorf("a number is a JSON number or string, not %s", b)
}

// Artifact is a file the run made: inside the project's directory or
// ACTION_DIR; harnsy records its size and sha256.
type Artifact struct {
	Path  string `json:"path"`
	Label string `json:"label,omitempty"`
}

// Link is an http or https URL the dashboard shows.
type Link struct {
	URL   string `json:"url"`
	Label string `json:"label,omitempty"`
}

// Check reports every way r breaks the contract's limits. harnsy keeps what
// passes and drops the rest; checking here finds the mistake before it does.
func (r Result) Check() error {
	var errs []error
	if strings.ContainsAny(r.Summary, "\n\r") {
		errs = append(errs, errors.New("summary: one line"))
	}
	if utf8.RuneCountInString(r.Summary) > MaxSummary {
		errs = append(errs, fmt.Errorf("summary: %d characters kept, it has %d", MaxSummary, utf8.RuneCountInString(r.Summary)))
	}
	if len(r.Numbers) > MaxNumbers {
		errs = append(errs, fmt.Errorf("numbers: at most %d, %d given", MaxNumbers, len(r.Numbers)))
	}
	for k, n := range r.Numbers {
		if !keyPattern.MatchString(k) {
			errs = append(errs, fmt.Errorf("numbers: key %q is not [a-z][a-z0-9_]{0,31}", k))
		}
		switch v := n.value.(type) {
		case float64:
		case string:
			if utf8.RuneCountInString(v) > MaxNumberText {
				errs = append(errs, fmt.Errorf("numbers.%s: text up to %d characters", k, MaxNumberText))
			}
		default:
			errs = append(errs, fmt.Errorf("numbers.%s: no value (use Num or Text)", k))
		}
	}
	if len(r.Artifacts) > MaxArtifacts {
		errs = append(errs, fmt.Errorf("artifacts: at most %d", MaxArtifacts))
	}
	for i, a := range r.Artifacts {
		if a.Path == "" {
			errs = append(errs, fmt.Errorf("artifacts[%d]: no path", i))
		}
		if utf8.RuneCountInString(a.Label) > MaxLabel {
			errs = append(errs, fmt.Errorf("artifacts[%d]: label up to %d characters", i, MaxLabel))
		}
	}
	if len(r.Links) > MaxLinks {
		errs = append(errs, fmt.Errorf("links: at most %d", MaxLinks))
	}
	for i, l := range r.Links {
		if !strings.HasPrefix(l.URL, "http://") && !strings.HasPrefix(l.URL, "https://") || len(l.URL) > MaxURL {
			errs = append(errs, fmt.Errorf("links[%d]: an http(s) URL up to %d characters", i, MaxURL))
		}
		if utf8.RuneCountInString(l.Label) > MaxLabel {
			errs = append(errs, fmt.Errorf("links[%d]: label up to %d characters", i, MaxLabel))
		}
	}
	return errors.Join(errs...)
}

// Write checks r and writes it to the run's result file.
func (e Env) Write(r Result) error {
	if err := r.Check(); err != nil {
		return err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if len(raw) > MaxResultBytes {
		return fmt.Errorf("result: %d bytes, at most %d", len(raw), MaxResultBytes)
	}
	return os.WriteFile(e.Result, append(raw, '\n'), 0o600)
}

func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
