package e2e_test

import (
	"strings"
	"testing"
)

func TestCompletionBash(t *testing.T) {
	r := newRunner(t)
	res := r.run("completion", "bash")
	requireExitCode(t, res, 0)
	if !strings.Contains(res.Stdout, "__start_wsm") {
		t.Fatalf("expected bash completion function in output, got:\n%s", res.Stdout)
	}
}

func TestCompletionZsh(t *testing.T) {
	r := newRunner(t)
	res := r.run("completion", "zsh")
	requireExitCode(t, res, 0)
	if !strings.Contains(res.Stdout, "#compdef wsm") {
		t.Fatalf("expected zsh completion header in output, got:\n%s", res.Stdout)
	}
}

func TestCompletionWorkspaceAndFlagValues(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("alpha")
	r.mustCreateWorkspace("beta")

	workspaceCompletion := r.run("__complete", "workspace", "show", "")
	requireExitCode(t, workspaceCompletion, 0)
	if !strings.Contains(workspaceCompletion.Stdout, "alpha") || !strings.Contains(workspaceCompletion.Stdout, "beta") {
		t.Fatalf("expected workspace names in completion output, got:\n%s", workspaceCompletion.Stdout)
	}

	snapshotCompletion := r.run("__complete", "snapshot", "generate", "")
	requireExitCode(t, snapshotCompletion, 0)
	if !strings.Contains(snapshotCompletion.Stdout, "alpha") || !strings.Contains(snapshotCompletion.Stdout, "beta") {
		t.Fatalf("expected workspace names in snapshot completion output, got:\n%s", snapshotCompletion.Stdout)
	}

	statusCompletion := r.run("__complete", "workspace", "list", "--status=")
	requireExitCode(t, statusCompletion, 0)
	if !strings.Contains(statusCompletion.Stdout, "active") || !strings.Contains(statusCompletion.Stdout, "archived") {
		t.Fatalf("expected status values in completion output, got:\n%s", statusCompletion.Stdout)
	}

	eventTypeCompletion := r.run("__complete", "event", "add", "alpha", "")
	requireExitCode(t, eventTypeCompletion, 0)
	if !strings.Contains(eventTypeCompletion.Stdout, "interaction.message") || !strings.Contains(eventTypeCompletion.Stdout, "mutation.file_patch") {
		t.Fatalf("expected event type values in completion output, got:\n%s", eventTypeCompletion.Stdout)
	}
}
