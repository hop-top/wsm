package detector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSecretSpecDetector_Name(t *testing.T) {
	d := &SecretSpecDetector{}
	if d.Name() != "secretspec" {
		t.Fatalf("expected name 'secretspec', got %q", d.Name())
	}
}

func TestSecretSpecDetector_NotDetected(t *testing.T) {
	dir := t.TempDir()
	d := &SecretSpecDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Detected {
		t.Fatal("expected not detected in empty dir")
	}
}

func TestSecretSpecDetector_SubsectionFormat(t *testing.T) {
	dir := t.TempDir()
	content := `
[secrets.DATABASE_URL]
required = true

[secrets.API_KEY]
required = true

[secrets.OPTIONAL_TOKEN]
required = false
`
	if err := os.WriteFile(filepath.Join(dir, "secretspec.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := &SecretSpecDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected")
	}
	if r.Details["secrets_count"] != "3" {
		t.Errorf("expected 3 secrets, got %q", r.Details["secrets_count"])
	}
	if r.Details["secret_keys"] != "DATABASE_URL,API_KEY,OPTIONAL_TOKEN" {
		t.Errorf("unexpected keys: %q", r.Details["secret_keys"])
	}
}

func TestSecretSpecDetector_FlatFormat(t *testing.T) {
	dir := t.TempDir()
	content := `
[secrets]
DB_HOST = "required"
DB_PASS = "required"
`
	if err := os.WriteFile(filepath.Join(dir, "secretspec.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := &SecretSpecDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected")
	}
	if r.Details["secrets_count"] != "2" {
		t.Errorf("expected 2 secrets, got %q", r.Details["secrets_count"])
	}
}

func TestSecretSpecDetector_DotPrefixed(t *testing.T) {
	dir := t.TempDir()
	content := `
[secrets.TOKEN]
required = true
`
	if err := os.WriteFile(filepath.Join(dir, ".secretspec.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := &SecretSpecDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected with dot-prefixed file")
	}
	if r.Details["secrets_count"] != "1" {
		t.Errorf("expected 1 secret, got %q", r.Details["secrets_count"])
	}
}

func TestSecretSpecDetector_EmptySecrets(t *testing.T) {
	dir := t.TempDir()
	content := `
[metadata]
version = "1.0"
`
	if err := os.WriteFile(filepath.Join(dir, "secretspec.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := &SecretSpecDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected (file exists)")
	}
	if r.Details["secrets_count"] != "0" {
		t.Errorf("expected 0 secrets, got %q", r.Details["secrets_count"])
	}
}

func TestSecretSpecDetector_Priority(t *testing.T) {
	dir := t.TempDir()
	// Create both; non-dotted should win.
	c1 := "[secrets.A]\nrequired = true\n"
	c2 := "[secrets.B]\nrequired = true\n[secrets.C]\nrequired = true\n"
	if err := os.WriteFile(filepath.Join(dir, "secretspec.toml"), []byte(c1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".secretspec.toml"), []byte(c2), 0644); err != nil {
		t.Fatal(err)
	}

	d := &SecretSpecDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Details["secrets_count"] != "1" {
		t.Errorf("expected 1 (non-dotted wins), got %q", r.Details["secrets_count"])
	}
}
