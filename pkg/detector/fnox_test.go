package detector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFnoxDetector_Name(t *testing.T) {
	d := &FnoxDetector{}
	if d.Name() != "fnox" {
		t.Fatalf("expected name 'fnox', got %q", d.Name())
	}
}

func TestFnoxDetector_NotDetected(t *testing.T) {
	dir := t.TempDir()
	d := &FnoxDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Detected {
		t.Fatal("expected not detected in empty dir")
	}
}

func TestFnoxDetector_SubsectionBindings(t *testing.T) {
	dir := t.TempDir()
	content := `
[bindings.send-email]
function = "email/send"
secrets = ["SMTP_PASS", "SMTP_USER"]

[bindings.fetch-data]
function = "data/fetch"
secrets = ["API_KEY"]
`
	if err := os.WriteFile(filepath.Join(dir, "fnox.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := &FnoxDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected")
	}
	if r.Kind != "fnox" {
		t.Fatalf("expected kind 'fnox', got %q", r.Kind)
	}
	if r.Details["bindings_count"] != "2" {
		t.Errorf("expected 2 bindings, got %q", r.Details["bindings_count"])
	}
	if r.Details["config_path"] == "" {
		t.Error("expected config_path in details")
	}
}

func TestFnoxDetector_FlatBindings(t *testing.T) {
	dir := t.TempDir()
	content := `
[bindings]
notify = "slack/notify"
deploy = "infra/deploy"
backup = "db/backup"
`
	if err := os.WriteFile(filepath.Join(dir, "fnox.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := &FnoxDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected")
	}
	if r.Details["bindings_count"] != "3" {
		t.Errorf("expected 3 bindings, got %q", r.Details["bindings_count"])
	}
}

func TestFnoxDetector_DotPrefixed(t *testing.T) {
	dir := t.TempDir()
	content := `
[bindings.auth]
function = "auth/verify"
secrets = ["JWT_SECRET"]
`
	if err := os.WriteFile(filepath.Join(dir, ".fnox.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := &FnoxDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected with dot-prefixed file")
	}
	if r.Details["bindings_count"] != "1" {
		t.Errorf("expected 1 binding, got %q", r.Details["bindings_count"])
	}
}

func TestFnoxDetector_NoBindings(t *testing.T) {
	dir := t.TempDir()
	content := `
[metadata]
name = "my-fnox-app"
version = "1.0"
`
	if err := os.WriteFile(filepath.Join(dir, "fnox.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := &FnoxDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected (file exists)")
	}
	if r.Details["bindings_count"] != "0" {
		t.Errorf("expected 0 bindings, got %q", r.Details["bindings_count"])
	}
}

func TestFnoxDetector_Priority(t *testing.T) {
	dir := t.TempDir()
	c1 := "[bindings.a]\nfunction = \"x\"\n"
	c2 := "[bindings.b]\nfunction = \"y\"\n[bindings.c]\nfunction = \"z\"\n"
	if err := os.WriteFile(filepath.Join(dir, "fnox.toml"), []byte(c1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".fnox.toml"), []byte(c2), 0644); err != nil {
		t.Fatal(err)
	}

	d := &FnoxDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	// Non-dotted should win.
	if r.Details["bindings_count"] != "1" {
		t.Errorf("expected 1 (non-dotted wins), got %q", r.Details["bindings_count"])
	}
}
