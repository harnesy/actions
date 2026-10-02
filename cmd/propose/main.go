// Command propose sends an action of this repository to harnsy as a
// proposal (POST /api/actions). Nothing runs until the human enables it on
// the dashboard.
//
//	go run ./cmd/propose -from <agent> -project <project> actions/<name>
//	go run ./cmd/propose -dry-run actions/<name>     # print the JSON only
//
// The node's address: -url, else $HARNSY_URL, else the agent port
// http://127.0.0.1:7790.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/harnesy/action-template/internal/def"
)

func main() {
	from := flag.String("from", "", "your agent name (seated in a team of the project)")
	project := flag.String("project", "", "the project's id or name")
	base := flag.String("url", "", "the harnsy node (default $HARNSY_URL or http://127.0.0.1:7790)")
	dry := flag.Bool("dry-run", false, "print the request body, send nothing")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/propose [-dry-run] -from <agent> -project <project> actions/<name>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	repo, err := os.Getwd()
	if err == nil {
		err = propose(os.Stdout, repo, flag.Arg(0), *from, *project, *base, *dry)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "propose:", err)
		os.Exit(1)
	}
}

// propose sends actions/<name> of the project repo (or prints it).
func propose(w io.Writer, repo, dir, from, project, base string, dry bool) error {
	d, err := def.Load(dir)
	if err != nil {
		return err
	}
	if d.Command != nil {
		// Where harnsy looks for it: argv[0] under the action's cwd.
		file, err := d.CommandFile(repo)
		if err != nil {
			return err
		}
		if _, err := os.Stat(file); err != nil {
			return fmt.Errorf("%s: %w; harnsy pins this file's sha256 when the human enables the action, build it first", d.Command.Argv[0], err)
		}
		if strings.HasSuffix(file, ".sh") {
			fmt.Fprintf(w, "note: harnsy pins %s only, not what it calls; propose a built binary, or the script itself as script.body\n", d.Command.Argv[0])
		}
	}
	d.From, d.Project = from, project
	body, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if dry {
		fmt.Fprintln(w, string(body))
		return nil
	}
	if from == "" || project == "" {
		return fmt.Errorf("-from and -project are required (or -dry-run)")
	}
	if base == "" {
		base = os.Getenv("HARNSY_URL")
	}
	if base == "" {
		base = "http://127.0.0.1:7790"
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Post(strings.TrimRight(base, "/")+"/api/actions", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		OK      bool   `json:"ok"`
		Version int    `json:"version"`
		Error   string `json:"error"`
		Code    string `json:"code"`
		Action  struct {
			ID int64 `json:"id"`
		} `json:"action"`
	}
	_ = json.Unmarshal(raw, &out) // an answer that is not JSON is shown as it is, below
	if resp.StatusCode != http.StatusCreated || !out.OK {
		if out.Error != "" {
			return fmt.Errorf("HTTP %d %s: %s", resp.StatusCode, out.Code, out.Error)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(raw))
	}
	fmt.Fprintf(w, "proposed %s: action %d, version %d. It waits for the human on the dashboard (Actions).\n", d.Name, out.Action.ID, out.Version)
	return nil
}
