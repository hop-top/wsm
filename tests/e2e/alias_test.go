package e2e_test

import (
	"strings"
	"testing"
)

// TestAliases_KitStore exercises the kit/console/alias YAML store wired
// in via Root.AliasCmd. Persistence to $XDG_CONFIG_HOME/wsm/aliases.yaml
// is the contract; the runner's per-test xdgConfigHome isolates state.
func TestAliases_KitStore(t *testing.T) {
	r := newRunner(t)

	add := r.runNoGlobal("alias", "add", "wl", "workspace", "list")
	requireExitCode(t, add, 0)
	if !strings.Contains(add.Stdout, "wl") {
		t.Fatalf("expected ack mentioning the alias name, got: %s", add.Stdout)
	}

	list := r.runNoGlobal("alias", "list", "--format", "json")
	requireExitCode(t, list, 0)
	if !strings.Contains(list.Stdout, "\"wl\"") {
		t.Fatalf("alias list missing wl: %s", list.Stdout)
	}

	rm := r.runNoGlobal("alias", "remove", "wl")
	requireExitCode(t, rm, 0)
}
