package irc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// secretFileMode keeps every credential file readable only by its owner.
const secretFileMode os.FileMode = 0o600

// passphraseBytes is the entropy behind a generated account passphrase: 32
// random bytes, base64url-encoded for safe transport through IRC commands and
// YAML.
const passphraseBytes = 32

// apsTimeout bounds an `aps secret get` call so a hung helper cannot wedge the
// daemon's reconcile loop.
const apsTimeout = 10 * time.Second

// EnvAPSBin overrides the aps binary used to resolve secret references. Tests
// point it at a fake script; operators can point it at a non-PATH install.
const EnvAPSBin = "APS_BIN"

// defaultAPSBin is the aps binary looked up on PATH when EnvAPSBin is unset.
const defaultAPSBin = "aps"

// redactedPlaceholder is what every string rendering of a secret shows in
// place of the value. Passphrases are never logged.
const redactedPlaceholder = "REDACTED"

// ErrNoBotSecret reports that wsm holds no password for the bot account: no
// aps secret_ref is configured and secrets.yaml has no entry for the account.
// Callers surface the fix (`--bot-password-stdin`, or set bot.secret_ref).
var ErrNoBotSecret = errors.New("irc: no bot secret")

// ErrNoAPIToken reports that no ergo HTTP API token could be resolved from the
// host profile, the configured aps reference, or the workspace's token file.
var ErrNoAPIToken = errors.New("irc: no api token")

// Secrets maps an account name to its passphrase. It is persisted at
// Paths.Secrets (secrets.yaml, 0600, in a 0700 directory). In managed mode it
// holds every account wsm created; in external mode it holds the bot account
// only, and only when the password was not delegated to aps.
//
// The zero value is not usable for Set; use LoadSecrets, which returns an
// initialised map even when the file does not exist.
type Secrets map[string]string

// String renders Secrets without any passphrase. Secrets is frequently
// embedded in structs that get %v-formatted into logs and error messages; this
// method is the guarantee that such a formatting never leaks a credential.
func (s Secrets) String() string {
	return fmt.Sprintf("irc.Secrets(%d accounts, %s)", len(s), redactedPlaceholder)
}

// GoString renders Secrets without any passphrase under %#v, which would
// otherwise bypass String and dump the whole map.
func (s Secrets) GoString() string { return s.String() }

// MarshalJSON refuses to serialise passphrases: Secrets reaches JSON only by
// accident (a status payload, a debug dump), never deliberately, so the safe
// rendering is the redacted one. The on-disk form is YAML, written by Save.
func (s Secrets) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", s.String())), nil
}

// Get returns the passphrase for account and whether one is held.
func (s Secrets) Get(account string) (string, bool) {
	if s == nil {
		return "", false
	}
	v, ok := s[account]
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// Set records the passphrase for account. It is a no-op on a nil map, which
// cannot be written to; callers obtain a usable map from LoadSecrets.
func (s Secrets) Set(account, passphrase string) {
	if s == nil {
		return
	}
	s[account] = passphrase
}

// Rotate generates a fresh passphrase for account, records it, and returns it.
// The caller is responsible for pushing the new value to the server (provider
// SetPassword) and for calling Save; Rotate never writes to disk on its own, so
// a failed server-side change cannot strand a secret wsm cannot use.
func (s Secrets) Rotate(account string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("irc secrets: rotate %s: no secrets store loaded", account)
	}
	if account == "" {
		return "", fmt.Errorf("irc secrets: rotate: account is required")
	}
	pass, err := GeneratePassphrase()
	if err != nil {
		return "", err
	}
	s[account] = pass
	return pass, nil
}

// GeneratePassphrase returns 32 cryptographically random bytes encoded with
// the URL-safe, unpadded base64 alphabet: no characters that need quoting in
// YAML, an IRC command line, or an ergo config.
func GeneratePassphrase() (string, error) {
	buf := make([]byte, passphraseBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("irc secrets: generate passphrase: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// LoadSecrets reads the secrets store at path. A missing file yields an empty,
// usable store and no error: a workspace that has never created an account is
// not a fault. Any other read or parse failure is reported, so a corrupt file
// is never silently treated as "no secrets" and overwritten.
func LoadSecrets(path string) (Secrets, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Secrets{}, nil
		}
		return nil, fmt.Errorf("read irc secrets %s: %w", path, err)
	}
	out := Secrets{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		// The error from yaml can quote offending input; keep it out of the
		// message so a malformed secrets file cannot echo a passphrase.
		return nil, fmt.Errorf("parse irc secrets %s: invalid YAML mapping", path)
	}
	if out == nil {
		out = Secrets{}
	}
	return out, nil
}

// Save writes the store to path at 0600, creating parent directories at 0700.
// The write goes to a temporary file in the destination directory and is
// renamed into place, so a crash or a full disk can never leave a truncated
// secrets file: readers see either the old contents or the new ones.
func (s Secrets) Save(path string) error {
	if path == "" {
		return fmt.Errorf("irc secrets: save: path is required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("create irc secrets dir %s: %w", dir, err)
	}

	store := s
	if store == nil {
		store = Secrets{}
	}
	data, err := yaml.Marshal(map[string]string(store))
	if err != nil {
		return fmt.Errorf("encode irc secrets: %w", err)
	}

	return writeSecretFile(path, dir, ".secrets-*.yaml", data)
}

// LoadAPIToken reads the ergo HTTP API bearer token at path. A missing file is
// reported as ErrNoAPIToken so callers can distinguish "not provisioned yet"
// from a permissions or I/O fault.
func LoadAPIToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("read irc api token %s: %w", path, ErrNoAPIToken)
		}
		return "", fmt.Errorf("read irc api token %s: %w", path, err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("irc api token %s is empty: %w", path, ErrNoAPIToken)
	}
	return token, nil
}

// SaveAPIToken writes the API bearer token to path at 0600, creating parent
// directories at 0700, atomically via a temporary file and a rename.
func SaveAPIToken(path, token string) error {
	if path == "" {
		return fmt.Errorf("irc secrets: save api token: path is required")
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("irc secrets: save api token: token is empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("create irc api token dir %s: %w", dir, err)
	}
	return writeSecretFile(path, dir, ".api-token-*", []byte(token+"\n"))
}

// writeSecretFile performs the shared 0600 atomic write: a temp file in the
// destination directory, chmod before any content is written, then rename.
// Creating the temp file in the same directory keeps the rename on one
// filesystem, which is what makes it atomic.
func writeSecretFile(path, dir, pattern string, data []byte) error {
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return fmt.Errorf("create irc secrets temp file: %w", err)
	}
	tmpName := tmp.Name()
	// Removing the temp file is a no-op once the rename succeeds; on any
	// failure path it is what keeps a partial secret from lingering on disk.
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(secretFileMode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod irc secrets temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write irc secrets: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close irc secrets temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install irc secrets %s: %w", path, err)
	}
	return nil
}

// ReadBotPassword resolves the password wsm's own bot authenticates with.
//
// Precedence, highest first:
//
//  1. cfg.Bot.SecretRef — the password is owned by aps; shell out to
//     `aps secret get <ref>`. wsm never persists the value.
//  2. secrets[cfg.Bot.Account] — the password wsm generated (managed) or was
//     handed via `wsm irc enable --bot-password-stdin` (external).
//
// A configured secret_ref that cannot be resolved is an error, never a silent
// fallback to secrets.yaml: falling back would authenticate with a stale
// password after an operator rotated the secret in aps. Neither source
// yielding a password is ErrNoBotSecret.
func ReadBotPassword(cfg Config, secrets Secrets) (string, error) {
	if ref := strings.TrimSpace(cfg.Bot.SecretRef); ref != "" {
		pass, err := apsSecretGet(ref)
		if err != nil {
			return "", err
		}
		return pass, nil
	}
	if pass, ok := secrets.Get(cfg.Bot.Account); ok {
		return pass, nil
	}
	return "", fmt.Errorf("irc bot account %q: %w", cfg.Bot.Account, ErrNoBotSecret)
}

// ReadAPIToken resolves the ergo HTTP API bearer token for this workspace.
//
// Precedence, highest first:
//
//  1. hostPaths — when this workspace is a guest on another workspace's server
//     (Profile.HostWorkspace set), the token is the host's. It is read straight
//     out of the host's own api.token: same machine, same user, same $DataDir.
//     The token is deliberately never copied into the guest's data dir, so the
//     host rotating it takes effect immediately and revoking the host's
//     workspace revokes the guest's access too.
//  2. cfg.Server.ErgoAPITokenRef — an external ergo whose token aps owns.
//  3. paths.APIToken — the managed token wsm generated for its own ergo.
//
// hostPaths is a pointer because "no host" is the common case and must be
// expressible; a non-nil hostPaths is what selects branch 1, so the caller
// passes it only after reading Profile.HostWorkspace.
func ReadAPIToken(cfg Config, paths Paths, hostPaths *Paths) (string, error) {
	if hostPaths != nil {
		if hostPaths.APIToken == "" {
			return "", fmt.Errorf("irc api token: host workspace has no token path: %w", ErrNoAPIToken)
		}
		return LoadAPIToken(hostPaths.APIToken)
	}
	if ref := strings.TrimSpace(cfg.Server.ErgoAPITokenRef); ref != "" {
		return apsSecretGet(ref)
	}
	if paths.APIToken == "" {
		return "", fmt.Errorf("irc api token: no token path resolved: %w", ErrNoAPIToken)
	}
	return LoadAPIToken(paths.APIToken)
}

// apsSecretGet resolves one aps secret reference by running
// `aps secret get <ref>` and taking its trimmed stdout as the value.
//
// The binary is EnvAPSBin when set, else `aps` from PATH. Failures report the
// reference and aps's stderr, never stdout: stdout is the secret.
func apsSecretGet(ref string) (string, error) {
	bin := strings.TrimSpace(os.Getenv(EnvAPSBin))
	if bin == "" {
		bin = defaultAPSBin
	}

	ctx, cancel := context.WithTimeout(context.Background(), apsTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "secret", "get", ref)
	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("resolve aps secret %s: timed out after %s", ref, apsTimeout)
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return "", fmt.Errorf("resolve aps secret %s: %s", ref, detail)
		}
		return "", fmt.Errorf("resolve aps secret %s: %w", ref, err)
	}

	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", fmt.Errorf("aps secret %s is empty: %w", ref, ErrNoBotSecret)
	}
	return value, nil
}
