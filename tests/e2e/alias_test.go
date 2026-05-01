package e2e_test

import "testing"

func TestAliases_TopLevel(t *testing.T) {
	r := newRunner(t)
	_ = r.mustCreateWorkspace("alias-test")

	wsListRes := r.run("workspace", "list", "--format", "json")
	requireExitCode(t, wsListRes, 0)

	wsRes := r.run("ws", "list", "--format", "json")
	requireExitCode(t, wsRes, 0)

	lsRes := r.run("ws", "ls", "--format", "json")
	requireExitCode(t, lsRes, 0)
}
