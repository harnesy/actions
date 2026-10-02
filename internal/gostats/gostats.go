// Package gostats counts the Go code under a directory: the work of the
// go-stats example action (cmd/go-stats).
package gostats

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Stats is what Count finds.
type Stats struct {
	Files     int // .go files
	TestFiles int // _test.go files among them
	Lines     int // lines in all of them
	Packages  int // directories holding .go files
}

// Count walks root and counts its Go files, skipping vendor/, testdata/,
// node_modules/ and hidden directories.
func Count(root string) (Stats, error) {
	var s Stats
	dirs := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		n, err := lines(path)
		if err != nil {
			return err
		}
		s.Files++
		s.Lines += n
		if strings.HasSuffix(name, "_test.go") {
			s.TestFiles++
		}
		dirs[filepath.Dir(path)] = true
		return nil
	})
	s.Packages = len(dirs)
	return s, err
}

func lines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	n := 0
	for sc.Scan() {
		n++
	}
	return n, sc.Err()
}
