package config

import (
	"os"
	"path/filepath"
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

func TestFindAnyLocalConfig_PrefersWsm(t *testing.T) {
	root := t.TempDir()
	// Create both .wsm/ and .hop/wsm/
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

	lc, err := FindAnyLocalConfig(root)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !lc.Found {
		t.Fatal("expected Found=true")
	}
	if lc.Settings["source"] != "wsm" {
		t.Fatalf("expected .wsm/ preferred, got source=%q", lc.Settings["source"])
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
