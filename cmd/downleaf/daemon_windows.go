//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func setSysProcAttr(cmd *exec.Cmd) {
	// Windows does not support Setsid; the child process is already detached
	// when started without an associated console.
}

func notifySyncSignal(ch chan os.Signal) {
	// SIGUSR1 does not exist on Windows. Sync must be triggered via
	// other means (e.g. named pipe) in a future enhancement.
}

func sendSyncSignal(proc *os.Process) error {
	return fmt.Errorf("sync signal is not supported on Windows; use 'downleaf umount' instead")
}
