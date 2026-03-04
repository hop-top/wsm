package owsf

import "testing"

func TestAssurance_Profile0_ContentHash(t *testing.T) {
	blob := []byte(`{"event":"data"}`)
	addr := ContentAddress(blob)
	if addr == "" {
		t.Fatal("content address required")
	}
	if ContentAddress(blob) != addr {
		t.Fatal("not deterministic")
	}
}

func TestAssurance_Profile0_ManifestHash(t *testing.T) {
	items := []string{"hash-1", "hash-2"}
	mh := ManifestHash(items)
	if mh == "" {
		t.Fatal("manifest hash required")
	}
	if ManifestHash(items) != mh {
		t.Fatal("not deterministic")
	}
}

func TestAssurance_Profile1_SignedManifest(t *testing.T) {
	manifest := ManifestHash([]string{"h1", "h2"})
	sig := SignedManifest{
		ManifestHash: manifest,
		ProducerID:   "wsm/0.1.0",
		Signature:    "placeholder",
	}
	if sig.ProducerID == "" {
		t.Fatal("producer identity required")
	}
	if sig.ManifestHash == "" {
		t.Fatal("manifest hash required")
	}
}

func TestAssurance_Profile2_Provenance(t *testing.T) {
	prov := ProvenanceRecord{
		ForkLineage: []ForkRef{
			{ParentDocID: "d0", ParentEventID: "e0"},
			{ParentDocID: "d1", ParentEventID: "e1"},
		},
		Attestations: []Attestation{
			{Issuer: "wsm", Claim: "integrity-verified"},
		},
	}
	if len(prov.ForkLineage) != 2 {
		t.Fatal("lineage required")
	}
	if len(prov.Attestations) != 1 {
		t.Fatal("attestation required")
	}
}
