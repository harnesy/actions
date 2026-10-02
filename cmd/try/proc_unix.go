//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// setGroup starts cmd as the leader of a process group of its own, as
// harnsy does, so the whole group can be stopped.
func setGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// signalGroup sends the group SIGTERM, or SIGKILL with kill.
func signalGroup(pid int, kill bool) {
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-pid, sig)
}
