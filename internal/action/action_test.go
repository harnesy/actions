package action

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkersAreOneLineAndCut(t *testing.T) {
	var buf bytes.Buffer
	out = &buf
	t.Cleanup(func() { out = os.Stdout })
	Step("build %s\nnext", "abc")
	Fail("%s", strings.Repeat("я", 300))
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 || lines[0] != "::step build abc next" {
		t.Fatalf("markers: %q", lines)
	}
	if got := len([]rune(strings.TrimPrefix(lines[1], "::fail "))); got != MaxMarker {
		t.Errorf("fail text: %d runes", got)
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("ACTION_RESULT", "")
	if _, err := FromEnv(); err == nil {
		t.Error("no ACTION_RESULT accepted")
	}
	t.Setenv("ACTION_RESULT", filepath.Join(t.TempDir(), "result.json"))
	t.Setenv("ACTION_CONTRACT", "2")
	if _, err := FromEnv(); err == nil {
		t.Error("another major accepted")
	}
	t.Setenv("ACTION_CONTRACT", "1")
	t.Setenv("ACTION_RUN_ID", "7")
	e, err := FromEnv()
	if err != nil || e.RunID != "7" {
		t.Fatalf("env: %+v %v", e, err)
	}
}

func TestWriteResult(t *testing.T) {
	e := Env{Result: filepath.Join(t.TempDir(), "result.json")}
	r := Result{
		Summary:   "built abc1234",
		Numbers:   map[string]Number{"size_b": Num(1024), "took": Text("2m 10s")},
		Artifacts: []Artifact{{Path: "bin/app", Label: "app"}},
		Links:     []Link{{URL: "https://example.com/build/7"}},
	}
	if err := e.Write(r); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(e.Result)
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	nums := back["numbers"].(map[string]any)
	if nums["size_b"] != float64(1024) || nums["took"] != "2m 10s" || back["summary"] != "built abc1234" {
		t.Errorf("written: %s", raw)
	}
	var again Result
	if err := json.Unmarshal(raw, &again); err != nil || again.Check() != nil {
		t.Errorf("round trip: %v %v", err, again.Check())
	}
}

func TestCheckFindsEveryLimit(t *testing.T) {
	many := map[string]Number{}
	for _, k := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		many[k] = Num(1)
	}
	for name, r := range map[string]Result{
		"two lines":    {Summary: "a\nb"},
		"long summary": {Summary: strings.Repeat("x", MaxSummary+1)},
		"nine numbers": {Numbers: many},
		"bad key":      {Numbers: map[string]Number{"Size": Num(1)}},
		"long text":    {Numbers: map[string]Number{"t": Text(strings.Repeat("x", 41))}},
		"empty number": {Numbers: map[string]Number{"t": {}}},
		"no path":      {Artifacts: []Artifact{{Label: "x"}}},
		"ftp link":     {Links: []Link{{URL: "ftp://x"}}},
		"no host":      {Links: []Link{{URL: "https:///path"}}},
		"long label":   {Links: []Link{{URL: "https://x", Label: strings.Repeat("x", 81)}}},
		"21 artifacts": {Artifacts: make([]Artifact, 21)},
		"eleven links": {Links: make([]Link, 11)},
	} {
		if r.Check() == nil {
			t.Errorf("%s: passed", name)
		}
	}
	if err := (Result{}).Check(); err != nil {
		t.Errorf("an empty result: %v", err)
	}
}
