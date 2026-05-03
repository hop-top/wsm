package e2e_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigShow_ProjectSourcePath covers T-0117: when a project-level
// config exists, `wsm config show` must report that file as the source —
// not the XDG default.
func TestConfigShow_ProjectSourcePath(t *testing.T) {
	cases := []struct {
		name string
		rel  string
	}{
		{"legacy_dotfile", ".wsm.yaml"},
		{"wsm_dir", filepath.Join(".wsm", "config.yaml")},
		{"hop_wsm_dir", filepath.Join(".hop", "wsm", "config.yaml")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRunner(t)
			full := filepath.Join(r.workDir, tc.rel)
			mustMkdirAll(t, filepath.Dir(full))
			writeFile(t, full, []byte("backend: sqlite\ndata_dir: /tmp/from-project-"+tc.name+"\n"))

			res := r.runNoGlobal("config", "show")
			requireExitCode(t, res, 0)

			// Path may be reported via realpath (macOS resolves
			// /var/folders → /private/var/folders), so assert the
			// candidate suffix is present rather than the exact prefix.
			wantSuffix := "/" + tc.rel
			if !strings.Contains(res.Stdout, "config_file:") ||
				!strings.Contains(res.Stdout, wantSuffix) {
				t.Fatalf("expected config_file pointing at %q, got:\n%s", tc.rel, res.Stdout)
			}
			if strings.Contains(res.Stdout, "(not found)") {
				t.Fatalf("did not expect (not found) when project file exists, got:\n%s", res.Stdout)
			}
			if !strings.Contains(res.Stdout, "source:      config_file") {
				t.Fatalf("expected config_file source, got:\n%s", res.Stdout)
			}
		})
	}
}

func TestConfigShow(t *testing.T) {
	r := newRunner(t)

	defaults := r.runNoGlobal("config", "show")
	requireExitCode(t, defaults, 0)
	if !strings.Contains(defaults.Stdout, "backend:     sqlite") {
		t.Fatalf("expected default sqlite backend in config output:\n%s", defaults.Stdout)
	}
	if !strings.Contains(defaults.Stdout, "(not found)") {
		t.Fatalf("expected missing config file note:\n%s", defaults.Stdout)
	}
	if !strings.Contains(defaults.Stdout, "source:      defaults") {
		t.Fatalf("expected defaults source in config output:\n%s", defaults.Stdout)
	}

	cfgDir := filepath.Join(r.xdgConfigHome, "wsm")
	mustMkdirAll(t, cfgDir)
	configPath := filepath.Join(cfgDir, "config.yaml")
	writeFile(t, configPath, []byte("backend: sqlite\ndata_dir: /tmp/from-config\n"))

	fromFile := r.runNoGlobal("config", "show")
	requireExitCode(t, fromFile, 0)
	if !strings.Contains(fromFile.Stdout, "data_dir:    /tmp/from-config") {
		t.Fatalf("expected config file data_dir, got:\n%s", fromFile.Stdout)
	}
	if !strings.Contains(fromFile.Stdout, "source:      config_file") {
		t.Fatalf("expected config_file source, got:\n%s", fromFile.Stdout)
	}

	fromEnv := r.runNoGlobalWithEnv(map[string]string{"WSM_DATA_DIR": "/tmp/from-env"}, "config", "show")
	requireExitCode(t, fromEnv, 0)
	if !strings.Contains(fromEnv.Stdout, "data_dir:    /tmp/from-env") {
		t.Fatalf("expected env data_dir override, got:\n%s", fromEnv.Stdout)
	}
	if !strings.Contains(fromEnv.Stdout, "source:      env") {
		t.Fatalf("expected env source, got:\n%s", fromEnv.Stdout)
	}

	fromFlag := r.runNoGlobalWithEnv(
		map[string]string{"WSM_DATA_DIR": "/tmp/from-env"},
		"--data-dir", "/tmp/from-flag", "config", "show",
	)
	requireExitCode(t, fromFlag, 0)
	if !strings.Contains(fromFlag.Stdout, "data_dir:    /tmp/from-flag") {
		t.Fatalf("expected flag data_dir override, got:\n%s", fromFlag.Stdout)
	}
	if !strings.Contains(fromFlag.Stdout, "source:      flags") {
		t.Fatalf("expected flags source, got:\n%s", fromFlag.Stdout)
	}
}
