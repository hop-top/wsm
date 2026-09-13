//go:build darwin || linux

package commands

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"hop.top/wsm/pkg/irc"
)

// EnvDaemonExe overrides the executable `wsm irc start` spawns as the
// daemon.
//
// The real spawn re-executes this binary, which is what makes the
// daemon a plain `wsm irc daemon <ws>` invocation rather than a second
// code path. That is also what makes it untestable at the process
// boundary: a test that lets the CLI spawn the genuine daemon gets a
// process that wants a live ergo, a bot session and a workspace's
// members file. Pointing this variable at a stub yields the one thing
// the lifecycle verbs actually contract on — a detached child that
// writes status.json and stays alive until signalled.
const EnvDaemonExe = "WSM_IRC_DAEMON_EXE"

// daemonExecutable resolves the binary to spawn for the daemon.
//
// os.Executable is resolved rather than assuming argv[0]: `wsm` is
// commonly invoked through a PATH lookup or a symlink, and the daemon
// outlives the shell that started it, so a relative argv[0] would
// break the moment the child's working directory differed.
func daemonExecutable() (string, error) {
	if exe := os.Getenv(EnvDaemonExe); exe != "" {
		return exe, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve wsm executable: %w", err)
	}
	return exe, nil
}

// spawnDaemon starts `wsm irc daemon <workspace>` detached and returns
// the child's pid.
//
// Setsid puts the child in its own session, so it survives the
// terminal that started it: closing the shell must not take the
// workspace's IRC presence with it. Stdio goes to the daemon log
// rather than being inherited, for the same reason — a detached child
// holding the parent's terminal open is a process that cannot be
// backgrounded cleanly, and its output would otherwise interleave with
// the CLI's own.
//
// The parent does not wait: the daemon is long-running by design, and
// readiness is observed through status.json rather than through the
// exit of a process that is not supposed to exit.
func spawnDaemon(paths irc.Paths, workspaceRef string, extraEnv []string) (int, error) {
	exe, err := daemonExecutable()
	if err != nil {
		return 0, err
	}

	if err := os.MkdirAll(paths.StateDir, 0o700); err != nil {
		return 0, fmt.Errorf("create irc state dir: %w", err)
	}
	logFile, err := os.OpenFile(paths.DaemonLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open daemon log: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.Command(exe, "irc", "daemon", workspaceRef)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("spawn irc daemon: %w", err)
	}

	// Release the child rather than reaping it: the CLI exits long
	// before the daemon does, and a Wait here would block the verb for
	// the daemon's entire lifetime.
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}

// signalDaemon sends sig to pid.
func signalDaemon(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}

// processAlive reports whether pid names a live process.
//
// Signal 0 performs the permission and existence checks without
// delivering anything, which is the portable way to ask "is this pid
// still there?" without disturbing the process.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
