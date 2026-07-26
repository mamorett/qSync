//go:build linux || darwin

package rsyncx

import (
	"os/exec"
	"syscall"
	"time"
)

// ConfigurePGID puts the child process in its own process group and
// configures cmd.Cancel to terminate the ENTIRE process group (-pgid) when ctx is cancelled.
func ConfigurePGID(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 500 * time.Millisecond
	cmd.Cancel = func() error {
		if cmd.Process != nil && cmd.Process.Pid > 0 {
			pgid := cmd.Process.Pid
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
			go func() {
				time.Sleep(200 * time.Millisecond)
				if cmd.Process != nil && cmd.Process.Pid > 0 {
					_ = syscall.Kill(-pgid, syscall.SIGKILL)
				}
			}()
		}
		return nil
	}
}

func configurePGID(cmd *exec.Cmd) {
	ConfigurePGID(cmd)
}
