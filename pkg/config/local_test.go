package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindLocalConfig_Found(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, "project", ".wsm")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := []byte("backend: memory\ndata_dir: /tmp/local\n")
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	lc, err := FindLocalConfig(filepath.Join(root, "project"))
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !lc.Found {
		t.Fatal("expected Found=true")
	}
	if lc.Settings["backend"] != "memory" {
		t.Fatalf("expected backend=memory, got %q", lc.Settings["backend"])
	}
	if lc.Settings["data_dir"] != "/tmp/local" {
		t.Fatalf("expected data_dir=/tmp/local, got %q", lc.Settings["data_dir"])
	}
}

func TestFindLocalConfig_WalksUp(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, ".wsm")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := []byte("key: value\n")
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	lc, err := FindLocalConfig(nested)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !lc.Found {
		t.Fatal("expected Found=true from ancestor")
	}
	if lc.Settings["key"] != "value" {
		t.Fatalf("expected key=value, got %q", lc.Settings["key"])
	}
}

func TestFindLocalConfig_NotFound(t *testing.T) {
	root := t.TempDir()
	lc, err := FindLocalConfig(root)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if lc.Found {
		t.Fatal("expected Found=false")
	}
}

func TestFindHopLocalConfig_Found(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, ".hop", "wsm")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := []byte("mode: hop\n")
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	lc, err := FindHopLocalConfig(root)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !lc.Found {
		t.Fatal("expected Found=true")
	}
	if lc.Settings["mode"] != "hop" {
		t.Fatalf("expected mode=hop, got %q", lc.Settings["mode"])
	}
}

func TestFindHopLocalConfig_NotFound(t *testing.T) {
	root := t.TempDir()
	lc, err := FindHopLocalConfig(root)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if lc.Found {
		t.Fatal("expected Found=false")
	}
}

func TestFindAnyLocalConfig_BothIndependent_Error(t *testing.T) {
	root := t.TempDir()
	// Create both .wsm/ and .hop/wsm/ as independent directories.
	wsmDir := filepath.Join(root, ".wsm")
	hopDir := filepath.Join(root, ".hop", "wsm")
	if err := os.MkdirAll(wsmDir, 0o755); err != nil {
		t.Fatalf("mkdir wsm: %v", err)
	}
	if err := os.MkdirAll(hopDir, 0o755); err != nil {
		t.Fatalf("mkdir hop: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsmDir, "config.yaml"), []byte("source: wsm\n"), 0o644); err != nil {
		t.Fatalf("write wsm: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hopDir, "config.yaml"), []byte("source: hop\n"), 0o644); err != nil {
		t.Fatalf("write hop: %v", err)
	}

	_, err := FindAnyLocalConfig(root)
	if err == nil {
		t.Fatal("expected error when both .wsm/ and .hop/wsm/ exist independently")
	}
	if !strings.Contains(err.Error(), "conflicting local config") {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestFindAnyLocalConfig_BothIndependent_HopBranch_Error(t *testing.T) {
	root := t.TempDir()
	// Both dirs exist independently, but only .hop/wsm/ has a config file.
	// This exercises the hop-branch conflict detection path.
	wsmDir := filepath.Join(root, ".wsm")
	hopDir := filepath.Join(root, ".hop", "wsm")
	if err := os.MkdirAll(wsmDir, 0o755); err != nil {
		t.Fatalf("mkdir wsm: %v", err)
	}
	if err := os.MkdirAll(hopDir, 0o755); err != nil {
		t.Fatalf("mkdir hop: %v", err)
	}
	// Only write config in .hop/wsm/, not in .wsm/
	if err := os.WriteFile(filepath.Join(hopDir, "config.yaml"), []byte("source: hop\n"), 0o644); err != nil {
		t.Fatalf("write hop: %v", err)
	}

	_, err := FindAnyLocalConfig(root)
	if err == nil {
		t.Fatal("expected error when both dirs exist independently (hop branch)")
	}
	if !strings.Contains(err.Error(), "conflicting local config") {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestFindAnyLocalConfig_BothSymlinked_OK(t *testing.T) {
	root := t.TempDir()
	// Create .wsm/ as the real directory, symlink .hop/wsm/ -> .wsm/
	wsmDir := filepath.Join(root, ".wsm")
	hopParent := filepath.Join(root, ".hop")
	hopDir := filepath.Join(hopParent, "wsm")
	if err := os.MkdirAll(wsmDir, 0o755); err != nil {
		t.Fatalf("mkdir wsm: %v", err)
	}
	if err := os.MkdirAll(hopParent, 0o755); err != nil {
		t.Fatalf("mkdir hop parent: %v", err)
	}
	if err := os.Symlink(wsmDir, hopDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsmDir, "config.yaml"), []byte("source: wsm\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	lc, err := FindAnyLocalConfig(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !lc.Found {
		t.Fatal("expected Found=true")
	}
	if lc.Settings["source"] != "wsm" {
		t.Fatalf("expected source=wsm, got %q", lc.Settings["source"])
	}
}

func TestFindAnyLocalConfig_ReverseSymlink_OK(t *testing.T) {
	root := t.TempDir()
	// Create .hop/wsm/ as real, symlink .wsm/ -> .hop/wsm/
	hopDir := filepath.Join(root, ".hop", "wsm")
	if err := os.MkdirAll(hopDir, 0o755); err != nil {
		t.Fatalf("mkdir hop: %v", err)
	}
	if err := os.Symlink(hopDir, filepath.Join(root, ".wsm")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hopDir, "config.yaml"), []byte("source: hop\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	lc, err := FindAnyLocalConfig(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !lc.Found {
		t.Fatal("expected Found=true")
	}
}

func TestFindAnyLocalConfig_FallsBackToHop(t *testing.T) {
	root := t.TempDir()
	hopDir := filepath.Join(root, ".hop", "wsm")
	if err := os.MkdirAll(hopDir, 0o755); err != nil {
		t.Fatalf("mkdir hop: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hopDir, "config.yaml"), []byte("source: hop\n"), 0o644); err != nil {
		t.Fatalf("write hop: %v", err)
	}

	lc, err := FindAnyLocalConfig(root)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !lc.Found {
		t.Fatal("expected Found=true")
	}
	if lc.Settings["source"] != "hop" {
		t.Fatalf("expected .hop/wsm/ fallback, got source=%q", lc.Settings["source"])
	}
}

func TestFindAnyLocalConfig_NotFound(t *testing.T) {
	root := t.TempDir()
	lc, err := FindAnyLocalConfig(root)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if lc.Found {
		t.Fatal("expected Found=false")
	}
}

func TestLoadLocalConfig_InvalidYAML(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bad.yaml")
	if err := os.WriteFile(path, []byte(":::invalid"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadLocalConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadLocalConfig_EmptyFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "empty.yaml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	lc, err := LoadLocalConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if lc.Settings == nil {
		t.Fatal("expected non-nil settings map")
	}
	if len(lc.Settings) != 0 {
		t.Fatalf("expected empty settings, got %v", lc.Settings)
	}
}

func TestInitLocalConfig_Standalone(t *testing.T) {
	root := t.TempDir()
	configPath, err := InitLocalConfig(root, false, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if configPath != filepath.Join(root, ".wsm", "config.yaml") {
		t.Fatalf("expected .wsm/config.yaml, got %s", configPath)
	}
	// .wsm/ should be a real directory.
	info, err := os.Lstat(filepath.Join(root, ".wsm"))
	if err != nil {
		t.Fatalf("lstat .wsm: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal(".wsm should not be a symlink")
	}
	// .hop/wsm/ should be a symlink to .wsm/.
	target, err := os.Readlink(filepath.Join(root, ".hop", "wsm"))
	if err != nil {
		t.Fatalf("readlink .hop/wsm: %v", err)
	}
	if target != filepath.Join(root, ".wsm") {
		t.Fatalf("expected symlink to .wsm, got %s", target)
	}
	// FindAnyLocalConfig should work.
	lc, err := FindAnyLocalConfig(root)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !lc.Found {
		t.Fatal("expected Found=true")
	}
}

func TestInitLocalConfig_Hop(t *testing.T) {
	root := t.TempDir()
	configPath, err := InitLocalConfig(root, true, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if configPath != filepath.Join(root, ".hop", "wsm", "config.yaml") {
		t.Fatalf("expected .hop/wsm/config.yaml, got %s", configPath)
	}
	// .hop/wsm/ should be a real directory.
	info, err := os.Lstat(filepath.Join(root, ".hop", "wsm"))
	if err != nil {
		t.Fatalf("lstat .hop/wsm: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal(".hop/wsm should not be a symlink")
	}
	// .wsm/ should be a symlink to .hop/wsm/.
	target, err := os.Readlink(filepath.Join(root, ".wsm"))
	if err != nil {
		t.Fatalf("readlink .wsm: %v", err)
	}
	if target != filepath.Join(root, ".hop", "wsm") {
		t.Fatalf("expected symlink to .hop/wsm, got %s", target)
	}
}

func TestInitLocalConfig_ExistingRefused(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".wsm"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := InitLocalConfig(root, false, false)
	if err == nil {
		t.Fatal("expected error when .wsm exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected 'already exists' error, got: %v", err)
	}
}

func TestInitLocalConfig_ForceOverwrites(t *testing.T) {
	root := t.TempDir()
	// Create existing .wsm/ with content.
	wsmDir := filepath.Join(root, ".wsm")
	if err := os.MkdirAll(wsmDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsmDir, "config.yaml"), []byte("old: true\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := InitLocalConfig(root, false, true)
	if err != nil {
		t.Fatalf("init with force: %v", err)
	}
	// Symlink should exist now.
	if _, err := os.Readlink(filepath.Join(root, ".hop", "wsm")); err != nil {
		t.Fatalf("expected .hop/wsm symlink after force init: %v", err)
	}
}
