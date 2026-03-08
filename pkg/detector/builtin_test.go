package detector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMakefileDetector(t *testing.T) {
	dir := t.TempDir()
	d := NewMakefileDetector()

	// Not detected
	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Detected {
		t.Error("expected not detected without Makefile")
	}

	// Detected
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte("all:"), 0644); err != nil {
		t.Fatal(err)
	}
	res, err = d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Detected {
		t.Error("expected detected with Makefile")
	}
	if res.Kind != "build" {
		t.Errorf("expected kind=build, got %s", res.Kind)
	}
	if res.Details["file"] != "Makefile" {
		t.Errorf("expected file=Makefile, got %s", res.Details["file"])
	}
}

func TestPackageJSONDetector(t *testing.T) {
	dir := t.TempDir()
	d := NewPackageJSONDetector()

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Detected {
		t.Error("expected not detected")
	}

	if err := os.WriteFile(
		filepath.Join(dir, "package.json"), []byte("{}"), 0644,
	); err != nil {
		t.Fatal(err)
	}
	res, err = d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Detected {
		t.Error("expected detected")
	}
	if res.Kind != "runtime" {
		t.Errorf("expected kind=runtime, got %s", res.Kind)
	}
}

func TestDockerComposeDetector(t *testing.T) {
	dir := t.TempDir()
	d := NewDockerComposeDetector()

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Detected {
		t.Error("expected not detected")
	}

	if err := os.WriteFile(
		filepath.Join(dir, "docker-compose.yml"), []byte("version: '3'"), 0644,
	); err != nil {
		t.Fatal(err)
	}
	res, err = d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Detected {
		t.Error("expected detected")
	}
}

func TestDevcontainerDetector(t *testing.T) {
	dir := t.TempDir()
	d := NewDevcontainerDetector()

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Detected {
		t.Error("expected not detected")
	}

	dcDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(dcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dcDir, "devcontainer.json"), []byte("{}"), 0644,
	); err != nil {
		t.Fatal(err)
	}
	res, err = d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Detected {
		t.Error("expected detected")
	}
}

func TestEnvExampleDetector(t *testing.T) {
	dir := t.TempDir()
	d := NewEnvExampleDetector()

	res, err := d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Detected {
		t.Error("expected not detected")
	}

	if err := os.WriteFile(
		filepath.Join(dir, ".env.example"), []byte("KEY=val"), 0644,
	); err != nil {
		t.Fatal(err)
	}
	res, err = d.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Detected {
		t.Error("expected detected")
	}
}

func TestRegisterBuiltins(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	dets := reg.Detectors()
	if len(dets) != 5 {
		t.Fatalf("expected 5 builtin detectors, got %d", len(dets))
	}
}
