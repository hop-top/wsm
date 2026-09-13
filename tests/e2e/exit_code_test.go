package e2e_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExitCode_Success(t *testing.T) {
	r := newRunner(t)
	res := r.run("workspace", "add", "exit-success")
	requireExitCode(t, res, 0)
}

func TestExitCode_Usage(t *testing.T) {
	r := newRunner(t)
	res := r.run("workspace", "add")
	requireExitCode(t, res, 2)
}

func TestExitCode_NotFound(t *testing.T) {
	r := newRunner(t)
	res := r.run("workspace", "show", "missing")
	requireExitCode(t, res, 3)
}

func TestExitCode_Conflict(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("exit-conflict")

	duplicate := r.run("workspace", "add", "exit-conflict")
	requireExitCode(t, duplicate, 4)

	requireExitCode(t, r.run("workspace", "archive", "exit-conflict", "--note", "stale"), 0)
	archivedConflict := r.run("workspace", "archive", "exit-conflict", "--note", "again")
	requireExitCode(t, archivedConflict, 4)
}

func TestExitCode_BackendError(t *testing.T) {
	// Story 015 scenario 5: backend / persistence failures exit 8.
	// Both real backend faults (unwriteable --data-dir) and
	// unrecognized --backend values are semantically Backend errors and
	// must land on 8, not the usage slot 2.
	t.Run("unwriteable_data_dir", func(t *testing.T) {
		r := newRunner(t)
		bad := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
			t.Fatalf("create bad data dir path: %v", err)
		}
		res := r.runNoGlobal("--data-dir", bad, "workspace", "list")
		requireExitCode(t, res, 8)
	})

	t.Run("unrecognized_backend", func(t *testing.T) {
		r := newRunner(t)
		res := r.runNoGlobal("--backend", "nonexistent", "workspace", "list")
		requireExitCode(t, res, 8)
	})
}
