// todo-count: TODO and FIXME marks in the project's source files (harnsy
// action, lang go).
//
// harnsy runs a single Go file as `go run <copy>.go`, outside any module:
// only the standard library can be imported. The few helpers below are a
// copy of internal/action, kept small on purpose.
//
// Argument 1, max_todo: fail when there are more TODO marks than this.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Source files looked into, by extension.
var sourceExt = map[string]bool{
	".go": true, ".js": true, ".ts": true, ".vue": true, ".py": true, ".rs": true,
	".java": true, ".kt": true, ".php": true, ".rb": true, ".c": true, ".h": true,
	".cpp": true, ".cs": true, ".swift": true, ".sh": true,
}

func main() {
	if err := run(); err != nil {
		fail(err.Error())
		os.Exit(1)
	}
}

func run() error {
	maxTodo := 1000
	if len(os.Args) > 1 {
		n, err := strconv.Atoi(os.Args[1])
		if err != nil {
			return fmt.Errorf("max_todo: %q is not a number", os.Args[1])
		}
		maxTodo = n
	}
	step("looking for TODO and FIXME")
	todo, fixme, files := 0, 0, 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != "." && (strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !sourceExt[filepath.Ext(path)] {
			return nil
		}
		t, f, err := marks(path)
		if err != nil {
			return err
		}
		todo, fixme = todo+t, fixme+f
		if t+f > 0 {
			files++
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := writeResult(map[string]any{
		"summary": fmt.Sprintf("%d TODO and %d FIXME in %d files", todo, fixme, files),
		"numbers": map[string]any{"todo": todo, "fixme": fixme, "files": files},
	}); err != nil {
		return err
	}
	if todo > maxTodo {
		return fmt.Errorf("%d TODO marks, above %d", todo, maxTodo)
	}
	return nil
}

func marks(path string) (todo, fixme int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		todo += strings.Count(line, "TODO")
		fixme += strings.Count(line, "FIXME")
	}
	return todo, fixme, sc.Err()
}

// ---- the contract, inline (internal/action has the full helper) ----------

func step(text string) { fmt.Println("::step " + oneLine(text)) }
func fail(text string) { fmt.Println("::fail " + oneLine(text)) }

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200])
	}
	return s
}

// writeResult writes the result file harnsy reads after the run.
func writeResult(r map[string]any) error {
	path := os.Getenv("ACTION_RESULT")
	if path == "" {
		return fmt.Errorf("ACTION_RESULT is not set: run under harnsy, or `make try A=todo-count`")
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}
