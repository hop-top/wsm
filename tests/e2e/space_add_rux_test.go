//go:build integration_wsm

package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// projectsFile mirrors the kit/go/core/projects schema enough to
// validate the wsm-side write. We keep this struct local to avoid
// importing kit (the kit projects package is being authored in
// parallel and may not be importable when this test first lands).
type projectsFile struct {
	Schema   int                       `yaml:"schema"`
	Projects map[string]projectsRecord `yaml:"projects"`
}

type projectsRecord struct {
	Path       string `yaml:"path"`
	StartupCmd string `yaml:"startup_cmd,omitempty"`
	Source     string `yaml:"source,omitempty"`
}

// TestE2E_SpaceAdd_WritesProjectsYaml exercises the wsm side of the
// rux-connect chain end-to-end:
//
//  1. `wsm workspace add ic-ops`      — workspace exists
//  2. `wsm space add ic-ops ops --path <tmp> --startup-cmd zsh`
//     — DB row created AND projects.yaml entry written
//  3. `wsm space add ic-ops second --path <tmp2>`
//     — second entry preserves first
//  4. `wsm space sync`                — replays both entries
//
// Read `$XDG_CONFIG_HOME/rux/projects.yaml` and assert both entries
// parse with correct path, startup_cmd, and source=wsm.
//
// Coordination: the actual `rux connect` happens in rux's own e2e
// suite (TestE2E_WsmIntegration in rux/tests/). This test verifies
// only the wsm-written contract. It tolerates the kit projects
// package being unavailable: if the file is absent and stderr
// contains the documented warning, the test reports the parallel-
// dev gap instead of failing.
func TestE2E_SpaceAdd_WritesProjectsYaml(t *testing.T) {
	r := newRunner(t)
	tmp1 := t.TempDir()
	tmp2 := t.TempDir()

	// Workspace must exist before space add.
	addWS := r.run("workspace", "add", "ic-ops", "--json")
	requireExitCode(t, addWS, 0)

	// First space write-through.
	add1 := r.run("space", "add", "ic-ops", "ops",
		"--path", tmp1, "--startup-cmd", "zsh")
	requireExitCode(t, add1, 0)
	if kitGapDetected(add1.Stderr) {
		t.Skipf("kit projects package not yet importable; warning observed: %s", add1.Stderr)
	}

	// Second space — manual entries (none here) would be preserved
	// by the kit contract; verifies write does not clobber.
	add2 := r.run("space", "add", "ic-ops", "second",
		"--path", tmp2)
	requireExitCode(t, add2, 0)

	got := readProjectsYaml(t, r.xdgConfigHome)
	mustHaveEntry(t, got, "ops", projectsRecord{
		Path: tmp1, StartupCmd: "zsh", Source: "wsm",
	})
	mustHaveEntry(t, got, "second", projectsRecord{
		Path: tmp2, Source: "wsm",
	})

	// Sync should be a no-op (entries already match) but must not
	// drop existing rows or error.
	sync := r.run("space", "sync")
	requireExitCode(t, sync, 0)

	got = readProjectsYaml(t, r.xdgConfigHome)
	mustHaveEntry(t, got, "ops", projectsRecord{
		Path: tmp1, StartupCmd: "zsh", Source: "wsm",
	})
	mustHaveEntry(t, got, "second", projectsRecord{
		Path: tmp2, Source: "wsm",
	})
}

// kitGapDetected matches the documented warning printed by `wsm
// space add` when the kit projects writer is unavailable. Used to
// skip the test until kit's projects implementation lands.
func kitGapDetected(stderr string) bool {
	return strings.Contains(stderr, "failed to update projects.yaml") &&
		strings.Contains(stderr, "wsm space sync")
}

func readProjectsYaml(t *testing.T, xdgConfigHome string) projectsFile {
	t.Helper()
	path := filepath.Join(xdgConfigHome, "rux", "projects.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read projects.yaml at %s: %v", path, err)
	}
	var f projectsFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse projects.yaml: %v\n--- raw ---\n%s", err, raw)
	}
	return f
}

func mustHaveEntry(t *testing.T, f projectsFile, name string, want projectsRecord) {
	t.Helper()
	got, ok := f.Projects[name]
	if !ok {
		t.Fatalf("projects.yaml missing entry %q (have: %v)", name, keys(f.Projects))
	}
	if got.Path != want.Path {
		t.Errorf("entry %q Path: got %q want %q", name, got.Path, want.Path)
	}
	if got.StartupCmd != want.StartupCmd {
		t.Errorf("entry %q StartupCmd: got %q want %q", name, got.StartupCmd, want.StartupCmd)
	}
	if got.Source != want.Source {
		t.Errorf("entry %q Source: got %q want %q", name, got.Source, want.Source)
	}
}

func keys(m map[string]projectsRecord) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
