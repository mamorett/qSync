//go:build linux || darwin

package rsyncx

import (
	"os/exec"
	"syscall"
)

// configurePGID puts the child in its own process group so a signal to the
// group tears down both ssh and rsync.
func configurePGID(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
