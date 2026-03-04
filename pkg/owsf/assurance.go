package owsf

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type AssuranceProfile int

const (
	ProfileIntegrity  AssuranceProfile = 0
	ProfileSigned     AssuranceProfile = 1
	ProfileProvenance AssuranceProfile = 2
)

func ContentAddress(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func ManifestHash(contentHashes []string) string {
	combined := strings.Join(contentHashes, "\n")
	h := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(h[:])
}

// Profile 1: signed manifest type (I5: actual signing deferred)
type SignedManifest struct {
	ManifestHash string `json:"manifest_hash"`
	ProducerID   string `json:"producer_id"`
	Signature    string `json:"signature"`
}

// Profile 2: provenance
type Attestation struct {
	Issuer    string `json:"issuer"`
	Claim     string `json:"claim"`
	Signature string `json:"signature,omitempty"`
}

type ProvenanceRecord struct {
	ForkLineage  []ForkRef     `json:"fork_lineage"`
	Attestations []Attestation `json:"attestations"`
	LogRef       string        `json:"log_ref,omitempty"`
}
