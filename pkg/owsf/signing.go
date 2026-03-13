package owsf

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
)

const signingKeyFile = "signing.key"

// LoadOrCreateSigningKey loads the ed25519 private key from dataDir/signing.key,
// generating and persisting a new keypair if the file does not exist.
func LoadOrCreateSigningKey(dataDir string) (ed25519.PrivateKey, error) {
	path := filepath.Join(dataDir, signingKeyFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return createAndSaveKey(path)
	}
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "ED25519 PRIVATE KEY" {
		return nil, errors.New("invalid signing key file")
	}
	if len(block.Bytes) != ed25519.PrivateKeySize {
		return nil, errors.New("signing key has wrong size")
	}
	return ed25519.PrivateKey(block.Bytes), nil
}

func createAndSaveKey(path string) (ed25519.PrivateKey, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	block := &pem.Block{
		Type:  "ED25519 PRIVATE KEY",
		Bytes: []byte(priv),
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return nil, err
	}
	return priv, nil
}

// SignManifest signs manifestHash with privKey and returns a base64-encoded signature.
func SignManifest(privKey ed25519.PrivateKey, manifestHash string) (string, error) {
	sig := ed25519.Sign(privKey, []byte(manifestHash))
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyManifest checks that sig (base64) is a valid ed25519 signature of manifestHash.
func VerifyManifest(pubKey ed25519.PublicKey, manifestHash, sig string) bool {
	raw, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(pubKey, []byte(manifestHash), raw)
}
