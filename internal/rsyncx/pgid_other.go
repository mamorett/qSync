//go:build !linux && !darwin

package rsyncx

import (
	"os/exec"
	"time"
)

// ConfigurePGID sets WaitDelay for non-unix systems.
func ConfigurePGID(cmd *exec.Cmd) {
	cmd.WaitDelay = 500 * time.Millisecond
}

func configurePGID(cmd *exec.Cmd) {
	ConfigurePGID(cmd)
}
