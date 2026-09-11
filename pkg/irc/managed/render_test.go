package managed

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"hop.top/wsm/pkg/irc"
)

// updateGolden rewrites the testdata files instead of comparing against them.
// Run with `go test ./pkg/irc/managed -update` after a deliberate change to
// the rendered document, and review the diff: these files are the record of
// what wsm actually hands ergo.
var updateGolden = flag.Bool("update", false, "rewrite golden files")

// bcryptHash matches a bcrypt digest as ergo's genpasswd emits one. The cost
// and salt vary per render, so golden comparison replaces the digest with a
// placeholder and the hash itself is asserted separately.
var bcryptHash = regexp.MustCompile(`\$2[aby]\$\d{2}\$[./A-Za-z0-9]{53}`)

// renderFixture builds a workspace whose paths, API token and languages
// directory all exist, so RenderConfig exercises its real lookups rather than
// being handed pre-baked strings.
func renderFixture(t *testing.T) (irc.Paths, string) {
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
	if err := irc.SaveAPIToken(paths.APIToken, "MLu7Ua8dg5DVHe8QE6_U0n0kfHpqxldZ8jtyk90q9xE"); err != nil {
		t.Fatalf("save api token: %v", err)
	}

	languages := filepath.Join(dir, "languages")
	if err := os.MkdirAll(languages, 0o755); err != nil {
		t.Fatalf("make languages dir: %v", err)
	}
	t.Setenv(EnvLanguagesPath, languages)

	return paths, languages
}

// stableRender renders and then replaces every environment-specific value
// (temp dir paths, the per-run bcrypt salt) with a stable placeholder, so the
// golden file records structure and policy rather than one machine's layout.
func stableRender(t *testing.T, cfg irc.Config, p irc.Profile, s irc.Secrets) (string, string) {
	t.Helper()

	paths, languages := renderFixture(t)

	out, err := Renderer{Paths: paths}.RenderConfig(cfg, p, s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	got := string(out)
	hash := bcryptHash.FindString(got)
	if hash == "" {
		t.Fatalf("rendered config carries no bcrypt hash:\n%s", got)
	}

	normalized := got
	normalized = strings.ReplaceAll(normalized, paths.DataDir, "/DATA")
	normalized = strings.ReplaceAll(normalized, paths.StateDir, "/STATE")
	normalized = strings.ReplaceAll(normalized, languages, "/LANGUAGES")
	// Literal replacement: ReplaceAllString would read "$B" as a capture-group
	// reference and expand it to empty, silently erasing the hash from the
	// golden file and hiding whether one was rendered at all.
	normalized = bcryptHash.ReplaceAllLiteralString(normalized, "BCRYPT-HASH")

	return normalized, hash
}

// assertGolden compares a normalized render against testdata, or rewrites it
// under -update.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("make testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create it)", path, err)
	}
	if got != string(want) {
		t.Errorf("rendered config differs from %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

// baseConfig is a managed config with the fields RenderConfig reads set to
// values a real enable would produce.
func baseConfig() irc.Config {
	cfg := irc.Defaults()
	cfg.Mode = irc.ModeManaged
	cfg.Provider = irc.ProviderErgo
	cfg.Bot.Account = "wsm"
	cfg.ServerName = "wsm.local"
	return cfg
}

// baseProfile carries an allocated port triple, which RenderConfig requires.
func baseProfile() irc.Profile {
	return irc.Profile{
		Mode:       irc.ModeManaged,
		Provider:   irc.ProviderErgo,
		BotAccount: "wsm",
		Ports:      irc.Ports{Plain: 6700, Remote: 6701, API: 6702},
	}
}

func TestRenderConfigMinimal(t *testing.T) {
	got, _ := stableRender(t, baseConfig(), baseProfile(), irc.Secrets{"wsm": "correct-horse-battery-staple"})
	assertGolden(t, "minimal.yaml", got)
}

func TestRenderConfigRemote(t *testing.T) {
	cfg := baseConfig()
	cfg.Remote.Enabled = true
	cfg.Remote.Host = "chat.example.com"
	cfg.Remote.AllowedOrigins = []string{"https://chat.example.com", "https://gamja.example.com"}

	p := baseProfile()
	p.Remote = true

	got, _ := stableRender(t, cfg, p, irc.Secrets{"wsm": "correct-horse-battery-staple"})
	assertGolden(t, "remote.yaml", got)
}

func TestRenderConfigCustomChannels(t *testing.T) {
	cfg := baseConfig()
	cfg.ServerName = "acme.internal"
	cfg.Bind = "10.0.0.5"
	cfg.Channels = []irc.ChannelTemplate{
		{Name: "#{{ws}}", Members: irc.MembersAll, Modes: "+ntRi", Topic: "{{ws}} — {{description}}"},
		{Name: "#{{ws}}-ops", Members: irc.MembersPrincipals, Modes: "+ntRi"},
		{Name: "#{{ws}}-alerts", Members: irc.MembersPrincipals, Modes: "+ntRim"},
	}

	got, _ := stableRender(t, cfg, baseProfile(), irc.Secrets{"wsm": "correct-horse-battery-staple"})
	assertGolden(t, "custom-channels.yaml", got)
}

// TestRenderConfigOperPasswordLandsOnRealEntry is a deliberate regression
// guard, not a formality.
//
// ergo's shipped default.yaml carries a commented `#password:` example roughly
// six hundred lines above the live `opers.admin` block. Any implementation
// that provisions by substituting into that upstream file by first match edits
// the comment and leaves the real oper entry without a password: ergo then
// starts fine, the bot's OPER silently fails, and every channel-registration
// and account operation degrades at runtime rather than at render time.
//
// This test pins the property that makes that impossible: the hash must be
// reachable by parsing the YAML and walking to opers.<bot>.password, and the
// document must contain no commented password line that could have absorbed
// the write instead.
func TestRenderConfigOperPasswordLandsOnRealEntry(t *testing.T) {
	const secret = "correct-horse-battery-staple"

	paths, _ := renderFixture(t)
	cfg := baseConfig()

	out, err := Renderer{Paths: paths}.RenderConfig(cfg, baseProfile(), irc.Secrets{"wsm": secret})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// Walk the parsed document, so a hash that only appears in a comment or
	// in some unrelated key cannot satisfy this test.
	var doc struct {
		Opers map[string]struct {
			Class    string `yaml:"class"`
			Password string `yaml:"password"`
			Auto     bool   `yaml:"auto"`
		} `yaml:"opers"`
		OperClasses map[string]struct {
			Capabilities []string `yaml:"capabilities"`
		} `yaml:"oper-classes"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("rendered config does not parse: %v\n%s", err, out)
	}

	entry, ok := doc.Opers["wsm"]
	if !ok {
		t.Fatalf("no opers entry for the bot account; got %v", doc.Opers)
	}
	if entry.Password == "" {
		t.Fatal("the real opers entry has an empty password: a substitution edited something else")
	}
	if !bcryptHash.MatchString(entry.Password) {
		t.Errorf("opers password is not a bcrypt hash: %q", entry.Password)
	}
	if entry.Class != OperClass {
		t.Errorf("oper class = %q, want %q", entry.Class, OperClass)
	}
	if !entry.Auto {
		t.Error("oper entry should auto-oper on a SASL-authenticated account")
	}

	// The trap itself: no commented password line may exist anywhere in the
	// document, because such a line is exactly what a first-match edit hits.
	for i, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, "password:") {
			t.Errorf("line %d is a commented password example, which a naive substitution would edit: %q", i+1, trimmed)
		}
	}

	// And the hash must verify against the bot's actual secret, so a render
	// that hashed the wrong value fails here rather than at OPER time.
	if err := bcryptCompare(entry.Password, secret); err != nil {
		t.Errorf("oper hash does not verify against the bot secret: %v", err)
	}

	class, ok := doc.OperClasses[OperClass]
	if !ok {
		t.Fatalf("no oper-classes entry named %q", OperClass)
	}
	if strings.Join(class.Capabilities, " ") != strings.Join(OperCaps, " ") {
		t.Errorf("oper capabilities = %v, want %v", class.Capabilities, OperCaps)
	}
}

// TestRenderConfigFixedPolicy pins the security posture the design fixes, so a
// future edit to the template cannot quietly open registration or move the API
// off loopback.
func TestRenderConfigFixedPolicy(t *testing.T) {
	paths, languages := renderFixture(t)

	cfg := baseConfig()
	out, err := Renderer{Paths: paths}.RenderConfig(cfg, baseProfile(), irc.Secrets{"wsm": "s3cret"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("parse: %v", err)
	}

	accounts, _ := doc["accounts"].(map[string]any)
	registration, _ := accounts["registration"].(map[string]any)
	if registration["enabled"] != false {
		t.Error("account registration must be disabled")
	}
	requireSASL, _ := accounts["require-sasl"].(map[string]any)
	if requireSASL["enabled"] != true {
		t.Error("require-sasl must be enabled")
	}
	nickReservation, _ := accounts["nick-reservation"].(map[string]any)
	if nickReservation["method"] != "strict" {
		t.Errorf("nick reservation method = %v, want strict", nickReservation["method"])
	}
	multiclient, _ := accounts["multiclient"].(map[string]any)
	if multiclient["always-on"] != "opt-in" {
		t.Errorf("multiclient always-on = %v, want opt-in", multiclient["always-on"])
	}

	channels, _ := doc["channels"].(map[string]any)
	if channels["default-modes"] != "+ntR" {
		t.Errorf("default-modes = %v, want +ntR", channels["default-modes"])
	}
	chanRegistration, _ := channels["registration"].(map[string]any)
	if chanRegistration["enabled"] != true {
		t.Error("channel registration must be enabled")
	}

	history, _ := doc["history"].(map[string]any)
	if history["chathistory-maxmessages"] != DefaultHistoryMessages {
		t.Errorf("chathistory-maxmessages = %v, want %d", history["chathistory-maxmessages"], DefaultHistoryMessages)
	}
	persistent, _ := history["persistent"].(map[string]any)
	if persistent["enabled"] != false {
		t.Error("history must stay in memory")
	}

	api, _ := doc["api"].(map[string]any)
	if api["enabled"] != true {
		t.Error("api must be enabled")
	}
	listener, _ := api["listener"].(string)
	if !strings.HasPrefix(listener, "127.0.0.1:") {
		t.Errorf("api listener = %q, must stay on loopback", listener)
	}
	tokens, _ := api["bearer-tokens"].([]any)
	if len(tokens) != 1 || tokens[0] == "" {
		t.Errorf("api bearer-tokens = %v, want exactly one non-empty token", tokens)
	}

	server, _ := doc["server"].(map[string]any)
	if server["enforce-utf8"] != true {
		t.Error("enforce-utf8 must be on")
	}
	relaymsg, _ := server["relaymsg"].(map[string]any)
	if relaymsg["enabled"] != true || relaymsg["separators"] != "/" || relaymsg["available-to-chanops"] != true {
		t.Errorf("relaymsg = %v, want enabled with / separators available to chanops", relaymsg)
	}

	// Finding 1: an absolute languages path, or every subcommand dies with
	// "Could not load languages" from wsm's data dir.
	langs, _ := doc["languages"].(map[string]any)
	path, _ := langs["path"].(string)
	if !filepath.IsAbs(path) {
		t.Errorf("languages path %q must be absolute", path)
	}
	if path != languages {
		t.Errorf("languages path = %q, want %q", path, languages)
	}

	// The datastore likewise must be absolute: ergo resolves it from cwd.
	datastore, _ := doc["datastore"].(map[string]any)
	dbPath, _ := datastore["path"].(string)
	if !filepath.IsAbs(dbPath) {
		t.Errorf("datastore path %q must be absolute", dbPath)
	}

	logging, _ := doc["logging"].([]any)
	if len(logging) == 0 {
		t.Fatal("logging must be configured")
	}
	first, _ := logging[0].(map[string]any)
	if first["method"] != "stderr" {
		t.Errorf("logging method = %v, want stderr", first["method"])
	}
}

// TestRenderConfigRemoteListener checks that the remote listener appears only
// when it is enabled, and carries TLS plus WebSocket when it does.
func TestRenderConfigRemoteListener(t *testing.T) {
	t.Run("absent when disabled", func(t *testing.T) {
		paths, _ := renderFixture(t)
		out, err := Renderer{Paths: paths}.RenderConfig(baseConfig(), baseProfile(), irc.Secrets{"wsm": "s"})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		listeners := listenerNames(t, out)
		if len(listeners) != 1 {
			t.Fatalf("listeners = %v, want only the loopback plaintext one", listeners)
		}
		if !strings.HasPrefix(listeners[0], "127.0.0.1:") {
			t.Errorf("sole listener %q must be loopback", listeners[0])
		}
	})

	t.Run("tls and websocket when enabled", func(t *testing.T) {
		paths, _ := renderFixture(t)
		cfg := baseConfig()
		cfg.Remote.Enabled = true
		cfg.Remote.Host = "chat.example.com"

		out, err := Renderer{Paths: paths}.RenderConfig(cfg, baseProfile(), irc.Secrets{"wsm": "s"})
		if err != nil {
			t.Fatalf("render: %v", err)
		}

		var doc struct {
			Server struct {
				Listeners map[string]struct {
					TLS *struct {
						Cert string `yaml:"cert"`
						Key  string `yaml:"key"`
					} `yaml:"tls"`
					WebSocket bool `yaml:"websocket"`
				} `yaml:"listeners"`
			} `yaml:"server"`
		}
		if err := yaml.Unmarshal(out, &doc); err != nil {
			t.Fatalf("parse: %v", err)
		}

		remote, ok := doc.Server.Listeners["127.0.0.1:6701"]
		if !ok {
			t.Fatalf("no remote listener on the allocated port; got %v", doc.Server.Listeners)
		}
		if remote.TLS == nil {
			t.Fatal("remote listener must present TLS")
		}
		if !remote.WebSocket {
			t.Error("remote listener must accept WebSocket")
		}
		// Finding 2: mkcerts writes privkey.pem, not key.pem, and both paths
		// must be absolute because mkcerts resolves them against cwd.
		if !filepath.IsAbs(remote.TLS.Cert) || !filepath.IsAbs(remote.TLS.Key) {
			t.Errorf("TLS paths must be absolute: cert=%q key=%q", remote.TLS.Cert, remote.TLS.Key)
		}
	})
}

// listenerNames returns the configured listener addresses.
func listenerNames(t *testing.T, out []byte) []string {
	t.Helper()

	var doc struct {
		Server struct {
			Listeners map[string]any `yaml:"listeners"`
		} `yaml:"server"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("parse: %v", err)
	}
	names := make([]string, 0, len(doc.Server.Listeners))
	for name := range doc.Server.Listeners {
		names = append(names, name)
	}
	return names
}

func TestRenderConfigRejectsUnallocatedPorts(t *testing.T) {
	paths, _ := renderFixture(t)

	_, err := Renderer{Paths: paths}.RenderConfig(baseConfig(), irc.Profile{}, irc.Secrets{"wsm": "s"})
	if err == nil {
		t.Fatal("rendering without an allocated port triple must fail")
	}
	if !strings.Contains(err.Error(), "port triple") {
		t.Errorf("error should name the missing allocation, got: %v", err)
	}
}

func TestRenderConfigRequiresBotSecret(t *testing.T) {
	paths, _ := renderFixture(t)

	_, err := Renderer{Paths: paths}.RenderConfig(baseConfig(), baseProfile(), irc.Secrets{})
	if err == nil {
		t.Fatal("rendering without a bot secret must fail")
	}
}

func TestRenderConfigRequiresAPIToken(t *testing.T) {
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
	languages := filepath.Join(dir, "languages")
	if err := os.MkdirAll(languages, 0o755); err != nil {
		t.Fatalf("make languages: %v", err)
	}
	t.Setenv(EnvLanguagesPath, languages)

	// No SaveAPIToken: the token has not been generated yet.
	_, err = Renderer{Paths: paths}.RenderConfig(baseConfig(), baseProfile(), irc.Secrets{"wsm": "s"})
	if err == nil {
		t.Fatal("rendering before the API token exists must fail")
	}
}

// TestRenderedConfigParsesWithErgo runs the real binary's own config check
// against a freshly rendered document. Skipped unless ERGO_BIN is set, so the
// unit suite never needs a server.
func TestRenderedConfigParsesWithErgo(t *testing.T) {
	bin := os.Getenv("ERGO_BIN")
	if bin == "" {
		t.Skip("ERGO_BIN not set; skipping the real ergo config parse check")
	}

	paths, _ := renderFixture(t)

	cfg := baseConfig()
	cfg.Binary = bin

	out, err := Renderer{Paths: paths}.RenderConfig(cfg, baseProfile(), irc.Secrets{"wsm": "s3cret"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := os.WriteFile(paths.ErgoYAML, out, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// mkcerts first: the ordering probe found initdb and run both fail in a
	// directory where the certificates named by the config do not exist.
	if err := smokeConfig(bin, paths.ErgoYAML); err != nil {
		t.Fatalf("ergo rejected the rendered config: %v", err)
	}
}
