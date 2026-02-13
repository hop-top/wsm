//go:build charmui

package e2e_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"hop.top/wsm/pkg/workspace"
)

func TestTUILaunch(t *testing.T) {
	r := newCharmRunner(t)
	res := runTUICommand(t, r, "?q")
	requireExitCode(t, res, 0)
	if !strings.Contains(res.Stdout, "wsm TUI") {
		t.Fatalf("expected tui header in output, got:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "Help:") {
		t.Fatalf("expected help copy after scripted ? key, got:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "No workspaces") {
		t.Fatalf("expected empty-state copy in tui output, got:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "DETAIL") {
		t.Fatalf("expected detail pane header in tui output, got:\n%s", res.Stdout)
	}
}

func TestTUIWorkspaceLifecycle(t *testing.T) {
	r := newCharmRunner(t)

	createFromTUI := runTUICommand(t, r, "cq")
	requireExitCode(t, createFromTUI, 0)
	if !strings.Contains(createFromTUI.Stdout, "DETAIL") || !strings.Contains(createFromTUI.Stdout, "Name:") {
		t.Fatalf("expected detail pane content after create flow, got:\n%s", createFromTUI.Stdout)
	}

	list := r.run("list", "--json")
	requireExitCode(t, list, 0)
	items := mustUnmarshalJSONArray[workspace.Workspace](t, list.Stdout)
	if len(items) != 1 {
		t.Fatalf("expected one workspace created by tui flow, got %d", len(items))
	}
	id := items[0].ID

	archive := runTUICommand(t, r, "aq")
	requireExitCode(t, archive, 0)
	inspectArchived := r.run("inspect", id, "--json")
	requireExitCode(t, inspectArchived, 0)
	var archived inspectJSON
	mustUnmarshalJSON(t, inspectArchived.Stdout, &archived)
	if archived.Workspace.Status != workspace.StatusArchived {
		t.Fatalf("expected workspace to be archived by tui, got %s", archived.Workspace.Status)
	}

	unarchive := runTUICommand(t, r, "aq")
	requireExitCode(t, unarchive, 0)
	inspectActive := r.run("inspect", id, "--json")
	requireExitCode(t, inspectActive, 0)
	var active inspectJSON
	mustUnmarshalJSON(t, inspectActive.Stdout, &active)
	if active.Workspace.Status != workspace.StatusActive {
		t.Fatalf("expected workspace to be active after tui unarchive, got %s", active.Workspace.Status)
	}

	deleteFlow := runTUICommand(t, r, "ddq")
	requireExitCode(t, deleteFlow, 0)
	deleted := r.run("inspect", id)
	requireExitCode(t, deleted, 3)
}

func runTUICommand(t *testing.T, r *e2eRunner, script string) commandResult {
	t.Helper()
	if runtime.GOOS == "darwin" {
		return r.runWithEnv(map[string]string{"WSM_TUI_SCRIPT": script}, "--ui=charm", "tui")
	}
	return runTUICommandExpect(t, r, script)
}

func runTUICommandExpect(t *testing.T, r *e2eRunner, script string) commandResult {
	t.Helper()
	if _, err := exec.LookPath("expect"); err != nil {
		t.Skip("expect command is required for Linux TUI interaction tests")
	}

	expectScript := buildExpectScript(r.binary, r.dataDir, script)
	scriptPath := filepath.Join(t.TempDir(), "tui.expect")
	writeFile(t, scriptPath, []byte(expectScript))

	cmd := exec.Command("expect", "-f", scriptPath)
	cmd.Dir = r.root
	cmd.Env = r.baseEnv(nil)
	out, err := cmd.CombinedOutput()
	res := commandResult{Stdout: normalizeOutput(string(out)), ExitCode: 0}
	if err == nil {
		return res
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		res.ExitCode = exitErr.ExitCode()
		return res
	}
	t.Fatalf("run expect failed: %v", err)
	return commandResult{}
}

func buildExpectScript(binary, dataDir, keys string) string {
	var b strings.Builder
	b.WriteString("set timeout 30\n")
	b.WriteString("log_user 1\n")
	b.WriteString(fmt.Sprintf("spawn %s --data-dir %s --ui=charm tui\n", tclQuote(binary), tclQuote(dataDir)))
	b.WriteString("expect \"wsm TUI\"\n")
	for _, k := range keys {
		if k == '\n' {
			b.WriteString("send -- \"\\r\"\n")
		} else if k == '"' {
			b.WriteString("send -- \"\\\"\"\n")
		} else {
			b.WriteString(fmt.Sprintf("send -- \"%c\"\n", k))
		}
		b.WriteString("after 250\n")
	}
	b.WriteString("expect eof\n")
	return b.String()
}

func tclQuote(s string) string {
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return "\"" + escaped + "\""
}
