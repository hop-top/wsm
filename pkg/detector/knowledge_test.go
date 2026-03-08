package detector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageRegistryDetectorNone(t *testing.T) {
	dir := t.TempDir()
	d := NewPackageRegistryDetector()

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Detected {
		t.Error("expected not detected in empty dir")
	}
}

func TestPackageRegistryDetectorGoMod(t *testing.T) {
	dir := t.TempDir()
	d := NewPackageRegistryDetector()

	writeFile(t, dir, "go.mod", "module example.com/foo\n")

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Detected {
		t.Fatal("expected detected with go.mod")
	}
	if res.Details["registry_go"] != "go.mod" {
		t.Errorf("expected registry_go=go.mod, got %s", res.Details["registry_go"])
	}
}

func TestPackageRegistryDetectorMultiple(t *testing.T) {
	dir := t.TempDir()
	d := NewPackageRegistryDetector()

	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "pyproject.toml", "[project]")

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Detected {
		t.Fatal("expected detected")
	}
	if res.Details["registry_npm"] != "package.json" {
		t.Error("expected npm registry detected")
	}
	if res.Details["registry_pypi"] != "pyproject.toml" {
		t.Error("expected pypi registry detected")
	}
}

func TestLocalDocsDetectorNone(t *testing.T) {
	dir := t.TempDir()
	d := NewLocalDocsDetector()

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Detected {
		t.Error("expected not detected in empty dir")
	}
}

func TestLocalDocsDetectorWithDocs(t *testing.T) {
	dir := t.TempDir()
	d := NewLocalDocsDetector()

	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "README.md", "# Hello")
	writeFile(t, dir, "CONTRIBUTING.md", "# Contributing")

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Detected {
		t.Fatal("expected detected")
	}
	if res.Details["dir_docs"] != "true" {
		t.Error("expected docs dir detected")
	}
	if res.Details["file_readme"] != "README.md" {
		t.Errorf("expected file_readme=README.md, got %s", res.Details["file_readme"])
	}
	if res.Details["file_contributing"] != "CONTRIBUTING.md" {
		t.Error("expected contributing file detected")
	}
	if res.Confidence <= 0 || res.Confidence > 1.0 {
		t.Errorf("confidence out of range: %f", res.Confidence)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(
		filepath.Join(dir, name), []byte(content), 0644,
	); err != nil {
		t.Fatal(err)
	}
}
