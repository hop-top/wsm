//go:build charmui

package e2e_test

import "testing"

func TestUIModeSelection_CharmBuild(t *testing.T) {
	r := newCharmRunner(t)
	r.mustCreateWorkspace("charm-ui")

	res := r.run("--ui=charm", "list")
	requireExitCode(t, res, 0)
}
