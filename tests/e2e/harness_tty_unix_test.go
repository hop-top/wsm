//go:build unix

package e2e_test

import (
	"os/exec"
	"syscall"
)

// adoptControllingTTY makes the child's stdin (fd 0), already wired to a
// PTY slave, its controlling terminal.
//
// kit's confirm gate asks on /dev/tty, not on stdin/stdout. A child that
// merely inherits the slave as fd 0..2 has no controlling terminal, so the
// open fails and the gate takes its non-interactive refusal (exit 5)
// without ever printing the [y/N] prompt.
func adoptControllingTTY(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
}
