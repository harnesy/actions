package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func action(t *testing.T) (repo, dir string) {
	t.Helper()
	repo = t.TempDir()
	dir = filepath.Join(repo, "actions", "x")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "action.json"), []byte(`{"name":"x","script":{"lang":"bash"}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "run.sh"), []byte("echo hi\n"), 0o644)
	return repo, dir
}

// The proposal carries the script's body, from and project, to /api/actions.
func TestPropose(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/actions" || r.Header.Get("Content-Type") != "application/json" {
			http.Error(rw, "wrong request", 400)
			return
		}
		json.NewDecoder(r.Body).Decode(&got)
		rw.WriteHeader(201)
		rw.Write([]byte(`{"ok":true,"action":{"id":7},"version":2}`))
	}))
	defer srv.Close()
	repo, dir := action(t)
	var out bytes.Buffer
	if err := propose(&out, repo, dir, "dev-ab", "acme", srv.URL, false); err != nil {
		t.Fatal(err)
	}
	script, _ := got["script"].(map[string]any)
	if got["from"] != "dev-ab" || got["project"] != "acme" || script["body"] != "echo hi\n" || !strings.Contains(out.String(), "action 7, version 2") {
		t.Errorf("sent %v, said %q", got, out.String())
	}
}

func TestProposeRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(403)
		rw.Write([]byte(`{"ok":false,"error":"dev-ab holds no seat in acme","code":"not_seated"}`))
	}))
	defer srv.Close()
	repo, dir := action(t)
	err := propose(&bytes.Buffer{}, repo, dir, "dev-ab", "acme", srv.URL, false)
	if err == nil || !strings.Contains(err.Error(), "403 not_seated") {
		t.Errorf("refusal: %v", err)
	}
}

// A dry run sends nothing; a command's file must exist under its cwd.
func TestProposeDryRunAndCommand(t *testing.T) {
	repo, dir := action(t)
	var out bytes.Buffer
	if err := propose(&out, repo, dir, "", "", "http://127.0.0.1:1", true); err != nil || !strings.Contains(out.String(), `"body": "echo hi\n"`) {
		t.Fatalf("dry run: %v %q", err, out.String())
	}
	os.WriteFile(filepath.Join(dir, "action.json"), []byte(`{"name":"x","cwd":"tools","command":{"argv":["bin/x"]}}`), 0o644)
	os.MkdirAll(filepath.Join(repo, "tools"), 0o755)
	if err := propose(&out, repo, dir, "", "", "", true); err == nil || !strings.Contains(err.Error(), "build it first") {
		t.Errorf("a missing binary: %v", err)
	}
	os.MkdirAll(filepath.Join(repo, "tools", "bin"), 0o755)
	os.WriteFile(filepath.Join(repo, "tools", "bin", "x"), []byte("x"), 0o755)
	if err := propose(&out, repo, dir, "", "", "", true); err != nil {
		t.Errorf("the binary under cwd: %v", err)
	}
}
