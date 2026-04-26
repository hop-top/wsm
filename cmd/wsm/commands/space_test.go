package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	backendsqlite "hop.top/wsm/pkg/backend/sqlite"
	"hop.top/wsm/pkg/workspace"
)

// testEnv builds an isolated wsm data dir + globalOptions targeting
// it, and seeds an empty workspace named "ws". Each subtest gets its
// own t.TempDir() so the SQLite DB is fresh.
type testEnv struct {
	opts    *globalOptions
	dataDir string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dataDir := t.TempDir()
	opts := &globalOptions{
		Backend: "sqlite",
		DataDir: dataDir,
		Quiet:   true,
	}
	// Seed: open backend directly and create one workspace so
	// `space add` has a target. This avoids invoking the workspace
	// add command from this test.
	b, err := backendsqlite.Open(filepath.Join(dataDir, "wsm.db"))
	if err != nil {
		t.Fatalf("open backend: %v", err)
	}
	if err := b.Migrate(context.Background()); err != nil {
		_ = b.Close()
		t.Fatalf("migrate: %v", err)
	}
	mgr, err := workspace.NewManager(b)
	if err != nil {
		_ = b.Close()
		t.Fatalf("new manager: %v", err)
	}
	if _, err := mgr.Create(context.Background(), "ws", workspace.CreateOptions{}); err != nil {
		_ = b.Close()
		t.Fatalf("create workspace: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("close backend: %v", err)
	}
	return &testEnv{opts: opts, dataDir: dataDir}
}

// withFakeWriter swaps the package-level writeProjectsEntry for a
// captured fake during the lifetime of t. Returns a pointer to the
// slice of captured calls.
func withFakeWriter(t *testing.T, errOnCall error) *[]projectsCall {
	t.Helper()
	calls := &[]projectsCall{}
	mu := sync.Mutex{}
	prev := writeProjectsEntry
	writeProjectsEntry = func(name string, e projectsEntry) error {
		mu.Lock()
		*calls = append(*calls, projectsCall{Name: name, Entry: e})
		mu.Unlock()
		return errOnCall
	}
	t.Cleanup(func() { writeProjectsEntry = prev })
	return calls
}

type projectsCall struct {
	Name  string
	Entry projectsEntry
}

// runSpaceAdd executes the `space add` cobra subcommand with the
// given args and captures stdout/stderr.
func runSpaceAdd(t *testing.T, env *testEnv, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newSpaceAddCmd(env.opts)
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func runSpaceSync(t *testing.T, env *testEnv) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newSpaceSyncCmd(env.opts)
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(nil)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// openManagerFor reopens the SQLite backend so tests can verify
// post-conditions (e.g., that the space row was persisted).
func (e *testEnv) openManager(t *testing.T) (*workspace.Manager, func()) {
	t.Helper()
	b, err := backendsqlite.Open(filepath.Join(e.dataDir, "wsm.db"))
	if err != nil {
		t.Fatalf("reopen backend: %v", err)
	}
	if err := b.Migrate(context.Background()); err != nil {
		_ = b.Close()
		t.Fatalf("migrate: %v", err)
	}
	mgr, err := workspace.NewManager(b)
	if err != nil {
		_ = b.Close()
		t.Fatalf("manager: %v", err)
	}
	return mgr, func() { _ = b.Close() }
}

func TestSpaceAdd_PathFlagAccepted(t *testing.T) {
	env := newTestEnv(t)
	withFakeWriter(t, nil)

	_, _, err := runSpaceAdd(t, env, "ws", "sp-1", "--path", "/tmp/foo")
	if err != nil {
		t.Fatalf("space add --path failed: %v", err)
	}

	mgr, cleanup := env.openManager(t)
	defer cleanup()
	got, err := mgr.GetSpace(context.Background(), "sp-1")
	if err != nil {
		t.Fatalf("get space: %v", err)
	}
	if got.Boundary != "/tmp/foo" {
		t.Fatalf("Boundary mismatch: got %q want %q", got.Boundary, "/tmp/foo")
	}
}

func TestSpaceAdd_BoundaryFlagRejected(t *testing.T) {
	env := newTestEnv(t)
	withFakeWriter(t, nil)

	_, _, err := runSpaceAdd(t, env, "ws", "sp-2", "--boundary", "/tmp/foo")
	if err == nil {
		t.Fatal("expected --boundary to be rejected after rename, got nil error")
	}
	if !strings.Contains(err.Error(), "unknown flag") && !strings.Contains(err.Error(), "boundary") {
		t.Fatalf("expected unknown-flag error mentioning boundary, got: %v", err)
	}
}

func TestSpaceAdd_WritesToProjectsYaml(t *testing.T) {
	env := newTestEnv(t)
	calls := withFakeWriter(t, nil)

	_, _, err := runSpaceAdd(t, env,
		"ws", "ops",
		"--path", "/Users/jadb/.ops",
		"--startup-cmd", "zsh",
	)
	if err != nil {
		t.Fatalf("space add failed: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("expected exactly 1 projects writer call, got %d", len(*calls))
	}
	c := (*calls)[0]
	if c.Name != "ops" {
		t.Errorf("Name: got %q want %q", c.Name, "ops")
	}
	if c.Entry.Path != "/Users/jadb/.ops" {
		t.Errorf("Path: got %q want %q", c.Entry.Path, "/Users/jadb/.ops")
	}
	if c.Entry.StartupCmd != "zsh" {
		t.Errorf("StartupCmd: got %q want %q", c.Entry.StartupCmd, "zsh")
	}
	if c.Entry.Source != projectsSourceWSM {
		t.Errorf("Source: got %q want %q", c.Entry.Source, projectsSourceWSM)
	}
}

func TestSpaceAdd_DefaultsApplied(t *testing.T) {
	env := newTestEnv(t)
	calls := withFakeWriter(t, nil)

	_, _, err := runSpaceAdd(t, env, "ws", "sp-3", "--path", "/tmp/bar")
	if err != nil {
		t.Fatalf("space add failed: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(*calls))
	}
	if (*calls)[0].Entry.StartupCmd != "" {
		t.Errorf("expected empty default StartupCmd, got %q", (*calls)[0].Entry.StartupCmd)
	}
}

func TestSpaceAdd_ConfigWriteFails_ExitsZero(t *testing.T) {
	env := newTestEnv(t)
	withFakeWriter(t, errors.New("disk full"))

	// stderr from the warning is written to os.Stderr, not the cobra
	// err writer, because the warning is informational and must not
	// be swallowed by `--json` callers. We capture the cobra err
	// stream to assert RunE itself returned nil.
	_, _, err := runSpaceAdd(t, env, "ws", "sp-4", "--path", "/tmp/baz")
	if err != nil {
		t.Fatalf("space add must exit 0 on projects write failure, got err: %v", err)
	}

	// DB write must have succeeded regardless.
	mgr, cleanup := env.openManager(t)
	defer cleanup()
	if _, err := mgr.GetSpace(context.Background(), "sp-4"); err != nil {
		t.Fatalf("space row missing after add: %v", err)
	}
}

func TestSpaceSync_ReplaysAllSpaces(t *testing.T) {
	env := newTestEnv(t)

	// Seed 3 spaces directly via the manager (bypass the add command
	// so we don't double-count writer calls during seeding).
	mgr, cleanup := env.openManager(t)
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("sp-sync-%d", i)
		_, err := mgr.AddSpaceToWorkspace(ctx, "ws", workspace.Space{
			ID:         id,
			Type:       workspace.SpaceFilesystem,
			Boundary:   fmt.Sprintf("/tmp/sync/%d", i),
			TrustLevel: workspace.TrustLocal,
		})
		if err != nil {
			cleanup()
			t.Fatalf("seed space %s: %v", id, err)
		}
	}
	cleanup()

	// Now swap in the fake writer and run sync.
	calls := withFakeWriter(t, nil)
	_, _, err := runSpaceSync(t, env)
	if err != nil {
		t.Fatalf("space sync failed: %v", err)
	}

	if len(*calls) != 3 {
		t.Fatalf("expected 3 writer calls (one per space), got %d", len(*calls))
	}
	seen := map[string]string{}
	for _, c := range *calls {
		seen[c.Name] = c.Entry.Path
		if c.Entry.Source != projectsSourceWSM {
			t.Errorf("sync wrote non-wsm Source for %q: %q", c.Name, c.Entry.Source)
		}
	}
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("sp-sync-%d", i)
		want := fmt.Sprintf("/tmp/sync/%d", i)
		if got, ok := seen[id]; !ok {
			t.Errorf("sync missing entry for %s", id)
		} else if got != want {
			t.Errorf("sync %s path: got %q want %q", id, got, want)
		}
	}
}
