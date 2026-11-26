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

// detachControllingTTY starts the child in a new session with no
// controlling terminal, which is what a script or CI job without a TTY
// looks like to it.
//
// Without this the child inherits the test runner's controlling terminal.
// Under `docker run -t` (or any interactive shell) that is a live terminal,
// so kit's confirm gate finds /dev/tty, prints its [y/N] prompt there, and
// blocks forever instead of taking the non-interactive refusal the
// non-TTY tests assert.
func detachControllingTTY(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
