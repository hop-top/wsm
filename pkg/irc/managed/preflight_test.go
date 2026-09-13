package managed

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"hop.top/wsm/pkg/access"
	"hop.top/wsm/pkg/irc"
)

// findPrereq returns the named prerequisite, failing the test when it is
// absent: every caller here asserts on a specific check.
func findPrereq(t *testing.T, prereqs []access.Prerequisite, name string) access.Prerequisite {
	t.Helper()

	for _, p := range prereqs {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no prerequisite named %q in %v", name, prereqNames(prereqs))
	return access.Prerequisite{}
}

func prereqNames(prereqs []access.Prerequisite) []string {
	names := make([]string, 0, len(prereqs))
	for _, p := range prereqs {
		names = append(names, p.Name)
	}
	return names
}

// hasPrereq reports whether a check with this name was returned.
func hasPrereq(prereqs []access.Prerequisite, name string) bool {
	for _, p := range prereqs {
		if p.Name == name {
			return true
		}
	}
	return false
}

// managedConfig is a managed config whose binary is whatever the test put on
// PATH.
func managedConfig() irc.Config {
	cfg := irc.Defaults()
	cfg.Mode = irc.ModeManaged
	cfg.Provider = irc.ProviderErgo
	cfg.Bot.Account = "wsm"
	return cfg
}

// TestPreflightStaticAllRequired pins that every static check is required:
// the CLI maps any failure here to exit 7, and a recommended check would
// silently downgrade a genuine blocker.
func TestPreflightStaticAllRequired(t *testing.T) {
	fakeErgoOnPath(t, `echo "ergo version 2.19.1"`)

	cfg := managedConfig()
	cfg.Remote.Enabled = true
	cfg.Remote.Host = "chat.example.com"

	for _, p := range PreflightStatic(cfg) {
		if p.Level != access.LevelRequired {
			t.Errorf("%s is %s, every managed preflight check must be required", p.Name, p.Level)
		}
		if strings.TrimSpace(p.Message) == "" {
			t.Errorf("%s has no Message; the fix text is what the operator sees on exit 7", p.Name)
		}
		if p.Check == nil {
			t.Errorf("%s has no Check", p.Name)
		}
	}
}

func TestPreflightStaticBinaryFound(t *testing.T) {
	fakeErgoOnPath(t, `echo "ergo version 2.19.1"`)

	prereqs := PreflightStatic(managedConfig())
	if err := findPrereq(t, prereqs, "irc.binary").Check(); err != nil {
		t.Errorf("binary on PATH should pass: %v", err)
	}
	if err := findPrereq(t, prereqs, "irc.binary.version").Check(); err != nil {
		t.Errorf("version 2.19.1 should pass: %v", err)
	}
}

func TestPreflightStaticBinaryMissing(t *testing.T) {
	// An empty PATH and a relative binary name: nothing to find.
	t.Setenv("PATH", t.TempDir())

	previous := Binary
	Binary = ""
	t.Cleanup(func() { Binary = previous })

	prereqs := PreflightStatic(managedConfig())

	check := findPrereq(t, prereqs, "irc.binary")
	if err := check.Check(); err == nil {
		t.Error("a missing binary must fail")
	}
	if !strings.Contains(check.Message, "install") {
		t.Errorf("message should tell the operator how to install ergo, got %q", check.Message)
	}
}

func TestPreflightStaticBinaryAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "my-ergo")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho \"ergo version 2.19.1\"\n"), 0o700); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	// Nothing on PATH: an absolute irc.binary must be honoured on its own.
	t.Setenv("PATH", t.TempDir())

	cfg := managedConfig()
	cfg.Binary = bin

	prereqs := PreflightStatic(cfg)
	if err := findPrereq(t, prereqs, "irc.binary").Check(); err != nil {
		t.Errorf("an absolute binary path should pass: %v", err)
	}
	if err := findPrereq(t, prereqs, "irc.binary.version").Check(); err != nil {
		t.Errorf("version check via absolute path should pass: %v", err)
	}
}

func TestPreflightStaticBinaryNotExecutable(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "my-ergo")
	if err := os.WriteFile(bin, []byte("not executable"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg := managedConfig()
	cfg.Binary = bin

	if err := findPrereq(t, PreflightStatic(cfg), "irc.binary").Check(); err == nil {
		t.Error("a non-executable irc.binary must fail")
	}
}

func TestPreflightStaticVersionTooOld(t *testing.T) {
	fakeErgoOnPath(t, `echo "ergo version 2.18.9"`)

	check := findPrereq(t, PreflightStatic(managedConfig()), "irc.binary.version")
	err := check.Check()
	if err == nil {
		t.Fatal("ergo 2.18.9 must fail the 2.19 minimum")
	}
	if !strings.Contains(err.Error(), "2.18.9") {
		t.Errorf("error should name the version found, got: %v", err)
	}
	if !strings.Contains(check.Message, MinErgoVersion) {
		t.Errorf("message should name the required version, got %q", check.Message)
	}
}

func TestPreflightStaticVersionNewerPasses(t *testing.T) {
	for _, version := range []string{"2.19", "2.19.1", "2.20.0", "3.0.0"} {
		t.Run(version, func(t *testing.T) {
			fakeErgoOnPath(t, `echo "ergo version `+version+`"`)
			if err := findPrereq(t, PreflightStatic(managedConfig()), "irc.binary.version").Check(); err != nil {
				t.Errorf("version %s should pass: %v", version, err)
			}
		})
	}
}

func TestPreflightStaticVersionUnparseable(t *testing.T) {
	fakeErgoOnPath(t, `echo "not a version at all"`)

	if err := findPrereq(t, PreflightStatic(managedConfig()), "irc.binary.version").Check(); err == nil {
		t.Error("an unparseable version must fail rather than be assumed good")
	}
}

func TestPreflightStaticPortRange(t *testing.T) {
	fakeErgoOnPath(t, `echo "ergo version 2.19.1"`)

	t.Run("range with room passes", func(t *testing.T) {
		cfg := managedConfig()
		cfg.PortRange = irc.DefaultPortRange
		if err := findPrereq(t, PreflightStatic(cfg), "irc.port_range").Check(); err != nil {
			t.Errorf("the default range should have a free triple: %v", err)
		}
	})

	t.Run("range too narrow fails", func(t *testing.T) {
		cfg := managedConfig()
		// Two ports cannot hold a triple, so this fails by arithmetic rather
		// than by what happens to be bound on the test machine.
		cfg.PortRange = [2]int{6700, 6701}

		check := findPrereq(t, PreflightStatic(cfg), "irc.port_range")
		if err := check.Check(); err == nil {
			t.Error("a range too narrow for a triple must fail")
		}
		if !strings.Contains(check.Message, "port_range") {
			t.Errorf("message should name the setting to widen, got %q", check.Message)
		}
	})

	t.Run("fully occupied range fails", func(t *testing.T) {
		// Hold every port in a three-wide range so no triple can be found.
		var held []net.Listener
		var ports []int
		for i := 0; i < 3; i++ {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			held = append(held, ln)
			ports = append(ports, ln.Addr().(*net.TCPAddr).Port)
		}
		t.Cleanup(func() {
			for _, ln := range held {
				_ = ln.Close()
			}
		})

		// A range covering exactly one of the held ports as its base, three
		// wide, cannot yield a free triple.
		cfg := managedConfig()
		cfg.PortRange = [2]int{ports[0], ports[0] + 2}

		if err := findPrereq(t, PreflightStatic(cfg), "irc.port_range").Check(); err == nil {
			t.Error("a range whose only triple is occupied must fail")
		}
	})
}

func TestPreflightStaticRemoteHost(t *testing.T) {
	fakeErgoOnPath(t, `echo "ergo version 2.19.1"`)

	t.Run("not checked when remote is off", func(t *testing.T) {
		cfg := managedConfig()
		cfg.Remote.Enabled = false
		cfg.Remote.Host = ""

		if hasPrereq(PreflightStatic(cfg), "irc.remote.host") {
			t.Error("remote.host must not be required when the remote listener is off")
		}
	})

	t.Run("required when remote is on", func(t *testing.T) {
		cfg := managedConfig()
		cfg.Remote.Enabled = true
		cfg.Remote.Host = ""

		check := findPrereq(t, PreflightStatic(cfg), "irc.remote.host")
		if err := check.Check(); err == nil {
			t.Error("remote.enabled with an empty host must fail")
		}
		if !strings.Contains(check.Message, "irc.remote.host") {
			t.Errorf("message should name the setting, got %q", check.Message)
		}
	})

	t.Run("passes when set", func(t *testing.T) {
		cfg := managedConfig()
		cfg.Remote.Enabled = true
		cfg.Remote.Host = "chat.example.com"

		if err := findPrereq(t, PreflightStatic(cfg), "irc.remote.host").Check(); err != nil {
			t.Errorf("a set host should pass: %v", err)
		}
	})
}

// TestPreflightStaticTLSFilesOnlyWhenConfigured pins the rule the design calls
// out explicitly: empty cert paths mean "generate a self-signed pair later",
// which must never be a static failure. Only paths the operator actually
// configured are required to exist up front.
func TestPreflightStaticTLSFilesOnlyWhenConfigured(t *testing.T) {
	fakeErgoOnPath(t, `echo "ergo version 2.19.1"`)

	t.Run("empty paths are not checked", func(t *testing.T) {
		cfg := managedConfig()
		cfg.Remote.Enabled = true
		cfg.Remote.Host = "chat.example.com"
		cfg.Remote.TLSCert = ""
		cfg.Remote.TLSKey = ""

		prereqs := PreflightStatic(cfg)
		if hasPrereq(prereqs, "irc.remote.tls_cert") || hasPrereq(prereqs, "irc.remote.tls_key") {
			t.Error("empty TLS paths mean generate-later and must not be checked statically")
		}
	})

	t.Run("configured but missing fails", func(t *testing.T) {
		cfg := managedConfig()
		cfg.Remote.Enabled = true
		cfg.Remote.Host = "chat.example.com"
		cfg.Remote.TLSCert = filepath.Join(t.TempDir(), "absent.pem")
		cfg.Remote.TLSKey = filepath.Join(t.TempDir(), "absent-key.pem")

		prereqs := PreflightStatic(cfg)
		certCheck := findPrereq(t, prereqs, "irc.remote.tls_cert")
		if err := certCheck.Check(); err == nil {
			t.Error("a configured but missing certificate must fail")
		}
		if !strings.Contains(certCheck.Message, "leave it empty") {
			t.Errorf("message should mention the generate-later option, got %q", certCheck.Message)
		}
		if err := findPrereq(t, prereqs, "irc.remote.tls_key").Check(); err == nil {
			t.Error("a configured but missing key must fail")
		}
	})

	t.Run("configured and present passes", func(t *testing.T) {
		dir := t.TempDir()
		cert := filepath.Join(dir, "fullchain.pem")
		key := filepath.Join(dir, "privkey.pem")
		for _, path := range []string{cert, key} {
			if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
		}

		cfg := managedConfig()
		cfg.Remote.Enabled = true
		cfg.Remote.Host = "chat.example.com"
		cfg.Remote.TLSCert = cert
		cfg.Remote.TLSKey = key

		prereqs := PreflightStatic(cfg)
		if err := findPrereq(t, prereqs, "irc.remote.tls_cert").Check(); err != nil {
			t.Errorf("an existing certificate should pass: %v", err)
		}
		if err := findPrereq(t, prereqs, "irc.remote.tls_key").Check(); err != nil {
			t.Errorf("an existing key should pass: %v", err)
		}
	})

	t.Run("checked even when remote is off", func(t *testing.T) {
		// A configured path that does not exist is a configuration error
		// regardless of whether the listener is currently switched on.
		cfg := managedConfig()
		cfg.Remote.Enabled = false
		cfg.Remote.TLSCert = filepath.Join(t.TempDir(), "absent.pem")

		if !hasPrereq(PreflightStatic(cfg), "irc.remote.tls_cert") {
			t.Error("a configured cert path should still be validated")
		}
	})
}

// readyFixture builds the paths, token and config a ready-stage check needs,
// with the API served by a local test server on the allocated API port.
func readyFixture(t *testing.T) (irc.Config, irc.Profile, irc.Paths) {
	t.Helper()

	dir := t.TempDir()
	paths, err := irc.ResolvePaths(irc.PathsInput{
		DataDir:  filepath.Join(dir, "data"),
		StateDir: filepath.Join(dir, "state"),
	}, "01HTESTWORKSPACEID")
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}

	cfg := managedConfig()
	p := irc.Profile{
		Mode:     irc.ModeManaged,
		Provider: irc.ProviderErgo,
		Ports:    irc.Ports{Plain: 6700, Remote: 6701, API: 6702},
	}
	return cfg, p, paths
}

func TestPreflightReadyAllRequired(t *testing.T) {
	cfg, p, paths := readyFixture(t)

	for _, check := range PreflightReady(cfg, p, paths) {
		if check.Level != access.LevelRequired {
			t.Errorf("%s is %s, every managed readiness check must be required", check.Name, check.Level)
		}
		if strings.TrimSpace(check.Message) == "" {
			t.Errorf("%s has no Message", check.Name)
		}
	}
}

func TestPreflightReadyPortsFree(t *testing.T) {
	cfg, p, paths := readyFixture(t)

	t.Run("unallocated triple fails", func(t *testing.T) {
		if err := findPrereq(t, PreflightReady(cfg, irc.Profile{}, paths), "irc.ports.free").Check(); err == nil {
			t.Error("a profile with no allocated ports must fail")
		}
	})

	t.Run("occupied port fails", func(t *testing.T) {
		// Bind the middle port of a real triple, then assert the check sees it.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		defer func() { _ = ln.Close() }()

		port := ln.Addr().(*net.TCPAddr).Port
		occupied := irc.Profile{Ports: irc.Ports{Plain: port - 1, Remote: port, API: port + 1}}

		check := findPrereq(t, PreflightReady(cfg, occupied, paths), "irc.ports.free")
		if err := check.Check(); err == nil {
			t.Error("an occupied allocated port must fail")
		}
		if !strings.Contains(check.Message, "re-enable") {
			t.Errorf("message should offer the fix, got %q", check.Message)
		}
	})

	_ = p
}

func TestPreflightReadyTLSOnlyWhenRemote(t *testing.T) {
	cfg, p, paths := readyFixture(t)

	t.Run("not checked when remote is off", func(t *testing.T) {
		cfg.Remote.Enabled = false
		prereqs := PreflightReady(cfg, p, paths)
		if hasPrereq(prereqs, "irc.tls.cert") || hasPrereq(prereqs, "irc.tls.key") {
			t.Error("the loopback plaintext listener needs no certificate")
		}
	})

	t.Run("generated files must exist when remote is on", func(t *testing.T) {
		cfg.Remote.Enabled = true
		cfg.Remote.Host = "chat.example.com"

		prereqs := PreflightReady(cfg, p, paths)
		certCheck := findPrereq(t, prereqs, "irc.tls.cert")
		if err := certCheck.Check(); err == nil {
			t.Error("a missing generated certificate must fail at readiness")
		}
		if !strings.Contains(certCheck.Message, "mkcerts") {
			t.Errorf("message should point at mkcerts, got %q", certCheck.Message)
		}

		// Write what mkcerts would have produced; the check then passes.
		if err := os.WriteFile(paths.TLSCertPath(), []byte("cert"), 0o644); err != nil {
			t.Fatalf("write cert: %v", err)
		}
		if err := os.WriteFile(paths.TLSKeyPath(), []byte("key"), 0o600); err != nil {
			t.Fatalf("write key: %v", err)
		}

		prereqs = PreflightReady(cfg, p, paths)
		if err := findPrereq(t, prereqs, "irc.tls.cert").Check(); err != nil {
			t.Errorf("generated certificate should pass: %v", err)
		}
		if err := findPrereq(t, prereqs, "irc.tls.key").Check(); err != nil {
			t.Errorf("generated key should pass: %v", err)
		}
	})
}

func TestPreflightReadyConfigParses(t *testing.T) {
	cfg, p, paths := readyFixture(t)

	t.Run("missing config fails", func(t *testing.T) {
		fakeErgoOnPath(t, `exit 0`)
		if err := findPrereq(t, PreflightReady(cfg, p, paths), "irc.config.parses").Check(); err == nil {
			t.Error("a config that has not been rendered must fail")
		}
	})

	if err := os.WriteFile(paths.ErgoYAML, []byte("network:\n  name: test\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Run("smoke success passes", func(t *testing.T) {
		fakeErgoOnPath(t, `echo "$@" | grep -q -- --smoke || exit 3; exit 0`)
		if err := findPrereq(t, PreflightReady(cfg, p, paths), "irc.config.parses").Check(); err != nil {
			t.Errorf("a config ergo accepts should pass: %v", err)
		}
	})

	t.Run("smoke failure folds stderr", func(t *testing.T) {
		fakeErgoOnPath(t, `echo "yaml: line 3: mapping values are not allowed" >&2; exit 1`)
		err := findPrereq(t, PreflightReady(cfg, p, paths), "irc.config.parses").Check()
		if err == nil {
			t.Fatal("a config ergo rejects must fail")
		}
		if !strings.Contains(err.Error(), "mapping values") {
			t.Errorf("ergo's own complaint should reach the error, got: %v", err)
		}
	})
}

// TestPreflightReadyAPIProbe is the check that exists because of the
// soft-failure finding: ergo logs an api.listener error and keeps running
// without the API, and `run --smoke` still exits 0. Readiness therefore has to
// ask the API itself rather than infer it from a clean startup.
func TestPreflightReadyAPIProbe(t *testing.T) {
	newFixture := func(t *testing.T) (irc.Config, irc.Profile, irc.Paths, *httptest.Server) {
		t.Helper()

		cfg, _, paths := readyFixture(t)
		if err := irc.SaveAPIToken(paths.APIToken, "tok-123"); err != nil {
			t.Fatalf("save token: %v", err)
		}
		return cfg, irc.Profile{}, paths, nil
	}

	t.Run("answers 200", func(t *testing.T) {
		cfg, _, paths, _ := newFixture(t)

		var gotAuth, gotMethod, gotPath string
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotMethod = r.Method
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"version":"2.19.1"}`))
		}))
		srv.Start()
		defer srv.Close()

		p := irc.Profile{Ports: portsForServer(t, srv)}

		if err := findPrereq(t, PreflightReady(cfg, p, paths), "irc.api.ready").Check(); err != nil {
			t.Fatalf("a responding API should pass: %v", err)
		}
		// Every ergo API endpoint is POST and bearer-authenticated.
		if gotMethod != http.MethodPost {
			t.Errorf("method = %s, want POST", gotMethod)
		}
		if gotPath != "/v1/status" {
			t.Errorf("path = %s, want /v1/status", gotPath)
		}
		if gotAuth != "Bearer tok-123" {
			t.Errorf("authorization = %q, want the workspace bearer token", gotAuth)
		}
	})

	t.Run("401 fails with the plain-text body", func(t *testing.T) {
		cfg, _, paths, _ := newFixture(t)

		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Auth failures are plain text, not JSON.
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Unauthorized"))
		}))
		srv.Start()
		defer srv.Close()

		p := irc.Profile{Ports: portsForServer(t, srv)}

		err := findPrereq(t, PreflightReady(cfg, p, paths), "irc.api.ready").Check()
		if err == nil {
			t.Fatal("a 401 must fail readiness: the running server has a different token")
		}
		if !strings.Contains(err.Error(), "Unauthorized") {
			t.Errorf("the plain-text body should reach the error, got: %v", err)
		}
	})

	t.Run("nothing listening fails", func(t *testing.T) {
		cfg, _, paths, _ := newFixture(t)

		// Take a port and release it, so nothing is listening there.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()

		p := irc.Profile{Ports: irc.Ports{Plain: port - 2, Remote: port - 1, API: port}}

		check := findPrereq(t, PreflightReady(cfg, p, paths), "irc.api.ready")
		if err := check.Check(); err == nil {
			t.Fatal("an absent API must fail even though ergo itself may be running")
		}
		// The message must explain the soft failure, or the operator has no
		// way to know why a running server has no API.
		if !strings.Contains(check.Message, "keeps running") {
			t.Errorf("message should explain ergo's soft API failure, got %q", check.Message)
		}
		if !strings.Contains(check.Message, paths.ErgoLog) {
			t.Errorf("message should point at the ergo log, got %q", check.Message)
		}
	})

	t.Run("missing token fails", func(t *testing.T) {
		cfg, _, paths := readyFixture(t)
		// No SaveAPIToken at all.
		p := irc.Profile{Ports: irc.Ports{Plain: 6700, Remote: 6701, API: 6702}}

		if err := findPrereq(t, PreflightReady(cfg, p, paths), "irc.api.ready").Check(); err == nil {
			t.Error("no API token must fail readiness")
		}
	})
}

// portsForServer turns a test server's address into a port triple whose API
// port is the one the server listens on.
func portsForServer(t *testing.T, srv *httptest.Server) irc.Ports {
	t.Helper()

	addr := strings.TrimPrefix(srv.URL, "http://")
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return irc.Ports{Plain: port - 2, Remote: port - 1, API: port}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.19.1", "2.19", 1},
		{"2.19", "2.19", 0},
		{"2.19.0", "2.19", 0},
		{"2.18.9", "2.19", -1},
		{"3.0", "2.19", 1},
		{"2.20", "2.19", 1},
		{"2.9", "2.19", -1},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s_vs_%s", c.a, c.b), func(t *testing.T) {
			if got := compareVersions(c.a, c.b); got != c.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

// TestLanguagesPathAbsolute covers finding 1 at the level of the helper: the
// rendered config can never carry ergo's shipped relative "languages" path.
func TestLanguagesPathAbsolute(t *testing.T) {
	t.Run("override wins", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv(EnvLanguagesPath, dir)

		got, err := LanguagesPath("ergo")
		if err != nil {
			t.Fatalf("LanguagesPath: %v", err)
		}
		if got != dir {
			t.Errorf("path = %q, want the override %q", got, dir)
		}
	})

	t.Run("found beside the binary", func(t *testing.T) {
		t.Setenv(EnvLanguagesPath, "")

		dir := fakeErgoOnPath(t, `exit 0`)
		languages := filepath.Join(dir, "languages")
		if err := os.MkdirAll(languages, 0o755); err != nil {
			t.Fatalf("make languages: %v", err)
		}

		got, err := LanguagesPath("ergo")
		if err != nil {
			t.Fatalf("LanguagesPath: %v", err)
		}
		if got != languages {
			t.Errorf("path = %q, want %q", got, languages)
		}
		if !filepath.IsAbs(got) {
			t.Errorf("path %q must be absolute", got)
		}
	})

	t.Run("reports the fix when absent", func(t *testing.T) {
		t.Setenv(EnvLanguagesPath, "")
		fakeErgoOnPath(t, `exit 0`)

		_, err := LanguagesPath("ergo")
		if err == nil {
			t.Fatal("no languages directory anywhere must be an error, never a relative fallback")
		}
		if !strings.Contains(err.Error(), EnvLanguagesPath) {
			t.Errorf("error should name the override variable, got: %v", err)
		}
	})
}
