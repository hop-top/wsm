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
	t.Setenv("HOME", root)
	t.Setenv("WSM_BACKEND", "")
	t.Setenv("WSM_DATA_DIR", "")
	t.Setenv("WSM_BACKEND_URL", "")
	return root
}

func TestConfig_Defaults(t *testing.T) {
	withCleanConfigEnv(t)
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
