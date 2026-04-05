//go:build !windows

package main

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func notifySyncSignal(ch chan os.Signal) {
	signal.Notify(ch, syscall.SIGUSR1)
}

func sendSyncSignal(proc *os.Process) error {
	return proc.Signal(syscall.SIGUSR1)
}
