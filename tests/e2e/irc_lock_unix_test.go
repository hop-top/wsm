//go:build darwin || linux

package e2e_test

import (
	"os"
	"syscall"
	"testing"
)

// holdIRCLock takes the workspace's exclusive IRC lock and returns the
// release function.
//
// This is the daemon's own lock, taken from outside it, which is how a
// test can stand in for "a daemon holds this workspace" without running
// one. The offline backfill must refuse while it is held: cursors are
// only ever written by the lock holder, and a second writer is exactly
// what the lock exists to prevent.
func holdIRCLock(t *testing.T, path string) func() {
	t.Helper()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open lock %s: %v", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		t.Fatalf("take lock %s: %v", path, err)
	}

	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}
}
