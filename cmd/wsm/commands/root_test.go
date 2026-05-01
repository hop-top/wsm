package commands

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
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
