//go:build windows

package engine

import (
	"os/exec"
	"syscall"
)

func setHighPriority(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// HIGH_PRIORITY_CLASS — reduces preemption during token generation.
	cmd.SysProcAttr.CreationFlags |= 0x00000080
}
