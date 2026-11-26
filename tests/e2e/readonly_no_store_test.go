package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolatedStoreEnv points every wsm directory at a not-yet-existing path
// under one temp root and returns the env plus the paths that must stay
// absent after a read-only query.
func isolatedStoreEnv(t *testing.T) (map[string]string, []string) {
	t.Helper()
	base := filepath.Join(t.TempDir(), "fresh")
	paths := map[string]string{
		"WSM_DATA_DIR":    filepath.Join(base, "data"),
		"XDG_CONFIG_HOME": filepath.Join(base, "xdg", "config"),
		"XDG_DATA_HOME":   filepath.Join(base, "xdg", "data"),
		"XDG_STATE_HOME":  filepath.Join(base, "xdg", "state"),
		"XDG_CACHE_HOME":  filepath.Join(base, "xdg", "cache"),
	}
	var absent []string
	for _, p := range paths {
		absent = append(absent, p)
	}
	absent = append(absent, filepath.Join(paths["WSM_DATA_DIR"], "wsm.db"), base)
	return paths, absent
}

func requireAbsent(t *testing.T, paths []string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("read-only query created %s (stat err: %v)", p, err)
		}
	}
}

func TestReadOnlyQuery_NoStore_CreatesNothing(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
	}{
		{"workspace list json", []string{"workspace", "list", "--format", "json"}, "[]", 0},
		{"device list json", []string{"device", "list", "--format", "json"}, "", 0},
		{"snapshot list json", []string{"snapshot", "list", "--format", "json"}, "[]", 0},
		{"workspace show unknown", []string{"workspace", "show", "nope"}, "", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRunner(t)
			env, absent := isolatedStoreEnv(t)
			res := r.runNoGlobalWithEnv(env, tc.args...)
			requireExitCode(t, res, tc.wantCode)
			if tc.wantOut != "" && res.StdoutTrim() != tc.wantOut {
				t.Fatalf("stdout = %q, want %q (stderr: %s)", res.Stdout, tc.wantOut, res.Stderr)
			}
			requireAbsent(t, absent)
		})
	}
}

func TestWriteCommand_StillCreatesStore(t *testing.T) {
	r := newRunner(t)
	env, _ := isolatedStoreEnv(t)

	res := r.runNoGlobalWithEnv(env, "workspace", "add", "alpha", "--format", "json")
	requireExitCode(t, res, 0)

	db := filepath.Join(env["WSM_DATA_DIR"], "wsm.db")
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("write command did not create store: %v", err)
	}

	res = r.runNoGlobalWithEnv(env, "workspace", "list", "--format", "json")
	requireExitCode(t, res, 0)
	if !strings.Contains(res.Stdout, `"alpha"`) {
		t.Fatalf("list after add missing workspace:\n%s", res.Stdout)
	}
}

func TestReadOnlyQuery_LegacyStoreStillMigrated(t *testing.T) {
	r := newRunner(t)
	env, _ := isolatedStoreEnv(t)

	legacyDir := filepath.Join(r.homeDir, ".wsm")
	mustMkdirAll(t, legacyDir)
	legacyDB := filepath.Join(legacyDir, "wsm.db")
	if err := os.WriteFile(legacyDB, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	res := r.runNoGlobalWithEnv(env, "workspace", "list", "--format", "json")
	requireExitCode(t, res, 0)

	if _, err := os.Stat(filepath.Join(env["WSM_DATA_DIR"], "wsm.db")); err != nil {
		t.Fatalf("legacy store not migrated: %v", err)
	}
	if _, err := os.Stat(legacyDB); !os.IsNotExist(err) {
		t.Fatalf("legacy store still present: %v", err)
	}
}
