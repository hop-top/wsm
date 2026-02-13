//go:build !darwin

package e2e_test

import "testing"

func TestDeleteConfirmBypassNotAppliedOnLinux(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("linux-no-bypass")

	res := r.runWithEnv(map[string]string{"WSM_DELETE_CONFIRM": "yes"}, "delete", "linux-no-bypass")
	requireExitCode(t, res, 2)
}
