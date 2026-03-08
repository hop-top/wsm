package config

import (
	"os"
	"path/filepath"
	"testing"
)

func withCleanConfigEnv(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "cfg"))
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
	wantCache := filepath.Join(root, ".cache", "wsm")
	if cfg.CacheDir != wantCache {
		t.Fatalf("expected default cache dir %q, got %q", wantCache, cfg.CacheDir)
	}
	wantState := filepath.Join(root, ".local", "state", "wsm")
	if cfg.StateDir != wantState {
		t.Fatalf("expected default state dir %q, got %q", wantState, cfg.StateDir)
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
