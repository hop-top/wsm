package detector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOrgPackageDetector_Name(t *testing.T) {
	d := &OrgPackageDetector{}
	if d.Name() != "org-package" {
		t.Fatalf("expected name 'org-package', got %q", d.Name())
	}
}

func TestOrgPackageDetector_NotDetected(t *testing.T) {
	dir := t.TempDir()
	d := &OrgPackageDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Detected {
		t.Fatal("expected not detected in empty dir")
	}
	if r.Kind != OrgPackageKind {
		t.Fatalf("expected kind %q, got %q", OrgPackageKind, r.Kind)
	}
}

func TestOrgPackageDetector_WithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: Acme Corp
slug: acme-corp
---

# Acme Corp

Company overview.
`
	if err := os.WriteFile(filepath.Join(dir, "COMPANY.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &OrgPackageDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected")
	}
	if r.Kind != OrgPackageKind {
		t.Fatalf("expected kind %q, got %q", OrgPackageKind, r.Kind)
	}
	if r.Details["path"] != "COMPANY.md" {
		t.Errorf("expected path 'COMPANY.md', got %q", r.Details["path"])
	}
	if r.Details["name"] != "Acme Corp" {
		t.Errorf("expected name 'Acme Corp', got %q", r.Details["name"])
	}
	if r.Details["slug"] != "acme-corp" {
		t.Errorf("expected slug 'acme-corp', got %q", r.Details["slug"])
	}
	if r.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %v", r.Confidence)
	}
}

func TestOrgPackageDetector_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	content := "# Acme Corp\n\nNo frontmatter here.\n"
	if err := os.WriteFile(filepath.Join(dir, "COMPANY.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &OrgPackageDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected without frontmatter")
	}
	if r.Details["path"] != "COMPANY.md" {
		t.Errorf("expected path 'COMPANY.md', got %q", r.Details["path"])
	}
	if _, ok := r.Details["name"]; ok {
		t.Errorf("expected no name, got %q", r.Details["name"])
	}
	if _, ok := r.Details["slug"]; ok {
		t.Errorf("expected no slug, got %q", r.Details["slug"])
	}
}

func TestOrgPackageDetector_MalformedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: [unclosed
  slug: : bad
---
# Broken
`
	if err := os.WriteFile(filepath.Join(dir, "COMPANY.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &OrgPackageDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detected {
		t.Fatal("expected detected despite malformed frontmatter")
	}
	if r.Details["path"] != "COMPANY.md" {
		t.Errorf("expected path 'COMPANY.md', got %q", r.Details["path"])
	}
	if _, ok := r.Details["name"]; ok {
		t.Errorf("expected no name on parse failure, got %q", r.Details["name"])
	}
	if _, ok := r.Details["slug"]; ok {
		t.Errorf("expected no slug on parse failure, got %q", r.Details["slug"])
	}
}

func TestOrgPackageDetector_NotInBuiltins(t *testing.T) {
	r := NewRegistry()
	RegisterBuiltins(r)
	for _, d := range r.Detectors() {
		if d.Name() == "org-package" {
			t.Fatal("org-package must not auto-register with builtins")
		}
	}
}

func TestOrgPackageDetector_RegistryRunAll(t *testing.T) {
	dir := t.TempDir()
	content := "---\nname: Acme Corp\nslug: acme-corp\n---\n# Acme\n"
	if err := os.WriteFile(filepath.Join(dir, "COMPANY.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	r.Register(&OrgPackageDetector{})
	results, err := r.RunAll(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	res := results[0]
	if res.DetectorName != "org-package" {
		t.Errorf("expected detector name 'org-package', got %q", res.DetectorName)
	}
	if !res.Detected || res.Kind != OrgPackageKind {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestOrgPackageDetector_NoRecursion(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "COMPANY.md"), []byte("# Nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &OrgPackageDetector{}
	r, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Detected {
		t.Fatal("expected no detection for nested marker (root only)")
	}
}
