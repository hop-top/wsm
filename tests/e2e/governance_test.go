package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryFoundation(t *testing.T) {
	r := newRunner(t)

	required := []string{
		"README.md",
		"AGENTS.md",
		"LICENSE",
		"CONTRIBUTING.md",
		"SECURITY.md",
		"CODE_OF_CONDUCT.md",
		"CHANGELOG.md",
		".github/ISSUE_TEMPLATE/bug_report.md",
		".github/ISSUE_TEMPLATE/feature_request.md",
		".github/pull_request_template.md",
		"Makefile",
	}
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(r.root, rel)); err != nil {
			t.Fatalf("missing governance/repo foundation file %s: %v", rel, err)
		}
	}

	help := r.runGo("run", "./cmd/wsm", "--help")
	requireExitCode(t, help, 0)
	if !strings.Contains(help.Stdout, "Workspace & Session Manager") {
		t.Fatalf("expected CLI help header, got:\n%s", help.Stdout)
	}
}

func TestMakeHelpTargets(t *testing.T) {
	r := newRunner(t)
	makeHelp := runMake(t, r.root, "help")
	requireExitCode(t, makeHelp, 0)
	for _, target := range []string{"build", "build-charm", "test", "verify-no-ui-leak", "fmt"} {
		if !strings.Contains(makeHelp.Stdout, target) {
			t.Fatalf("expected make help to include target %q, got:\n%s", target, makeHelp.Stdout)
		}
	}
}

func runMake(t *testing.T, root string, args ...string) commandResult {
	t.Helper()
	cmd := exec.Command("make", args...)
	cmd.Dir = root
	cmd.Env = upsertEnv(os.Environ(), "GOCACHE", filepath.Join(root, ".tmp", "gocache"))
	out, err := cmd.CombinedOutput()
	res := commandResult{Stdout: normalizeOutput(string(out)), ExitCode: 0}
	if err == nil {
		return res
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		res.ExitCode = exitErr.ExitCode()
		return res
	}
	t.Fatalf("run make failed: %v", err)
	return commandResult{}
}
