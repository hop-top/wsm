package e2e_test

import (
	"strings"
	"testing"

	"hop.top/wsm/pkg/workspace"
)

func TestJSON_Create(t *testing.T) {
	r := newRunner(t)
	res := r.run("workspace", "add", "json-create", "--format", "json")
	requireExitCode(t, res, 0)
	var payload map[string]any
	mustUnmarshalJSON(t, res.Stdout, &payload)
	if payload["id"] == "" || payload["created_at"] == "" {
		t.Fatalf("expected create json payload with id/created_at, got: %s", res.Stdout)
	}
}

func TestJSON_List(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("json-list")
	res := r.run("workspace", "list", "--format", "json")
	requireExitCode(t, res, 0)
	items := mustUnmarshalJSONArray[workspace.Workspace](t, res.Stdout)
	if len(items) != 1 || items[0].Name != "json-list" {
		t.Fatalf("unexpected list json output: %+v", items)
	}
}

func TestJSON_Show(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("json-show")
	res := r.run("workspace", "show", id, "--format", "json")
	requireExitCode(t, res, 0)
	var payload inspectJSON
	mustUnmarshalJSON(t, res.Stdout, &payload)
	if payload.Workspace.ID != id {
		t.Fatalf("expected show workspace id %s, got %s", id, payload.Workspace.ID)
	}
	if payload.EventCount == 0 {
		t.Fatalf("expected non-zero event_count for show output, got %d", payload.EventCount)
	}
}

func TestJSON_Archive(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("json-archive")
	// archive-requires-note default policy (T-1288) gates archive on a
	// non-empty --note.
	res := r.run("workspace", "archive", id, "--note", "json test", "--format", "json")
	requireExitCode(t, res, 0)
	var payload map[string]any
	mustUnmarshalJSON(t, res.Stdout, &payload)
	if payload["id"] != id {
		t.Fatalf("expected archive payload id %s, got %+v", id, payload)
	}
	if payload["archived_at"] == "" {
		t.Fatalf("expected archived_at in archive payload, got %+v", payload)
	}
}

func TestJSON_ErrorIsolation(t *testing.T) {
	r := newRunner(t)
	res := r.run("workspace", "show", "missing-workspace", "--format", "json")
	requireExitCode(t, res, 3)
	if strings.TrimSpace(res.Stdout) != "" {
		t.Fatalf("expected stdout to stay empty on json error, got: %q", res.Stdout)
	}
	if strings.TrimSpace(res.Stderr) == "" {
		t.Fatalf("expected stderr to include error details")
	}
}

func TestPipe_NoANSI(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("pipe-no-ansi")
	res := r.run("workspace", "list")
	requireExitCode(t, res, 0)
	if hasANSI(res.Stdout) || hasANSI(res.Stderr) {
		t.Fatalf("expected no ANSI escapes in non-tty output:\nstdout=%q\nstderr=%q", res.Stdout, res.Stderr)
	}
}

func TestPipe_QuietIds(t *testing.T) {
	r := newRunner(t)
	idA := r.mustCreateWorkspace("pipe-a")
	idB := r.mustCreateWorkspace("pipe-b")

	res := r.run("workspace", "list", "--quiet")
	requireExitCode(t, res, 0)
	lines := sortedLines(res.Stdout)
	if len(lines) != 2 {
		t.Fatalf("expected two ids in quiet output, got %d:\n%s", len(lines), res.Stdout)
	}
	if lines[0] != idA && lines[0] != idB {
		t.Fatalf("unexpected quiet output ids: %v", lines)
	}
	if lines[1] != idA && lines[1] != idB {
		t.Fatalf("unexpected quiet output ids: %v", lines)
	}
}

// TestPipe_RemoveRequiresConfirm asserts that destructive `workspace
// delete` on a non-TTY (piped) invocation, without --confirm=yes or
// --force (the legacy bridge), is refused by kit's policy gate. T-0122
// moves the gate from a wsm-local usage error (exit 2) to kit's
// UNAUTHORIZED envelope (exit 5).
func TestPipe_RemoveRequiresConfirm(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("pipe-remove")

	res := r.run("workspace", "delete", "pipe-remove")
	requireExitCode(t, res, 5)
	if !strings.Contains(strings.ToLower(res.Stderr), "confirm=no") &&
		!strings.Contains(strings.ToLower(res.Stderr), "unauthorized") {
		t.Fatalf("expected non-tty delete confirm-gate refusal, got: %s", res.Stderr)
	}
}

func TestNoColorFlag(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("no-color-flag")

	res := r.run("--no-color", "workspace", "list")
	requireExitCode(t, res, 0)
	if hasANSI(res.Stdout) || hasANSI(res.Stderr) {
		t.Fatalf("expected no ANSI escapes with --no-color:\nstdout=%q\nstderr=%q", res.Stdout, res.Stderr)
	}
}

func TestNoColorEnv(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("no-color-env")

	res := r.exec(runOptions{
		Args:     []string{"--data-dir", r.dataDir, "workspace", "list"},
		TTY:      true,
		ExtraEnv: map[string]string{"NO_COLOR": "1"},
	})
	requireExitCode(t, res, 0)
	if hasANSI(res.Stdout) || hasANSI(res.Stderr) {
		t.Fatalf("expected NO_COLOR tty output without ANSI escapes:\nstdout=%q\nstderr=%q", res.Stdout, res.Stderr)
	}
}
