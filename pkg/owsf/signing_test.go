package owsf

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateSigningKey_CreatesOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	key, err := LoadOrCreateSigningKey(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(key) == 0 {
		t.Fatal("expected non-empty private key")
	}
	// File must be created
	if _, err := os.Stat(filepath.Join(dir, "signing.key")); err != nil {
		t.Fatalf("key file not created: %v", err)
	}
}

func TestLoadOrCreateSigningKey_LoadsExisting(t *testing.T) {
	dir := t.TempDir()
	key1, err := LoadOrCreateSigningKey(dir)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	key2, err := LoadOrCreateSigningKey(dir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if string(key1) != string(key2) {
		t.Fatal("key changed between loads")
	}
}

func TestSignAndVerifyManifest(t *testing.T) {
	dir := t.TempDir()
	priv, err := LoadOrCreateSigningKey(dir)
	if err != nil {
		t.Fatalf("load key: %v", err)
	}
	hash := ManifestHash([]string{"h1", "h2"})

	sig, err := SignManifest(priv, hash)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if sig == "" {
		t.Fatal("expected non-empty signature")
	}

	pub := priv.Public().(ed25519.PublicKey)
	if !VerifyManifest(pub, hash, sig) {
		t.Fatal("signature did not verify")
	}
}

func TestVerifyManifest_WrongHash(t *testing.T) {
	dir := t.TempDir()
	priv, _ := LoadOrCreateSigningKey(dir)
	sig, _ := SignManifest(priv, "correct-hash")
	pub := priv.Public().(ed25519.PublicKey)
	if VerifyManifest(pub, "wrong-hash", sig) {
		t.Fatal("expected verification to fail for wrong hash")
	}
}
