package e2e_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExitCode_Success(t *testing.T) {
	r := newRunner(t)
	res := r.run("create", "exit-success")
	requireExitCode(t, res, 0)
}

func TestExitCode_Usage(t *testing.T) {
	r := newRunner(t)
	res := r.run("create")
	requireExitCode(t, res, 2)
}

func TestExitCode_NotFound(t *testing.T) {
	r := newRunner(t)
	res := r.run("inspect", "missing")
	requireExitCode(t, res, 3)
}

func TestExitCode_Conflict(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("exit-conflict")

	duplicate := r.run("create", "exit-conflict")
	requireExitCode(t, duplicate, 4)

	requireExitCode(t, r.run("archive", "exit-conflict"), 0)
	archivedConflict := r.run("archive", "exit-conflict")
	requireExitCode(t, archivedConflict, 4)
}

func TestExitCode_BackendError(t *testing.T) {
	r := newRunner(t)
	bad := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatalf("create bad data dir path: %v", err)
	}

	res := r.runNoGlobal("--data-dir", bad, "list")
	requireExitCode(t, res, 5)
}
