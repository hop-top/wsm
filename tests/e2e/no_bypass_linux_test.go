//go:build !darwin

package e2e_test

import "testing"

// TestRemoveConfirmBypassNotAppliedOnLinux retains the original intent:
// the darwin-only WSM_DELETE_CONFIRM env var bypass must NOT leak to
// Linux. T-0122 removed that env var entirely (kit's --confirm=yes is
// now the canonical bypass), but the env var should still be ignored on
// Linux — confirmed indirectly because the delete is refused by kit's
// gate (UNAUTHORIZED, exit 5) when no --confirm/--force is passed.
func TestRemoveConfirmBypassNotAppliedOnLinux(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("linux-no-bypass")

	res := r.runWithEnv(map[string]string{"WSM_DELETE_CONFIRM": "yes"}, "workspace", "delete", "linux-no-bypass")
	requireExitCode(t, res, 5)
}
