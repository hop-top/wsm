package commands

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"hop.top/kit/go/console/cli"
)

// executeCommand invokes the root command with the given args and
// returns the combined stdout + stderr. Drains stdout in a goroutine
// so a large help dump can't deadlock on the pipe buffer.
func executeCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	root := NewRootCmd()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)

	pr, pw := io.Pipe()
	var drained bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&drained, pr)
	}()

	root.SetOut(io.MultiWriter(&outBuf, pw))
	err := root.Execute()
	_ = pw.Close()
	wg.Wait()

	return outBuf.String() + errBuf.String() + drained.String(), err
}

// TestHelpAll asserts the kit/cli auto-provided --help-all flag is
// registered on the wsm root. This is the contract test for adopting
// cli.New: the flag exists and Lookup returns non-nil.
func TestHelpAll(t *testing.T) {
	root := NewRootCmd()
	if f := root.Flags().Lookup("help-all"); f == nil {
		t.Fatalf("expected --help-all flag on root; not registered")
	}

	out, err := executeCommand(t, "--help-all")
	if err != nil {
		t.Fatalf("--help-all returned error: %v", err)
	}
	if !strings.Contains(out, "wsm") {
		t.Fatalf("--help-all output missing program name; got: %s", out)
	}
}

// TestCommandGroupsApplied verifies every top-level subcommand named in
// commandGroups has its GroupID set to the mapped value. Commands not in
// the map are intentionally ungrouped and skipped.
func TestCommandGroupsApplied(t *testing.T) {
	root := NewRootCmd()
	for _, c := range root.Commands() {
		wantGroup, expected := commandGroups[c.Name()]
		if !expected {
			continue
		}
		if c.GroupID != wantGroup {
			t.Errorf("command %q: GroupID=%q, want %q",
				c.Name(), c.GroupID, wantGroup)
		}
	}
}

// TestGroupCmdUnknownSubcommandErrors regression-tests T-0096: a group
// command (workspace, space, snapshot, event, device) invoked with an
// unknown subcommand must return a usage error, not silently print help
// with exit 0. CLI conventions §8.1.
func TestGroupCmdUnknownSubcommandErrors(t *testing.T) {
	groups := []string{"workspace", "space", "snapshot", "event", "device"}
	for _, grp := range groups {
		t.Run(grp+"/unknown", func(t *testing.T) {
			_, err := executeCommand(t, grp, "bogus_subcommand_xyz")
			if err == nil {
				t.Fatalf("%s: expected error for unknown subcommand, got nil", grp)
			}
			if !strings.Contains(err.Error(), "unknown command") {
				t.Fatalf("%s: error %q missing 'unknown command'", grp, err.Error())
			}
		})
		t.Run(grp+"/bare", func(t *testing.T) {
			_, err := executeCommand(t, grp)
			if err != nil {
				t.Fatalf("%s: bare invocation should not error, got %v", grp, err)
			}
		})
	}
}

// TestLayerAAnnotationsPresent locks in the kit Layer-A annotation
// coverage. Without this guard, a future PR could silently delete an
// annotation; the breakage would only surface when DisableValidate is
// flipped to false in the gate-flip follow-up.
//
// Asserts every leaf the kit shape validator scrutinizes carries
// kit/side-effect, and every depth-1 runnable leaf carries
// kit/top-level-verb. kit/idempotent is auto-filled from kit's
// defaultIdempotency table on Validate(), so this test does not
// enforce it; the gate-flip follow-up's validator run will.
func TestLayerAAnnotationsPresent(t *testing.T) {
	root := NewRootCmd()

	walkDepth(root, 0, func(cmd *cobra.Command, depth int) {
		if cmd == root || isCobraBuiltin(cmd) {
			return
		}
		// Kit-reserved status is built-in to kit; its annotations are
		// kit's responsibility, not wsm's.
		if depth == 1 && cmd.Name() == "status" {
			return
		}
		if !cmd.Runnable() {
			return
		}
		if depth == 1 && !cli.IsTopLevelVerb(cmd) {
			t.Errorf("depth-1 verb %q missing kit/top-level-verb", cmd.CommandPath())
		}
		// Only enforce side-effect on actual leaves (no children) —
		// group nodes with a help-RunE shim aren't the validator's
		// concern.
		if len(cmd.Commands()) == 0 {
			if _, ok := cli.GetSideEffect(cmd); !ok {
				t.Errorf("leaf %q missing kit/side-effect", cmd.CommandPath())
			}
		}
	})
}

// TestTopLevelVerbCountLock pins the depth-1 runnable verb count at
// its steady-state value. kit's shape validator caps depth-1 verbs at
// MaxTopLevelVerbs=10 but excludes reserved subcommands (`status`,
// future kit-shipped reserved verbs) from the count — see
// kit's shape validator, but excludes reserved subcommands (`status`,
// future kit-shipped reserved verbs) from the count. Locking the count
// here prevents an unintended verb sneaking in unobserved.
//
// The steady-state count is 12, two over kit's default cap of 10, so
// NewRootCmd raises Config.MaxTopLevelVerbs to match. Both additions
// are deliberate:
//
//   - capabilities: the machine-readable command surface agents read
//     instead of scraping help text. Must be a depth-1 verb — it
//     describes the whole tree, so it belongs to no noun group.
//   - config: pre-existing group, now runnable so an unmatched
//     positional is a usage error (exit 2) instead of cobra's
//     help-and-exit-0. Runnability is what makes it count; the verb
//     itself is not new surface.
//
// Update this constant when intentionally adding or removing a
// top-level verb.
func TestTopLevelVerbCountLock(t *testing.T) {
	const want = 12

	root := NewRootCmd()
	// kitRootInstance is populated by NewRootCmd. Use its IsReserved
	// directly so the test tracks kit's reservedSubcommands set without
	// duplicating it here — kit owns the source of truth.
	if kitRootInstance == nil {
		t.Fatal("kitRootInstance not populated after NewRootCmd()")
	}
	got := 0
	for _, c := range root.Commands() {
		if isCobraBuiltin(c) {
			continue
		}
		if kitRootInstance.IsReserved(c.Name()) {
			continue
		}
		if !c.Runnable() {
			continue
		}
		got++
	}
	if got != want {
		t.Fatalf("top-level verb count: got=%d want=%d (intentional change? update the constant)", got, want)
	}
}

// TestFormatFlagOnlyOnRoot regression-locks T-0132: kit v0.4.0-alpha.4's
// "defer --format to inherited root global" fix (PR hop-top/poly-kit#80)
// means subcommands must NOT declare their own --format flag — kit
// registers --format as a persistent flag on the root via
// output.RegisterFlags, and cobra inherits it through the parent chain.
//
// A per-subcommand --format declaration would shadow the inherited
// persistent flag and break viper binding, silently regressing the
// alpha.4 fix. This test walks every command in the tree and asserts
// that the only command whose *local* flag set carries --format is the
// root.
func TestFormatFlagOnlyOnRoot(t *testing.T) {
	root := NewRootCmd()

	walkDepth(root, 0, func(cmd *cobra.Command, depth int) {
		if cmd == root || isCobraBuiltin(cmd) {
			return
		}
		// LocalFlags excludes inherited persistent flags; PersistentFlags
		// on the subcommand itself would also constitute a redundant
		// shim. Check both.
		if f := cmd.LocalFlags().Lookup("format"); f != nil {
			t.Errorf("subcommand %q declares its own --format flag (local); "+
				"remove the shim — kit registers --format as a root persistent flag",
				cmd.CommandPath())
		}
		if f := cmd.PersistentFlags().Lookup("format"); f != nil {
			t.Errorf("subcommand %q declares its own --format flag (persistent); "+
				"remove the shim — kit registers --format as a root persistent flag",
				cmd.CommandPath())
		}
	})
}

// walkDepth visits every command in the subtree rooted at cmd in DFS
// order, invoking visit with the relative depth (root = 0).
func walkDepth(cmd *cobra.Command, depth int, visit func(*cobra.Command, int)) {
	visit(cmd, depth)
	for _, child := range cmd.Commands() {
		walkDepth(child, depth+1, visit)
	}
}

// isCobraBuiltin matches kit's built-in classification: completion,
// help, __complete* and completion shell sub-leaves. Status is
// reserved by kit but not built-in (wsm mounts it via WithStatus).
func isCobraBuiltin(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "completion", "help", "__complete", "__completeNoDesc":
		return true
	}
	if p := cmd.Parent(); p != nil && p.Name() == "completion" {
		return true
	}
	return false
}
