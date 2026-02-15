//go:build charmui && !darwin

package e2e_test

import "testing"

func TestTUIScriptBypassNotAppliedOnLinux(t *testing.T) {
	r := newCharmRunner(t)
	res := r.runWithEnv(map[string]string{"WSM_TUI_SCRIPT": "q"}, "tui")
	if res.ExitCode == 0 {
		t.Fatalf("expected non-tty charm tui run with WSM_TUI_SCRIPT to fail on linux; bypass should be disabled")
	}
}
