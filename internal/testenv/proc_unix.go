//go:build !windows

package testenv

import (
	"os/exec"
	"syscall"
)

// ownGroup runs cmd in its own process group and makes cancelling it kill
// the whole group, so a hung hook (sh → gids → …) can't outlive the test.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
