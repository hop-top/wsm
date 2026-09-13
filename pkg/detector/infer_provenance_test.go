package detector

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hop.top/wsm/pkg/model"
)

// The tests below pin a REQUIREMENT, not current behaviour: an agent
// consuming an inference must be able to tell a positive determination
// from a fallback that fired because nothing matched. Before this
// change the two were indistinguishable, so none of these assertions
// could have been written against the old output.

// TestInference_SchemeMatchIsDetermined pins the requirement that a
// boundary whose scheme matched discloses scheme provenance and does
// NOT report itself as defaulted.
func TestInference_SchemeMatchIsDetermined(t *testing.T) {
	for _, boundary := range []string{
		"https://api.example.com",
		"docker://myapp",
		"ssh://host.example.com",
		"postgres://localhost/db",
	} {
		inf := InferSpaceFromBoundary(context.Background(), boundary)
		for _, field := range []string{"type", "trust_level"} {
			f, ok := inf.Inferred[field]
			if !ok {
				t.Fatalf("%s: no provenance disclosed for %q", boundary, field)
			}
			if f.Source != SourceBoundaryScheme {
				t.Errorf("%s: %s source = %q, want %q",
					boundary, field, f.Source, SourceBoundaryScheme)
			}
			if !f.Determined() {
				t.Errorf("%s: %s reported as not determined", boundary, field)
			}
			if f.Reason == "" {
				t.Errorf("%s: %s disclosed an empty reason", boundary, field)
			}
		}
		if !inf.Determined() {
			t.Errorf("%s: inference reported as not determined", boundary)
		}
	}
}

// TestInference_NothingMatchedIsDefaulted pins the central requirement:
// an empty directory matches no scheme and no detector, so the
// filesystem/local result must disclose itself as a default, not as a
// finding. This is the case the gap made invisible.
func TestInference_NothingMatchedIsDefaulted(t *testing.T) {
	inf := InferSpaceFromBoundary(context.Background(), t.TempDir())

	if inf.Type != model.SpaceFilesystem || inf.TrustLevel != model.TrustLocal {
		t.Fatalf("unexpected fallback values: type=%s trust=%s",
			inf.Type, inf.TrustLevel)
	}
	for _, field := range []string{"type", "trust_level"} {
		f, ok := inf.Inferred[field]
		if !ok {
			t.Fatalf("no provenance disclosed for %q", field)
		}
		if f.Source != SourceDefault {
			t.Errorf("%s source = %q, want %q", field, f.Source, SourceDefault)
		}
		if f.Determined() {
			t.Errorf("%s claims to be determined, but nothing matched", field)
		}
	}
	if inf.Determined() {
		t.Error("inference claims determined, but nothing matched")
	}
	// Nothing was consulted, so nothing may be claimed as a path.
	if p := inf.Inferred["type"].Path; p != "" {
		t.Errorf("defaulted field fabricated a path: %q", p)
	}
	// No detector matched, so no capabilities provenance may appear.
	if _, ok := inf.Inferred["capabilities"]; ok {
		t.Error("capabilities provenance emitted with no detections")
	}
}

// TestInference_DetectorMatchNamesThePath pins the requirement that a
// detector-determined field names the file that decided it, so the
// claim can be checked against the filesystem.
func TestInference_DetectorMatchNamesThePath(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(compose, []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	inf := InferSpaceFromBoundary(context.Background(), dir)

	f, ok := inf.Inferred["type"]
	if !ok {
		t.Fatal("no provenance disclosed for type")
	}
	if f.Source != SourceDetector {
		t.Errorf("type source = %q, want %q", f.Source, SourceDetector)
	}
	if !f.Determined() {
		t.Error("detector-matched type reported as not determined")
	}
	if f.Path != compose {
		t.Errorf("type path = %q, want the file that decided it %q", f.Path, compose)
	}
}

// TestInference_CapabilitiesDisclosedOnlyWhenDetected pins that the
// capabilities provenance appears exactly when detectors matched, and
// names them.
func TestInference_CapabilitiesDisclosedOnlyWhenDetected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	inf := InferSpaceFromBoundary(context.Background(), dir)

	f, ok := inf.Inferred["capabilities"]
	if !ok {
		t.Fatal("capabilities provenance missing despite a detection")
	}
	if f.Source != SourceDetector {
		t.Errorf("capabilities source = %q, want %q", f.Source, SourceDetector)
	}
	if f.Path == "" {
		t.Error("capabilities provenance consulted a file but disclosed no path")
	}

	// package.json alone must not upgrade the type: that stays a
	// default, and must still say so even though a detector fired.
	tf := inf.Inferred["type"]
	if tf.Source != SourceDefault {
		t.Errorf("type source = %q, want %q (package.json must not decide type)",
			tf.Source, SourceDefault)
	}
	if inf.Determined() {
		t.Error("inference claims fully determined while type was defaulted")
	}
}

// TestInference_NoEmptyProvenanceEntries pins the honesty requirement
// from the factor: wsm must not emit provenance entries it has no real
// source for. Every disclosed field carries a source and a reason.
func TestInference_NoEmptyProvenanceEntries(t *testing.T) {
	dirs := []string{t.TempDir(), "https://example.com", "ssh://h"}
	withCompose := t.TempDir()
	if err := os.WriteFile(filepath.Join(withCompose, "compose.yml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs = append(dirs, withCompose)

	for _, b := range dirs {
		inf := InferSpaceFromBoundary(context.Background(), b)
		if len(inf.Inferred) == 0 {
			t.Fatalf("%s: no provenance disclosed at all", b)
		}
		for name, f := range inf.Inferred {
			if f.Source == "" {
				t.Errorf("%s: field %q has an empty source", b, name)
			}
			if f.Reason == "" {
				t.Errorf("%s: field %q has an empty reason", b, name)
			}
		}
	}
}
