// Command go-stats is an example harnsy action built as a binary and
// proposed as a "command" (actions/go-stats/action.json): it counts the Go
// code under its working directory and reports the figures.
//
// Build it with `make build`; harnsy pins the binary's sha256 when the human
// enables the action, so every rebuild waits for the human again.
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/harnesy/action-template/internal/action"
	"github.com/harnesy/action-template/internal/gostats"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		action.Fail("%v", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run takes one parameter, min_tests: the share of test files (percent)
// below which the run fails.
func run(args []string) error {
	env, err := action.FromEnv()
	if err != nil {
		return err
	}
	minTests := 0
	if len(args) > 0 {
		if minTests, err = strconv.Atoi(args[0]); err != nil {
			return fmt.Errorf("min_tests: %q is not a number", args[0])
		}
	}
	action.Step("counting Go files")
	s, err := gostats.Count(".")
	if err != nil {
		return err
	}
	share := 0
	if s.Files > 0 {
		share = s.TestFiles * 100 / s.Files
	}
	r := action.Result{
		Summary: fmt.Sprintf("%d Go files in %d packages, %d lines; %d%% are tests", s.Files, s.Packages, s.Lines, share),
		Numbers: map[string]action.Number{
			"files":     action.Num(float64(s.Files)),
			"lines":     action.Num(float64(s.Lines)),
			"packages":  action.Num(float64(s.Packages)),
			"tests_pct": action.Num(float64(share)),
		},
	}
	if err := env.Write(r); err != nil {
		return err
	}
	if share < minTests {
		return fmt.Errorf("tests are %d%% of Go files, below %d%%", share, minTests)
	}
	return nil
}
