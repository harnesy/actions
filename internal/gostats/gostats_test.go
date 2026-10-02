package gostats

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCount(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main\n\nfunc main() {}\n")
	write("lib/lib.go", "package lib\n")
	write("lib/lib_test.go", "package lib\n\nimport \"testing\"\n")
	write("vendor/x/x.go", "package x\n")
	write(".git/hook.go", "package x\n")
	write("README.md", "# x\n")
	s, err := Count(root)
	if err != nil {
		t.Fatal(err)
	}
	if s != (Stats{Files: 3, TestFiles: 1, Lines: 7, Packages: 2}) {
		t.Errorf("stats: %+v", s)
	}
}
