package irc

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testPassphrase is a recognisable stand-in for a real credential: every
// leak-hunting assertion looks for this exact string.
const testPassphrase = "s3cr3t-passphrase-do-not-leak"

// writeFakeAPS installs a shell script at <dir>/aps that answers
// `aps secret get <ref>` from the supplied table, and points APS_BIN at it for
// the duration of the test. Refs absent from the table exit non-zero with a
// message on stderr, mirroring how the real aps reports an unknown key.
func writeFakeAPS(t *testing.T, values map[string]string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake aps helper is a POSIX shell script")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "aps")

	var cases strings.Builder
	for ref, value := range values {
		// Single-quote the literals so the script cannot interpret them.
		fmt.Fprintf(&cases, "    '%s') printf '%%s\\n' '%s' ;;\n", ref, value)
	}

	script := "#!/bin/sh\n" +
		"if [ \"$1\" != secret ] || [ \"$2\" != get ]; then\n" +
		"  echo \"unexpected args: $*\" >&2\n" +
		"  exit 2\n" +
		"fi\n" +
		"case \"$3\" in\n" +
		cases.String() +
		"    *) echo \"no such secret: $3\" >&2 ; exit 1 ;;\n" +
		"esac\n"

	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake aps: %v", err)
	}
	t.Setenv(EnvAPSBin, path)
	return path
}

func TestGeneratePassphrase(t *testing.T) {
	t.Run("decodes to 32 random bytes", func(t *testing.T) {
		got, err := GeneratePassphrase()
		if err != nil {
			t.Fatalf("GeneratePassphrase: %v", err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(got)
		if err != nil {
			t.Fatalf("passphrase %q is not unpadded base64url: %v", got, err)
		}
		if len(raw) != passphraseBytes {
			t.Errorf("decoded length = %d, want %d", len(raw), passphraseBytes)
		}
	})

	t.Run("is url safe", func(t *testing.T) {
		// base64url must never emit + or /, which would need quoting in an
		// ergo config or an IRC command line.
		for i := 0; i < 32; i++ {
			got, err := GeneratePassphrase()
			if err != nil {
				t.Fatalf("GeneratePassphrase: %v", err)
			}
			if strings.ContainsAny(got, "+/=") {
				t.Fatalf("passphrase %q contains a non-url-safe character", got)
			}
		}
	})

	t.Run("does not repeat", func(t *testing.T) {
		seen := map[string]bool{}
		for i := 0; i < 64; i++ {
			got, err := GeneratePassphrase()
			if err != nil {
				t.Fatalf("GeneratePassphrase: %v", err)
			}
			if seen[got] {
				t.Fatalf("passphrase %q generated twice: source is not random", got)
			}
			seen[got] = true
		}
	})
}

func TestLoadSecretsMissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "irc", "secrets.yaml")

	got, err := LoadSecrets(path)
	if err != nil {
		t.Fatalf("LoadSecrets on a missing file: %v", err)
	}
	if got == nil {
		t.Fatal("LoadSecrets returned a nil map; want an empty usable store")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}

	// The returned store must be writable: a nil map would panic here.
	got.Set("wsm", testPassphrase)
	if v, ok := got.Get("wsm"); !ok || v != testPassphrase {
		t.Errorf("Set/Get round trip failed on a store from a missing file")
	}
}

func TestLoadSecretsCorruptFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	if err := os.WriteFile(path, []byte("::not a mapping::\n\t- broken"), secretFileMode); err != nil {
		t.Fatalf("write corrupt secrets: %v", err)
	}

	// A corrupt file must not read as "no secrets": that would let Save
	// overwrite real credentials with an empty store.
	if _, err := LoadSecrets(path); err == nil {
		t.Fatal("LoadSecrets on a corrupt file returned no error")
	}
}

func TestSecretsSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "irc", "ws", "secrets.yaml")

	store, err := LoadSecrets(path)
	if err != nil {
		t.Fatalf("LoadSecrets: %v", err)
	}
	store.Set("wsm", testPassphrase)
	store.Set("reed-forge", "another-value")

	if err := store.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := LoadSecrets(path)
	if err != nil {
		t.Fatalf("LoadSecrets after Save: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if v, ok := got.Get("wsm"); !ok || v != testPassphrase {
		t.Errorf("wsm = %q/%v, want the saved passphrase", v, ok)
	}
	if v, ok := got.Get("reed-forge"); !ok || v != "another-value" {
		t.Errorf("reed-forge = %q/%v, want the saved passphrase", v, ok)
	}
}

func TestSecretsSavePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits")
	}
	base := t.TempDir()
	// A directory that does not exist yet, so Save has to create it and we
	// can assert the mode it chose rather than t.TempDir's.
	dir := filepath.Join(base, "irc", "01JWORKSPACE")
	path := filepath.Join(dir, "secrets.yaml")

	store := Secrets{"wsm": testPassphrase}
	if err := store.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat secrets: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("secrets.yaml mode = %#o, want 0600", got)
	}

	di, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat secrets dir: %v", err)
	}
	if got := di.Mode().Perm(); got != 0o700 {
		t.Errorf("secrets dir mode = %#o, want 0700", got)
	}
}

func TestSecretsSaveIsAtomicAndLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.yaml")

	store := Secrets{"wsm": testPassphrase}
	if err := store.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Overwrite, which is the case where a non-atomic write would truncate
	// the existing file before writing the new contents.
	store.Set("second", "value-two")
	if err := store.Save(path); err != nil {
		t.Fatalf("Save (overwrite): %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "secrets.yaml" {
		t.Errorf("directory contains %v, want only secrets.yaml (a temp file leaked)", names)
	}

	// The surviving file is the complete new version, not a partial one.
	got, err := LoadSecrets(path)
	if err != nil {
		t.Fatalf("LoadSecrets after overwrite: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2 after overwrite", len(got))
	}
}

func TestSecretsRotate(t *testing.T) {
	store := Secrets{"wsm": testPassphrase}

	got, err := store.Rotate("wsm")
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if got == testPassphrase {
		t.Fatal("Rotate returned the old passphrase")
	}
	if v, _ := store.Get("wsm"); v != got {
		t.Errorf("store holds %q, want the rotated value %q", v, got)
	}

	t.Run("rotating an unknown account creates it", func(t *testing.T) {
		pass, err := store.Rotate("fresh-account")
		if err != nil {
			t.Fatalf("Rotate: %v", err)
		}
		if v, ok := store.Get("fresh-account"); !ok || v != pass {
			t.Errorf("fresh-account = %q/%v, want the rotated value", v, ok)
		}
	})

	t.Run("rotate on a nil store errors", func(t *testing.T) {
		var nilStore Secrets
		if _, err := nilStore.Rotate("wsm"); err == nil {
			t.Error("Rotate on a nil store returned no error")
		}
	})

	t.Run("empty account errors", func(t *testing.T) {
		if _, err := store.Rotate(""); err == nil {
			t.Error("Rotate with an empty account returned no error")
		}
	})
}

func TestSecretsGetMissingAndEmpty(t *testing.T) {
	store := Secrets{"blank": ""}

	if _, ok := store.Get("absent"); ok {
		t.Error("Get on an absent account reported ok")
	}
	// An empty stored value is "no secret", not a secret that is the empty
	// string: authenticating with "" would fail confusingly.
	if _, ok := store.Get("blank"); ok {
		t.Error("Get on an empty value reported ok")
	}

	var nilStore Secrets
	if _, ok := nilStore.Get("wsm"); ok {
		t.Error("Get on a nil store reported ok")
	}
}

// TestSecretsNeverRenderTheirValues is the security property: no string
// rendering of a Secrets value may contain a passphrase, because these structs
// end up in log lines and error messages formatted with %v, %s, %#v and JSON.
func TestSecretsNeverRenderTheirValues(t *testing.T) {
	store := Secrets{"wsm": testPassphrase, "reed-forge": "second-" + testPassphrase}

	renderings := map[string]string{
		"String()": store.String(),
		"%v":       fmt.Sprintf("%v", store),
		"%s":       fmt.Sprintf("%s", store),
		"%q":       fmt.Sprintf("%q", store),
		"%#v":      fmt.Sprintf("%#v", store),
		"%+v":      fmt.Sprintf("%+v", store),
	}

	// Formatting a struct that merely contains a Secrets field must be safe
	// too: that is how it will actually reach a log line.
	type carrier struct {
		Workspace string
		Secrets   Secrets
	}
	c := carrier{Workspace: "forge", Secrets: store}
	renderings["struct %v"] = fmt.Sprintf("%v", c)
	renderings["struct %+v"] = fmt.Sprintf("%+v", c)

	encoded, err := json.Marshal(store)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	renderings["json"] = string(encoded)

	for name, out := range renderings {
		if strings.Contains(out, testPassphrase) {
			t.Errorf("%s leaked a passphrase: %s", name, out)
		}
		if !strings.Contains(out, redactedPlaceholder) && name != "json" {
			t.Errorf("%s = %s, want a redaction marker", name, out)
		}
	}
}

func TestAPITokenRoundTripAndPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits")
	}
	base := t.TempDir()
	dir := filepath.Join(base, "irc", "01JWORKSPACE")
	path := filepath.Join(dir, "api.token")

	const token = "bearer-token-value"
	if err := SaveAPIToken(path, token); err != nil {
		t.Fatalf("SaveAPIToken: %v", err)
	}

	got, err := LoadAPIToken(path)
	if err != nil {
		t.Fatalf("LoadAPIToken: %v", err)
	}
	if got != token {
		t.Errorf("token = %q, want %q", got, token)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat api token: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("api.token mode = %#o, want 0600", perm)
	}
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat api token dir: %v", err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("api token dir mode = %#o, want 0700", perm)
	}

	t.Run("no temp file left behind", func(t *testing.T) {
		if err := SaveAPIToken(path, "rotated-token"); err != nil {
			t.Fatalf("SaveAPIToken (rotate): %v", err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir: %v", err)
		}
		if len(entries) != 1 || entries[0].Name() != "api.token" {
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("directory contains %v, want only api.token", names)
		}
	})
}

func TestLoadAPITokenMissingAndEmpty(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadAPIToken(filepath.Join(dir, "absent.token"))
	if !errors.Is(err, ErrNoAPIToken) {
		t.Errorf("missing token error = %v, want ErrNoAPIToken", err)
	}

	empty := filepath.Join(dir, "empty.token")
	if err := os.WriteFile(empty, []byte("   \n"), secretFileMode); err != nil {
		t.Fatalf("write empty token: %v", err)
	}
	if _, err := LoadAPIToken(empty); !errors.Is(err, ErrNoAPIToken) {
		t.Errorf("empty token error = %v, want ErrNoAPIToken", err)
	}
}

func TestSaveAPITokenRejectsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.token")
	if err := SaveAPIToken(path, "  "); err == nil {
		t.Error("SaveAPIToken with a blank token returned no error")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Error("SaveAPIToken with a blank token created a file")
	}
}

func TestReadBotPasswordPrecedence(t *testing.T) {
	t.Run("secret_ref wins over secrets.yaml", func(t *testing.T) {
		writeFakeAPS(t, map[string]string{"irc/bot": "from-aps"})

		cfg := Defaults()
		cfg.Bot = Bot{Account: "wsm", SecretRef: "irc/bot"}
		store := Secrets{"wsm": testPassphrase}

		got, err := ReadBotPassword(cfg, store)
		if err != nil {
			t.Fatalf("ReadBotPassword: %v", err)
		}
		if got != "from-aps" {
			t.Errorf("password = %q, want the aps value (secrets.yaml must not win)", got)
		}
	})

	t.Run("falls back to secrets.yaml when no ref is set", func(t *testing.T) {
		cfg := Defaults()
		cfg.Bot = Bot{Account: "wsm"}
		store := Secrets{"wsm": testPassphrase}

		got, err := ReadBotPassword(cfg, store)
		if err != nil {
			t.Fatalf("ReadBotPassword: %v", err)
		}
		if got != testPassphrase {
			t.Errorf("password = %q, want the secrets.yaml value", got)
		}
	})

	t.Run("unresolvable ref errors instead of falling back", func(t *testing.T) {
		writeFakeAPS(t, map[string]string{"irc/other": "unrelated"})

		cfg := Defaults()
		cfg.Bot = Bot{Account: "wsm", SecretRef: "irc/missing"}
		store := Secrets{"wsm": testPassphrase}

		got, err := ReadBotPassword(cfg, store)
		if err == nil {
			t.Fatalf("ReadBotPassword returned %q, want an error", got)
		}
		// Falling back here would authenticate with a password the operator
		// already rotated away in aps.
		if got == testPassphrase {
			t.Error("ReadBotPassword fell back to secrets.yaml after a failed ref")
		}
		if !strings.Contains(err.Error(), "irc/missing") {
			t.Errorf("error %v does not name the unresolved ref", err)
		}
	})

	t.Run("missing everywhere is ErrNoBotSecret", func(t *testing.T) {
		cfg := Defaults()
		cfg.Bot = Bot{Account: "wsm"}

		_, err := ReadBotPassword(cfg, Secrets{})
		if !errors.Is(err, ErrNoBotSecret) {
			t.Errorf("error = %v, want ErrNoBotSecret", err)
		}

		_, err = ReadBotPassword(cfg, nil)
		if !errors.Is(err, ErrNoBotSecret) {
			t.Errorf("error on a nil store = %v, want ErrNoBotSecret", err)
		}
	})

	t.Run("aps errors do not leak the secrets.yaml value", func(t *testing.T) {
		writeFakeAPS(t, nil)

		cfg := Defaults()
		cfg.Bot = Bot{Account: "wsm", SecretRef: "irc/bot"}
		store := Secrets{"wsm": testPassphrase}

		_, err := ReadBotPassword(cfg, store)
		if err == nil {
			t.Fatal("want an error from the fake aps")
		}
		if strings.Contains(err.Error(), testPassphrase) {
			t.Errorf("error leaked the held passphrase: %v", err)
		}
	})
}

func TestReadAPITokenPrecedence(t *testing.T) {
	// newPaths builds a Paths whose token file is populated with value.
	newPaths := func(t *testing.T, value string) Paths {
		t.Helper()
		p, err := ResolvePaths(PathsInput{DataDir: filepath.Join(t.TempDir(), "data"), StateDir: filepath.Join(t.TempDir(), "state")}, "01JWORKSPACE")
		if err != nil {
			t.Fatalf("ResolvePaths: %v", err)
		}
		if value != "" {
			if err := SaveAPIToken(p.APIToken, value); err != nil {
				t.Fatalf("SaveAPIToken: %v", err)
			}
		}
		return p
	}

	t.Run("host token wins and is read in place", func(t *testing.T) {
		writeFakeAPS(t, map[string]string{"irc/api": "from-aps"})

		guest := newPaths(t, "guest-own-token")
		host := newPaths(t, "host-token")

		cfg := Defaults()
		cfg.Server.ErgoAPITokenRef = "irc/api"

		got, err := ReadAPIToken(cfg, guest, &host)
		if err != nil {
			t.Fatalf("ReadAPIToken: %v", err)
		}
		if got != "host-token" {
			t.Errorf("token = %q, want the host's token", got)
		}

		// The host's token must never be copied into the guest's data dir:
		// the guest's own file is untouched by the read.
		guestToken, err := LoadAPIToken(guest.APIToken)
		if err != nil {
			t.Fatalf("LoadAPIToken(guest): %v", err)
		}
		if guestToken != "guest-own-token" {
			t.Errorf("guest token = %q, want it left untouched", guestToken)
		}
	})

	t.Run("token ref beats the local file", func(t *testing.T) {
		writeFakeAPS(t, map[string]string{"irc/api": "from-aps"})

		paths := newPaths(t, "local-token")
		cfg := Defaults()
		cfg.Server.ErgoAPITokenRef = "irc/api"

		got, err := ReadAPIToken(cfg, paths, nil)
		if err != nil {
			t.Fatalf("ReadAPIToken: %v", err)
		}
		if got != "from-aps" {
			t.Errorf("token = %q, want the aps value", got)
		}
	})

	t.Run("falls back to the local token file", func(t *testing.T) {
		paths := newPaths(t, "local-token")
		cfg := Defaults()

		got, err := ReadAPIToken(cfg, paths, nil)
		if err != nil {
			t.Fatalf("ReadAPIToken: %v", err)
		}
		if got != "local-token" {
			t.Errorf("token = %q, want the local token", got)
		}
	})

	t.Run("nothing anywhere is ErrNoAPIToken", func(t *testing.T) {
		paths := newPaths(t, "")
		cfg := Defaults()

		if _, err := ReadAPIToken(cfg, paths, nil); !errors.Is(err, ErrNoAPIToken) {
			t.Errorf("error = %v, want ErrNoAPIToken", err)
		}
	})

	t.Run("host without a token path is ErrNoAPIToken", func(t *testing.T) {
		paths := newPaths(t, "local-token")
		cfg := Defaults()
		empty := Paths{}

		if _, err := ReadAPIToken(cfg, paths, &empty); !errors.Is(err, ErrNoAPIToken) {
			t.Errorf("error = %v, want ErrNoAPIToken", err)
		}
	})
}

func TestAPSBinOverrideIsHonoured(t *testing.T) {
	// With APS_BIN pointing at a script that always fails, resolution must
	// fail rather than silently finding a real aps on PATH.
	dir := t.TempDir()
	failing := filepath.Join(dir, "aps-fail")
	if err := os.WriteFile(failing, []byte("#!/bin/sh\necho 'refused' >&2\nexit 3\n"), 0o700); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv(EnvAPSBin, failing)

	cfg := Defaults()
	cfg.Bot = Bot{Account: "wsm", SecretRef: "irc/bot"}

	_, err := ReadBotPassword(cfg, Secrets{})
	if err == nil {
		t.Fatal("want an error from the overridden aps binary")
	}
	if !strings.Contains(err.Error(), "refused") {
		t.Errorf("error = %v, want it to carry the helper's stderr", err)
	}
}
