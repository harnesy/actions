//go:build unix

package main

import (
	"os/exec"
	"syscall"
	"time"
)

// inGroup runs cmd in a process group of its own and, when its context
// ends, stops the whole group as harnsy does: SIGTERM, then SIGKILL after
// grace.
func inGroup(cmd *exec.Cmd, grace time.Duration) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pgid := -cmd.Process.Pid
		err := syscall.Kill(pgid, syscall.SIGTERM)
		time.AfterFunc(grace, func() { _ = syscall.Kill(pgid, syscall.SIGKILL) })
		return err
	}
	cmd.WaitDelay = grace + 2*time.Second
}
