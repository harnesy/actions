//go:build !unix

package main

import (
	"os/exec"
	"time"
)

// inGroup: without process groups the program itself is killed when its
// context ends; Wait returns after grace even if a child still holds its
// output.
func inGroup(cmd *exec.Cmd, grace time.Duration) {
	cmd.WaitDelay = grace
}
