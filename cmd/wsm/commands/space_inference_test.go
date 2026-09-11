package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"hop.top/wsm/pkg/detector"
	"hop.top/wsm/pkg/workspace"
)

func sampleSpace() workspace.Space {
	return workspace.Space{
		ID:         "sp-1",
		Type:       workspace.SpaceFilesystem,
		Boundary:   "/tmp/x",
		TrustLevel: workspace.TrustLocal,
	}
}

// TestWithInference_NoAutoIsByteIdenticalToBareSpace pins a
// BACKWARD-COMPATIBILITY REQUIREMENT: without --auto the emitted
// document must be exactly the space, so existing JSON consumers see
// no change at all. Asserted by comparing against the independently
// marshalled Space rather than against a copied literal, so the test
// cannot drift into pinning whatever the wrapper happens to emit.
func TestWithInference_NoAutoIsByteIdenticalToBareSpace(t *testing.T) {
	s := sampleSpace()

	want, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(withInference(s, nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("payload changed without --auto:\n got: %s\nwant: %s", got, want)
	}
	if strings.Contains(string(got), "_inferred") {
		t.Error("_inferred present without --auto")
	}
}

// TestWithInference_AutoAddsOnlyTheInferredKey pins that the
// disclosure is ADDITIVE: every key of the bare space survives
// unchanged and exactly one new top-level key appears.
func TestWithInference_AutoAddsOnlyTheInferredKey(t *testing.T) {
	s := sampleSpace()
	inf := detector.InferSpaceFromBoundary(context.Background(), t.TempDir())

	bare := map[string]any{}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &bare); err != nil {
		t.Fatal(err)
	}

	withRaw, err := json.Marshal(withInference(s, &inf))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	if err := json.Unmarshal(withRaw, &got); err != nil {
		t.Fatal(err)
	}

	if _, ok := got["_inferred"]; !ok {
		t.Fatal("_inferred missing with --auto")
	}
	for k, v := range bare {
		gv, ok := got[k]
		if !ok {
			t.Errorf("existing key %q dropped when --auto added disclosure", k)
			continue
		}
		if string(mustJSON(t, gv)) != string(mustJSON(t, v)) {
			t.Errorf("existing key %q changed value: got %v want %v", k, gv, v)
		}
	}
	if len(got) != len(bare)+1 {
		t.Errorf("expected exactly one added key, got %d vs %d", len(got), len(bare))
	}
}

// TestWithInference_DefaultedIsLegibleAsNotDetermined pins the
// requirement that an agent can read determined=false out of the
// structured payload when nothing matched.
func TestWithInference_DefaultedIsLegibleAsNotDetermined(t *testing.T) {
	inf := detector.InferSpaceFromBoundary(context.Background(), t.TempDir())

	raw := mustJSON(t, withInference(sampleSpace(), &inf))
	var doc struct {
		Inferred struct {
			Determined bool `json:"determined"`
			Applied    bool `json:"applied"`
			Fields     map[string]struct {
				Source string `json:"source"`
			} `json:"fields"`
		} `json:"_inferred"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	if doc.Inferred.Determined {
		t.Error("determined=true for an inference where nothing matched")
	}
	if doc.Inferred.Applied {
		t.Error("applied=true: wsm discloses inference, it does not apply it")
	}
	if doc.Inferred.Fields["type"].Source != "default" {
		t.Errorf("type source = %q, want \"default\"",
			doc.Inferred.Fields["type"].Source)
	}
}

// TestWithInference_SchemeMatchIsLegibleAsDetermined is the other half
// of the side-by-side: a matched boundary must read as determined.
func TestWithInference_SchemeMatchIsLegibleAsDetermined(t *testing.T) {
	inf := detector.InferSpaceFromBoundary(context.Background(), "https://api.example.com")

	raw := mustJSON(t, withInference(sampleSpace(), &inf))
	var doc struct {
		Inferred struct {
			Determined bool `json:"determined"`
			Fields     map[string]struct {
				Source string `json:"source"`
			} `json:"fields"`
		} `json:"_inferred"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	if !doc.Inferred.Determined {
		t.Error("determined=false for a boundary whose scheme matched")
	}
	if got := doc.Inferred.Fields["type"].Source; got != "boundary-scheme" {
		t.Errorf("type source = %q, want \"boundary-scheme\"", got)
	}
}

// TestReportInference_DistinguishesDeterminedFromDefaulted pins that
// the human disclosure carries the same distinction as the JSON.
func TestReportInference_DistinguishesDeterminedFromDefaulted(t *testing.T) {
	var defaulted bytes.Buffer
	reportInference(&defaulted,
		detector.InferSpaceFromBoundary(context.Background(), t.TempDir()))
	if !strings.Contains(defaulted.String(), "defaulted") {
		t.Errorf("empty-dir report does not say defaulted:\n%s", defaulted.String())
	}

	var determined bytes.Buffer
	reportInference(&determined,
		detector.InferSpaceFromBoundary(context.Background(), "docker://app"))
	if !strings.Contains(determined.String(), "determined") {
		t.Errorf("scheme-match report does not say determined:\n%s", determined.String())
	}
	if strings.Contains(determined.String(), "defaulted") {
		t.Errorf("scheme-match report wrongly says defaulted:\n%s", determined.String())
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
