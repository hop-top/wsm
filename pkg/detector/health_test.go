package detector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDependencyHealthDetectorNone(t *testing.T) {
	dir := t.TempDir()
	d := NewDependencyHealthDetector()

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Detected {
		t.Error("expected not detected in empty dir")
	}
}

func TestDependencyHealthDetectorGoMod(t *testing.T) {
	dir := t.TempDir()
	d := NewDependencyHealthDetector()

	gomod := `module example.com/foo

go 1.21

require (
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.8.0
)
`
	if err := os.WriteFile(
		filepath.Join(dir, "go.mod"), []byte(gomod), 0644,
	); err != nil {
		t.Fatal(err)
	}

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Detected {
		t.Fatal("expected detected")
	}
	if res.Details["file_go"] != "go.mod" {
		t.Error("expected file_go=go.mod")
	}
	if res.Details["deps_go"] != "2" {
		t.Errorf("expected deps_go=2, got %s", res.Details["deps_go"])
	}
	if res.Details["total_deps"] != "2" {
		t.Errorf("expected total_deps=2, got %s", res.Details["total_deps"])
	}
}

func TestDependencyHealthDetectorRequirementsTxt(t *testing.T) {
	dir := t.TempDir()
	d := NewDependencyHealthDetector()

	reqs := "flask==2.0\nrequests>=2.28\n# comment\n\nnumpy\n"
	if err := os.WriteFile(
		filepath.Join(dir, "requirements.txt"), []byte(reqs), 0644,
	); err != nil {
		t.Fatal(err)
	}

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Detected {
		t.Fatal("expected detected")
	}
	if res.Details["deps_pip"] != "3" {
		t.Errorf("expected deps_pip=3, got %s", res.Details["deps_pip"])
	}
}

func TestDependencyHealthDetectorGemfile(t *testing.T) {
	dir := t.TempDir()
	d := NewDependencyHealthDetector()

	gemfile := "source 'https://rubygems.org'\ngem 'rails'\ngem 'pg'\n"
	if err := os.WriteFile(
		filepath.Join(dir, "Gemfile"), []byte(gemfile), 0644,
	); err != nil {
		t.Fatal(err)
	}

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Detected {
		t.Fatal("expected detected")
	}
	if res.Details["deps_ruby"] != "2" {
		t.Errorf("expected deps_ruby=2, got %s", res.Details["deps_ruby"])
	}
}

func TestDependencyHealthDetectorMultiple(t *testing.T) {
	dir := t.TempDir()
	d := NewDependencyHealthDetector()

	gomod := "module x\n\ngo 1.21\n\nrequire (\n\tgithub.com/a v1\n)\n"
	reqs := "flask\n"

	if err := os.WriteFile(
		filepath.Join(dir, "go.mod"), []byte(gomod), 0644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "requirements.txt"), []byte(reqs), 0644,
	); err != nil {
		t.Fatal(err)
	}

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Details["total_files"] != "2" {
		t.Errorf("expected total_files=2, got %s", res.Details["total_files"])
	}
	if res.Details["total_deps"] != "2" {
		t.Errorf("expected total_deps=2, got %s", res.Details["total_deps"])
	}
}
