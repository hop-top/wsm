package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withCleanConfigEnv(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "cfg"))
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", root)
	t.Setenv("WSM_BACKEND", "")
	t.Setenv("WSM_DATA_DIR", "")
	t.Setenv("WSM_CACHE_DIR", "")
	t.Setenv("WSM_STATE_DIR", "")
	t.Setenv("WSM_BACKEND_URL", "")
	return root
}

func TestConfig_Defaults(t *testing.T) {
	root := withCleanConfigEnv(t)
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Backend != "sqlite" {
		t.Fatalf("expected sqlite backend, got %q", cfg.Backend)
	}
	if cfg.DataDir == "" {
		t.Fatal("expected default data dir")
	}
	// kit/xdg uses OS-native fallbacks (HIG on macOS, XDG on Linux);
	// assert contract: path lives under HOME and ends in /wsm.
	if !strings.HasPrefix(cfg.CacheDir, root) || !strings.HasSuffix(cfg.CacheDir, "wsm") {
		t.Fatalf("cache dir %q must be under %q and end with /wsm", cfg.CacheDir, root)
	}
	if !strings.HasPrefix(cfg.StateDir, root) || !strings.HasSuffix(cfg.StateDir, "wsm") {
		t.Fatalf("state dir %q must be under %q and end with /wsm", cfg.StateDir, root)
	}
}

func TestConfig_FileLoad(t *testing.T) {
	root := withCleanConfigEnv(t)
	path := filepath.Join(root, "cfg", "wsm")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := []byte("backend: sqlite\ndata_dir: /tmp/wsm-test\n")
	if err := os.WriteFile(filepath.Join(path, "config.yaml"), content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DataDir != "/tmp/wsm-test" {
		t.Fatalf("expected file data_dir override, got %q", cfg.DataDir)
	}
}

func TestConfig_EnvOverride(t *testing.T) {
	withCleanConfigEnv(t)
	t.Setenv("WSM_DATA_DIR", "/tmp/from-env")
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DataDir != "/tmp/from-env" {
		t.Fatalf("expected env override, got %q", cfg.DataDir)
	}
}

func TestConfig_FlagOverride(t *testing.T) {
	withCleanConfigEnv(t)
	t.Setenv("WSM_DATA_DIR", "/tmp/from-env")
	cfg, err := Load(LoadOptions{FlagDataDir: "/tmp/from-flag"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DataDir != "/tmp/from-flag" {
		t.Fatalf("expected flag override, got %q", cfg.DataDir)
	}
}

func TestConfig_CacheDir_XDG(t *testing.T) {
	withCleanConfigEnv(t)
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg-cache")
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := filepath.Join("/tmp/xdg-cache", "wsm")
	if cfg.CacheDir != want {
		t.Fatalf("expected XDG cache dir %q, got %q", want, cfg.CacheDir)
	}
}

func TestConfig_CacheDir_EnvOverride(t *testing.T) {
	withCleanConfigEnv(t)
	t.Setenv("WSM_CACHE_DIR", "/tmp/cache-env")
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.CacheDir != "/tmp/cache-env" {
		t.Fatalf("expected env cache dir, got %q", cfg.CacheDir)
	}
}

func TestConfig_CacheDir_FlagOverride(t *testing.T) {
	withCleanConfigEnv(t)
	t.Setenv("WSM_CACHE_DIR", "/tmp/cache-env")
	cfg, err := Load(LoadOptions{FlagCacheDir: "/tmp/cache-flag"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.CacheDir != "/tmp/cache-flag" {
		t.Fatalf("expected flag cache dir, got %q", cfg.CacheDir)
	}
}

func TestConfig_StateDir_XDG(t *testing.T) {
	withCleanConfigEnv(t)
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := filepath.Join("/tmp/xdg-state", "wsm")
	if cfg.StateDir != want {
		t.Fatalf("expected XDG state dir %q, got %q", want, cfg.StateDir)
	}
}

func TestConfig_StateDir_EnvOverride(t *testing.T) {
	withCleanConfigEnv(t)
	t.Setenv("WSM_STATE_DIR", "/tmp/state-env")
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.StateDir != "/tmp/state-env" {
		t.Fatalf("expected env state dir, got %q", cfg.StateDir)
	}
}

func TestConfig_StateDir_FlagOverride(t *testing.T) {
	withCleanConfigEnv(t)
	t.Setenv("WSM_STATE_DIR", "/tmp/state-env")
	cfg, err := Load(LoadOptions{FlagStateDir: "/tmp/state-flag"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.StateDir != "/tmp/state-flag" {
		t.Fatalf("expected flag state dir, got %q", cfg.StateDir)
	}
}

func TestConfig_DataDir_XDG(t *testing.T) {
	withCleanConfigEnv(t)
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := filepath.Join("/tmp/xdg-data", "wsm")
	if cfg.DataDir != want {
		t.Fatalf("expected XDG data dir %q, got %q", want, cfg.DataDir)
	}
}

func TestConfig_ProjectConfig_Candidates(t *testing.T) {
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
			withCleanConfigEnv(t)
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatalf("getwd: %v", err)
			}
			t.Cleanup(func() { _ = os.Chdir(cwd) })
			project := t.TempDir()
			if err := os.Chdir(project); err != nil {
				t.Fatalf("chdir: %v", err)
			}
			full := filepath.Join(project, tc.rel)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			content := []byte("data_dir: /tmp/wsm-project-" + tc.name + "\n")
			if err := os.WriteFile(full, content, 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			cfg, err := Load(LoadOptions{})
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			want := "/tmp/wsm-project-" + tc.name
			if cfg.DataDir != want {
				t.Fatalf("expected project data_dir %q, got %q", want, cfg.DataDir)
			}
		})
	}
}

func TestConfig_ConfigFile_ReportsProjectSource(t *testing.T) {
	// Each project candidate, when present, should be reported as the
	// resolved ConfigFile path (not the XDG user default). This is the
	// T-0117 regression: layered loaders correctly used the project
	// values but the metadata kept pointing at the XDG location.
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
			withCleanConfigEnv(t)
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatalf("getwd: %v", err)
			}
			t.Cleanup(func() { _ = os.Chdir(cwd) })
			project := t.TempDir()
			if err := os.Chdir(project); err != nil {
				t.Fatalf("chdir: %v", err)
			}
			full := filepath.Join(project, tc.rel)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(full, []byte("backend: sqlite\n"), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			cfg, err := Load(LoadOptions{})
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			// macOS resolves /var/folders → /private/var/folders; compare via EvalSymlinks.
			gotResolved, _ := filepath.EvalSymlinks(cfg.ConfigFile)
			wantResolved, _ := filepath.EvalSymlinks(full)
			if gotResolved != wantResolved {
				t.Fatalf("ConfigFile = %q (resolved %q), want %q (resolved %q)",
					cfg.ConfigFile, gotResolved, full, wantResolved)
			}
			if !cfg.ConfigFileFound {
				t.Fatalf("ConfigFileFound = false, want true")
			}
			if cfg.Source != "config_file" {
				t.Fatalf("Source = %q, want config_file", cfg.Source)
			}
		})
	}
}

func TestConfig_ConfigFile_NotFoundReportsUserPath(t *testing.T) {
	// When no layer file exists, the reported path should still point at
	// the user-scope (XDG) path so `(not found)` hint is actionable.
	withCleanConfigEnv(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ConfigFileFound {
		t.Fatalf("ConfigFileFound = true, want false")
	}
	if cfg.ConfigFile != userConfigPath() {
		t.Fatalf("ConfigFile = %q, want user XDG path %q", cfg.ConfigFile, userConfigPath())
	}
}

func TestConfig_ConfigFile_ProjectBeatsUser(t *testing.T) {
	// When both a user XDG file and a project file exist, the project
	// path wins (highest precedence existing file).
	root := withCleanConfigEnv(t)
	userDir := filepath.Join(root, "cfg", "wsm")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "config.yaml"),
		[]byte("backend: sqlite\n"), 0o644); err != nil {
		t.Fatalf("write user: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	project := t.TempDir()
	if err := os.Chdir(project); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	projectFile := filepath.Join(project, ".wsm", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(projectFile), 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.WriteFile(projectFile, []byte("data_dir: /tmp/from-project\n"), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	gotResolved, _ := filepath.EvalSymlinks(cfg.ConfigFile)
	wantResolved, _ := filepath.EvalSymlinks(projectFile)
	if gotResolved != wantResolved {
		t.Fatalf("ConfigFile = %q (resolved %q), want %q (resolved %q)",
			cfg.ConfigFile, gotResolved, projectFile, wantResolved)
	}
}

func TestConfig_CacheDir_FileOverride(t *testing.T) {
	root := withCleanConfigEnv(t)
	path := filepath.Join(root, "cfg", "wsm")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := []byte("cache_dir: /tmp/cache-file\nstate_dir: /tmp/state-file\n")
	if err := os.WriteFile(filepath.Join(path, "config.yaml"), content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.CacheDir != "/tmp/cache-file" {
		t.Fatalf("expected file cache_dir override, got %q", cfg.CacheDir)
	}
	if cfg.StateDir != "/tmp/state-file" {
		t.Fatalf("expected file state_dir override, got %q", cfg.StateDir)
	}
}
