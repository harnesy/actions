//go:build !unix

package main

import (
	"os"
	"os/exec"
)

// harnsy runs actions on Linux only (contract 1.1.0); here, without process
// groups, try stops the program itself.
func setGroup(*exec.Cmd) {}

func signalGroup(pid int, kill bool) {
	if p, err := os.FindProcess(pid); err == nil && kill {
		_ = p.Kill()
	}
}
