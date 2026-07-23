package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"hop.top/wsm/pkg/workspace"
)

// runWorkspaceAdd executes the `workspace add` cobra subcommand with
// the given args and captures the cobra writers. Quiet mode (set by
// newTestEnv) keeps stdout to the bare ID, so we don't need the
// os.Stdout pipe dance here.
func runWorkspaceAdd(t *testing.T, env *testEnv, args ...string) (stderr string, err error) {
	t.Helper()
	cmd := newWorkspaceAddCmd(env.opts)
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return errBuf.String(), err
}

func runWorkspaceUpdate(t *testing.T, env *testEnv, args ...string) (stderr string, err error) {
	t.Helper()
	cmd := newWorkspaceUpdateCmd(env.opts)
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return errBuf.String(), err
}

func layoutOf(t *testing.T, env *testEnv, ref string) *workspace.Layout {
	t.Helper()
	mgr, cleanup := env.openManager(t)
	defer cleanup()
	ws, err := mgr.ResolveWorkspaceRef(context.Background(), ref)
	if err != nil {
		t.Fatalf("resolve %q: %v", ref, err)
	}
	return ws.Layout
}

// orgDir creates a temp dir, optionally seeding a COMPANY.md marker.
func orgDir(t *testing.T, withMarker bool) string {
	t.Helper()
	dir := t.TempDir()
	if withMarker {
		if err := os.WriteFile(filepath.Join(dir, "COMPANY.md"), []byte("# co\n"), 0o644); err != nil {
			t.Fatalf("write marker: %v", err)
		}
	}
	return dir
}

func TestWorkspaceAdd_OrgPackageMarkerDetectsKind(t *testing.T) {
	env := newTestEnv(t)
	dir := orgDir(t, true)

	if _, err := runWorkspaceAdd(t, env, "org-ws", "--org-package", dir); err != nil {
		t.Fatalf("add --org-package: %v", err)
	}
	l := layoutOf(t, env, "org-ws")
	if l == nil {
		t.Fatal("expected layout attached")
	}
	if l.Kind != "agentcompanies/v1" {
		t.Fatalf("expected kind agentcompanies/v1, got %q", l.Kind)
	}
	if l.Path != dir {
		t.Fatalf("expected path stored as given (%q), got %q", dir, l.Path)
	}
}

func TestWorkspaceAdd_OrgPackageNoMarkerUnknownKind(t *testing.T) {
	env := newTestEnv(t)
	dir := orgDir(t, false)

	if _, err := runWorkspaceAdd(t, env, "org-ws", "--org-package", dir); err != nil {
		t.Fatalf("add --org-package: %v", err)
	}
	l := layoutOf(t, env, "org-ws")
	if l == nil || l.Kind != "unknown" {
		t.Fatalf("expected kind unknown without marker, got %+v", l)
	}
}

func TestWorkspaceAdd_OrgPackageKindOverrideWins(t *testing.T) {
	env := newTestEnv(t)
	dir := orgDir(t, true) // marker present, override must still win

	if _, err := runWorkspaceAdd(t, env, "org-ws",
		"--org-package", dir, "--org-package-kind", "custom/v2"); err != nil {
		t.Fatalf("add --org-package --org-package-kind: %v", err)
	}
	l := layoutOf(t, env, "org-ws")
	if l == nil || l.Kind != "custom/v2" {
		t.Fatalf("expected override kind custom/v2, got %+v", l)
	}
}

func TestWorkspaceAdd_OrgPackageKindOnlyFromMarkerFile(t *testing.T) {
	env := newTestEnv(t)
	// Content that could tempt heuristics — near-miss names, yaml
	// manifests, kind strings inside files — must NOT be treated as
	// a marker. (Lowercase company.md is excluded: case-insensitive
	// filesystems make it indistinguishable from the marker itself.)
	dir := t.TempDir()
	for name, body := range map[string]string{
		"COMPANY.yaml":  "kind: agentcompanies/v1\n",
		"COMPANIES.md":  "# plural, not the marker\n",
		"org.md":        "agentcompanies/v1\n",
		"agentco.marks": "agentcompanies/v1\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if _, err := runWorkspaceAdd(t, env, "org-ws", "--org-package", dir); err != nil {
		t.Fatalf("add --org-package: %v", err)
	}
	l := layoutOf(t, env, "org-ws")
	if l == nil || l.Kind != "unknown" {
		t.Fatalf("expected kind unknown (COMPANY.md is the only marker), got %+v", l)
	}
}

func TestWorkspaceAdd_NoOrgPackageNoLayout(t *testing.T) {
	env := newTestEnv(t)

	if _, err := runWorkspaceAdd(t, env, "plain-ws"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if l := layoutOf(t, env, "plain-ws"); l != nil {
		t.Fatalf("expected no layout without --org-package, got %+v", l)
	}
}

func TestWorkspaceUpdate_OrgPackageAttach(t *testing.T) {
	env := newTestEnv(t)
	dir := orgDir(t, true)

	if _, err := runWorkspaceUpdate(t, env, "ws", "--org-package", dir); err != nil {
		t.Fatalf("update --org-package: %v", err)
	}
	l := layoutOf(t, env, "ws")
	if l == nil || l.Kind != "agentcompanies/v1" || l.Path != dir {
		t.Fatalf("expected attached layout agentcompanies/v1 at %q, got %+v", dir, l)
	}
}

func TestWorkspaceUpdate_OrgPackageDashDetaches(t *testing.T) {
	env := newTestEnv(t)
	setLayoutFor(t, env, "ws", &workspace.Layout{Kind: "agentcompanies/v1", Path: "org/"})

	if _, err := runWorkspaceUpdate(t, env, "ws", "--org-package", "-"); err != nil {
		t.Fatalf("update --org-package -: %v", err)
	}
	if l := layoutOf(t, env, "ws"); l != nil {
		t.Fatalf("expected layout detached, got %+v", l)
	}
}

func TestWorkspaceUpdate_NoFlagsUsageError(t *testing.T) {
	env := newTestEnv(t)

	if _, err := runWorkspaceUpdate(t, env, "ws"); err == nil {
		t.Fatal("expected usage error when no mutation flags given")
	}
}
