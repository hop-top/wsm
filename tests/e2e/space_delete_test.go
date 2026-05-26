package e2e_test

import (
	"strings"
	"testing"
)

// TestSpaceDelete_ConfirmGate exercises kit's --confirm policy on
// `wsm space delete`. T-0122 promoted space delete from SideEffectWrite
// to SideEffectDestructive, so kit's confirmation gate now fires:
//
//   - --confirm=yes  → succeeds (positive control)
//   - --force        → bridges to --confirm=yes + emits deprecation hint
//   - bare           → refused on non-TTY (UNAUTHORIZED, exit 5)
//
// The cascade semantics of --force are preserved and unchanged.
func TestSpaceDelete_ConfirmGate(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("space-confirm")
	tmpA := t.TempDir()
	tmpB := t.TempDir()
	tmpC := t.TempDir()

	// Three spaces, one per scenario, all keyed off the same workspace.
	requireExitCode(t, r.run("space", "add", "space-confirm", "alpha",
		"--path", tmpA, "--type", "filesystem"), 0)
	requireExitCode(t, r.run("space", "add", "space-confirm", "beta",
		"--path", tmpB, "--type", "filesystem"), 0)
	requireExitCode(t, r.run("space", "add", "space-confirm", "gamma",
		"--path", tmpC, "--type", "filesystem"), 0)

	// Positive: --confirm=yes succeeds without --force.
	resYes := r.run("space", "delete", "space-confirm", "alpha",
		"--confirm=yes", "--note", "cleanup", "--format", "json")
	requireExitCode(t, resYes, 0)

	// Bridge: --force still works (back-compat) and prints the
	// deprecation hint to stderr.
	resForce := r.run("space", "delete", "space-confirm", "beta",
		"--force", "--note", "cleanup", "--format", "json")
	requireExitCode(t, resForce, 0)
	if !strings.Contains(strings.ToLower(resForce.Stderr), "--force is deprecated") {
		t.Fatalf("expected --force deprecation hint on stderr, got: %s", resForce.Stderr)
	}

	// Negative: bare invocation on non-TTY → UNAUTHORIZED.
	resBare := r.run("space", "delete", "space-confirm", "gamma",
		"--note", "cleanup")
	requireExitCode(t, resBare, 5)
	if !strings.Contains(strings.ToLower(resBare.Stderr), "unauthorized") &&
		!strings.Contains(strings.ToLower(resBare.Stderr), "confirm=no") {
		t.Fatalf("expected UNAUTHORIZED refusal envelope, got: %s", resBare.Stderr)
	}
}
