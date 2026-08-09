package e2e_test

import (
	"strings"
	"testing"
)

// Machine-grade conformance surface: classified exit codes for unknown
// commands, a closed --format contract, a live capabilities manifest,
// and --dry-run previews that apply nothing. These behaviors carry the
// F4/F5/F6/F11 verdicts in e2e/conformance/; the tests here pin them at
// the process boundary.

// TestUnknownCommand_RootIsUsage locks CLI conv §8.1: an unmatched
// top-level command is a usage error (exit 2), never help + exit 0 and
// never the generic exit 1.
func TestUnknownCommand_RootIsUsage(t *testing.T) {
	r := newRunner(t)
	res := r.run("frobnicate")
	requireExitCode(t, res, 2)
	if !strings.Contains(strings.ToLower(res.Stderr), "unknown command") {
		t.Fatalf("expected stderr to name the unknown command, got:\n%s", res.Stderr)
	}
}

// TestUnknownCommand_GroupIsUsage locks the same contract for group
// nodes: an unmatched positional on a group renders a corrective usage
// envelope (exit 2) instead of cobra's help-and-exit-0 (config group)
// or a generic exit 1 (workspace group).
func TestUnknownCommand_GroupIsUsage(t *testing.T) {
	for _, group := range []string{"workspace", "space", "event", "device", "snapshot", "config"} {
		t.Run(group, func(t *testing.T) {
			r := newRunner(t)
			res := r.run(group, "frobnicate")
			requireExitCode(t, res, 2)
			if !strings.Contains(strings.ToLower(res.Stderr), "unknown command") {
				t.Fatalf("expected stderr to name the unknown command, got:\n%s", res.Stderr)
			}
			if !strings.Contains(res.Stderr, "--help") {
				t.Fatalf("expected corrective --help pointer on stderr, got:\n%s", res.Stderr)
			}
		})
	}
}

// TestUnknownCommand_BareGroupKeepsHelpPath ensures the usage hardening
// did not break the bare-invocation help path: `wsm config` renders
// help and exits 0.
func TestUnknownCommand_BareGroupKeepsHelpPath(t *testing.T) {
	r := newRunner(t)
	res := r.run("config")
	requireExitCode(t, res, 0)
}

// TestFormatContract_InvalidRefused locks the closed output contract:
// a --format value outside kit's formatter registry is refused loudly
// as a usage error (exit 2), never silently coerced to table.
func TestFormatContract_InvalidRefused(t *testing.T) {
	r := newRunner(t)
	res := r.run("workspace", "list", "--format", "bogus")
	requireExitCode(t, res, 2)
	if !strings.Contains(res.Stderr, "invalid --format") {
		t.Fatalf("expected 'invalid --format' diagnostic on stderr, got:\n%s", res.Stderr)
	}
}

// TestCapabilitiesManifest locks the machine-readable command surface:
// `wsm capabilities --format json` emits the live command tree with
// side-effect and idempotency contracts per leaf, exiting clean.
func TestCapabilitiesManifest(t *testing.T) {
	r := newRunner(t)
	res := r.run("capabilities", "--format", "json")
	requireExitCode(t, res, 0)

	var doc struct {
		Tool          string `json:"tool"`
		SchemaVersion string `json:"schema_version"`
		Leaves        []struct {
			Path       string `json:"path"`
			SideEffect string `json:"side_effect"`
			Idempotent string `json:"idempotent"`
			DryRun     bool   `json:"dry_run_supported"`
		} `json:"leaves"`
	}
	mustUnmarshalJSON(t, res.Stdout, &doc)

	if doc.Tool != "wsm" {
		t.Fatalf("expected tool=wsm, got %q", doc.Tool)
	}
	if doc.SchemaVersion == "" {
		t.Fatal("expected a schema_version on the capabilities document")
	}
	byPath := map[string]int{}
	for i, l := range doc.Leaves {
		byPath[l.Path] = i
	}
	for _, want := range []string{
		"wsm workspace add",
		"wsm workspace list",
		"wsm workspace delete",
		"wsm snapshot generate",
		"wsm event add",
		"wsm capabilities",
	} {
		if _, ok := byPath[want]; !ok {
			t.Fatalf("expected leaf %q in capabilities manifest; have %d leaves", want, len(doc.Leaves))
		}
	}
	if got := doc.Leaves[byPath["wsm workspace add"]].SideEffect; got != "write" {
		t.Fatalf("expected wsm workspace add side_effect=write, got %q", got)
	}
	if got := doc.Leaves[byPath["wsm workspace list"]].SideEffect; got != "read" {
		t.Fatalf("expected wsm workspace list side_effect=read, got %q", got)
	}
	if got := doc.Leaves[byPath["wsm workspace delete"]].SideEffect; got != "destructive" {
		t.Fatalf("expected wsm workspace delete side_effect=destructive, got %q", got)
	}
}

// TestDryRun_WorkspaceAddPreviewsWithoutWriting locks F6: --dry-run on
// a write leaf emits a structured plan naming the effect and applies
// nothing — the follow-up listing stays empty.
func TestDryRun_WorkspaceAddPreviewsWithoutWriting(t *testing.T) {
	r := newRunner(t)

	res := r.run("workspace", "add", "preview-ws", "--dry-run", "--format", "json")
	requireExitCode(t, res, 0)

	var plan struct {
		Command string `json:"command"`
		Effects []struct {
			Kind   string `json:"kind"`
			Target string `json:"target"`
		} `json:"effects"`
	}
	mustUnmarshalJSON(t, res.Stdout, &plan)
	if plan.Command != "wsm workspace add" {
		t.Fatalf("expected plan.command 'wsm workspace add', got %q", plan.Command)
	}
	if len(plan.Effects) == 0 || plan.Effects[0].Kind != "create" {
		t.Fatalf("expected a create effect in the plan, got: %+v", plan.Effects)
	}
	if !strings.Contains(plan.Effects[0].Target, "preview-ws") {
		t.Fatalf("expected the effect target to name the workspace, got %q", plan.Effects[0].Target)
	}

	list := r.run("workspace", "list", "--format", "json")
	requireExitCode(t, list, 0)
	var items []any
	mustUnmarshalJSON(t, list.Stdout, &items)
	if len(items) != 0 {
		t.Fatalf("expected no workspaces after --dry-run add, got %d", len(items))
	}
}

// TestDryRun_MutatingLeavesPreviewWithoutWriting sweeps the remaining
// wsm-owned mutating leaves: each honors --dry-run by exiting clean
// with a plan on stdout and leaving state untouched.
func TestDryRun_MutatingLeavesPreviewWithoutWriting(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("dryrun-target")
	_ = id

	cases := []struct {
		name string
		args []string
	}{
		{"workspace_archive", []string{"workspace", "archive", "dryrun-target", "--note", "preview"}},
		{"workspace_update", []string{"workspace", "update", "dryrun-target", "--org-package", "pkgdir"}},
		{"event_add", []string{"event", "add", "dryrun-target", "interaction.message", "--role", "user", "--content", "hi"}},
		{"snapshot_generate", []string{"snapshot", "generate", "dryrun-target"}},
		{"space_add", []string{"space", "add", "dryrun-target", "spc-1", "--path", "/tmp/spc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, tc.args...), "--dry-run", "--format", "json")
			res := r.run(args...)
			requireExitCode(t, res, 0)
			var plan struct {
				Effects []struct {
					Kind string `json:"kind"`
				} `json:"effects"`
			}
			mustUnmarshalJSON(t, res.Stdout, &plan)
			if len(plan.Effects) == 0 {
				t.Fatalf("expected at least one planned effect, got stdout:\n%s", res.Stdout)
			}
		})
	}

	// Nothing above may have mutated state: still exactly one
	// workspace, zero snapshots, zero spaces, and only the creation
	// event on the log.
	show := r.run("workspace", "show", "dryrun-target", "--format", "json")
	requireExitCode(t, show, 0)
	var detail struct {
		EventCount    int `json:"event_count"`
		SnapshotCount int `json:"snapshot_count"`
		Workspace     struct {
			Status string `json:"status"`
			Layout any    `json:"layout"`
		} `json:"workspace"`
	}
	mustUnmarshalJSON(t, show.Stdout, &detail)
	if detail.EventCount != 1 {
		t.Fatalf("expected only the creation event after dry-run sweep, got %d", detail.EventCount)
	}
	if detail.SnapshotCount != 0 {
		t.Fatalf("expected no snapshots after dry-run sweep, got %d", detail.SnapshotCount)
	}
	if detail.Workspace.Status != "active" {
		t.Fatalf("expected workspace still active after dry-run archive, got %q", detail.Workspace.Status)
	}
	if detail.Workspace.Layout != nil {
		t.Fatalf("expected no layout after dry-run update, got %v", detail.Workspace.Layout)
	}

	spaces := r.run("space", "list", "dryrun-target", "--format", "json")
	requireExitCode(t, spaces, 0)
	var spaceItems []any
	mustUnmarshalJSON(t, spaces.Stdout, &spaceItems)
	if len(spaceItems) != 0 {
		t.Fatalf("expected no spaces after dry-run add, got %d", len(spaceItems))
	}
}
