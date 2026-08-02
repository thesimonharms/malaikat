//go:build !windows

package engine

import "os/exec"

func setHighPriority(cmd *exec.Cmd) {
	// no-op on non-Windows
}
