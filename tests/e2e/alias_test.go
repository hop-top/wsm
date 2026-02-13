package e2e_test

import "testing"

func TestAliases_TopLevel(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("alias-test")

	// wsm ls should work like wsm list
	lsRes := r.run("ls", "--json")
	requireExitCode(t, lsRes, 0)

	// wsm i <ws> should work like wsm inspect
	iRes := r.run("i", wsID, "--json")
	requireExitCode(t, iRes, 0)

	// wsm info <ws> should work like wsm inspect
	infoRes := r.run("info", wsID, "--json")
	requireExitCode(t, infoRes, 0)
}
