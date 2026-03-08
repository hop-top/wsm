package detector

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMiseDetector_Name(t *testing.T) {
	d := &MiseDetector{}
	if d.Name() != "mise" {
		t.Fatalf("expected name 'mise', got %q", d.Name())
	}
}

func TestMiseDetector_NotDetected(t *testing.T) {
	dir := t.TempDir()
	d := &MiseDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Detected {
		t.Fatal("expected not detected in empty dir")
	}
}

func TestMiseDetector_RootConfig(t *testing.T) {
	dir := t.TempDir()
	content := `
[tools]
node = "20.11.0"
go = "1.23"
python = "3.12"
`
	if err := os.WriteFile(filepath.Join(dir, ".mise.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := &MiseDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected")
	}
	if r.Kind != "mise" {
		t.Fatalf("expected kind 'mise', got %q", r.Kind)
	}
	if r.Confidence != 1.0 {
		t.Fatalf("expected confidence 1.0, got %f", r.Confidence)
	}

	tools := r.Details["tools"]
	for _, want := range []string{"node@20.11.0", "go@1.23", "python@3.12"} {
		if !strings.Contains(tools, want) {
			t.Errorf("tools %q missing %q", tools, want)
		}
	}

	if r.Details["config_path"] == "" {
		t.Error("expected config_path in details")
	}
}

func TestMiseDetector_SubdirConfig(t *testing.T) {
	dir := t.TempDir()
	miseDir := filepath.Join(dir, ".mise")
	if err := os.MkdirAll(miseDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `
[tools]
rust = "1.78"
`
	if err := os.WriteFile(filepath.Join(miseDir, "config.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := &MiseDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected")
	}
	if r.Details["tools"] != "rust@1.78" {
		t.Errorf("expected tools 'rust@1.78', got %q", r.Details["tools"])
	}
}

func TestMiseDetector_EmptyToolsSection(t *testing.T) {
	dir := t.TempDir()
	content := `
[settings]
experimental = true

[tools]
`
	if err := os.WriteFile(filepath.Join(dir, ".mise.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := &MiseDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected (config file exists)")
	}
	if r.Details["tools"] != "" {
		t.Errorf("expected empty tools, got %q", r.Details["tools"])
	}
}

func TestMiseDetector_RootConfigPriority(t *testing.T) {
	dir := t.TempDir()
	// Create both; root should win.
	if err := os.WriteFile(filepath.Join(dir, ".mise.toml"), []byte("[tools]\nnode = \"20\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	miseDir := filepath.Join(dir, ".mise")
	if err := os.MkdirAll(miseDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(miseDir, "config.toml"), []byte("[tools]\ngo = \"1.23\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	d := &MiseDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Details["config_path"], ".mise.toml") {
		t.Errorf("expected root config path, got %q", r.Details["config_path"])
	}
	if !strings.Contains(r.Details["tools"], "node@20") {
		t.Errorf("expected root tools, got %q", r.Details["tools"])
	}
}
