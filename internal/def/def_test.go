package def

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every action in this repository loads and passes the checks harnsy makes.
func TestRepositoryActions(t *testing.T) {
	dirs, _ := filepath.Glob("../../actions/*")
	if len(dirs) == 0 {
		t.Fatal("no actions found")
	}
	for _, dir := range dirs {
		d, err := Load(dir)
		if err != nil {
			t.Errorf("%s: %v", dir, err)
			continue
		}
		if d.Name != filepath.Base(dir) {
			t.Errorf("%s: name %q differs from its directory", dir, d.Name)
		}
		if _, err := d.Args(nil, "."); err != nil {
			t.Errorf("%s: its defaults do not run: %v", dir, err)
		}
	}
}

func intp(n int) *int { return &n }

func TestArgs(t *testing.T) {
	d := &Definition{Name: "x", Command: &Command{Argv: []string{"bin/x", "--level", "{level}", "{n}", "--n={n}", "fixed"}},
		Params: []Param{
			{Name: "level", Type: "enum", Values: []string{"low", "high"}, Default: "low"},
			{Name: "n", Type: "int", Min: intp(1), Max: intp(5)},
			{Name: "dry", Type: "bool", Default: false},
		}}
	args, err := d.Args(map[string]string{"n": "3"}, ".")
	// Only a whole {name} argument is replaced, as harnsy does.
	if err != nil || strings.Join(args, " ") != "--level low 3 --n={n} fixed" {
		t.Fatalf("command args: %q %v", args, err)
	}
	d.Command, d.Script = nil, &Script{Lang: "bash", Body: "true"}
	args, err = d.Args(map[string]string{"n": " 3 ", "level": "high", "dry": "TRUE"}, ".")
	if err != nil || strings.Join(args, " ") != "high 3 true" { // trimmed; bool as strconv.ParseBool
		t.Fatalf("script args: %q %v", args, err)
	}
	for _, bad := range []map[string]string{
		{},                         // n has no default
		{"n": "9"},                 // out of range
		{"n": "1", "level": "mid"}, // not a value
		{"n": "1", "dry": "yes"},   // not a bool
		{"n": "1", "nope": "1"},    // no such parameter
	} {
		if _, err := d.Args(bad, "."); err == nil {
			t.Errorf("%v passed", bad)
		}
	}
}

func TestCheckRefuses(t *testing.T) {
	for name, d := range map[string]Definition{
		"bad name":       {Name: "Disk", Script: &Script{Lang: "bash", Body: "x"}},
		"both":           {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Command: &Command{Argv: []string{"bin/x"}}},
		"neither":        {Name: "x"},
		"absolute argv0": {Name: "x", Command: &Command{Argv: []string{"/usr/bin/x"}}},
		"bare argv0":     {Name: "x", Command: &Command{Argv: []string{"make"}}},
		"dash value":     {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Params: []Param{{Name: "a", Type: "enum", Values: []string{"-rf"}}}},
		"reserved env":   {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Env: []string{"HARNSY_TOKEN"}},
		"long timeout":   {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Timeout: "3h"},
		"blank body":     {Name: "x", Script: &Script{Lang: "bash", Body: " \n"}},
		"long title":     {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Title: strings.Repeat("t", 81)},
		"long desc":      {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Description: strings.Repeat("d", 2001)},
		"cwd outside":    {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Cwd: "../other"},
		"cwd absolute":   {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Cwd: "/tmp"},
		"same param":     {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Params: []Param{{Name: "a", Type: "int"}, {Name: "a", Type: "bool"}}},
		"21 values":      {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Params: []Param{{Name: "a", Type: "enum", Values: strings.Split("a b c d e f g h i j k l m n o p q r s t u", " ")}}},
		"long value":     {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Params: []Param{{Name: "a", Type: "enum", Values: []string{strings.Repeat("v", 81)}}}},
		"bad mark":       {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Marks: []string{"reboots"}},
		"bad number key": {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Numbers: []NumberD{{Key: "Size"}}},
		"bad number of":  {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Numbers: []NumberD{{Key: "used", Of: "Total"}}},
		"brace argv0":    {Name: "x", Command: &Command{Argv: []string{"bin/{tool}"}}},
		"unknown {p}":    {Name: "x", Command: &Command{Argv: []string{"bin/x", "{nope}"}}},
		"long arg":       {Name: "x", Command: &Command{Argv: []string{"bin/x", strings.Repeat("a", 501)}}},
		"env twice":      {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Env: []string{"GOPATH", "GOPATH"}},
		"same number":    {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Numbers: []NumberD{{Key: "a"}, {Key: "a"}}},
		"long label":     {Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Params: []Param{{Name: "a", Type: "int", Label: strings.Repeat("l", 81)}}},
	} {
		if d.Check() == nil {
			t.Errorf("%s: passed", name)
		}
	}
}

func TestLoadRefusesUnknownFields(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "action.json"), []byte(`{"name":"x","script":{"lang":"bash"},"timeot":"1m"}`), 0o644)
	os.WriteFile(filepath.Join(dir, "run.sh"), []byte("true\n"), 0o644)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "timeot") {
		t.Errorf("a misspelled field: %v", err)
	}
}

// A command's file is argv[0] under the action's cwd, inside the project.
func TestCommandFile(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "tools", "bin"), 0o755)
	os.WriteFile(filepath.Join(root, "tools", "bin", "x"), []byte("x"), 0o755)
	d := &Definition{Name: "x", Cwd: "tools", Command: &Command{Argv: []string{"bin/x"}}}
	got, err := d.CommandFile(root)
	real, _ := filepath.EvalSymlinks(filepath.Join(root, "tools", "bin", "x"))
	if err != nil || got != real {
		t.Fatalf("file: %q %v", got, err)
	}
	d.Command.Argv[0] = "../../elsewhere/x"
	if _, err := d.CommandFile(root); err == nil {
		t.Error("a file outside the project passed")
	}
	d.Cwd = "../"
	if _, err := d.WorkDir(root); err == nil {
		t.Error("a cwd outside the project passed")
	}
}

// A JSON default of 1000000 is passed as 1000000, as harnsy passes it.
func TestBigDefault(t *testing.T) {
	d := &Definition{Name: "x", Script: &Script{Lang: "bash", Body: "x"}, Params: []Param{{Name: "n", Type: "int", Default: float64(1000000)}}}
	args, err := d.Args(nil, ".")
	if err != nil || len(args) != 1 || args[0] != "1000000" {
		t.Fatalf("args %q %v", args, err)
	}
}

// The script body counts characters, as harnsy does.
func TestBodyInCharacters(t *testing.T) {
	d := &Definition{Name: "x", Script: &Script{Lang: "bash", Body: strings.Repeat("я", 65536)}}
	if err := d.Check(); err != nil {
		t.Errorf("65536 characters: %v", err)
	}
}
