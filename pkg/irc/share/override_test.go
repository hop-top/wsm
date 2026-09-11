package share

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/wsm/pkg/irc"
)

// hostForOverride is a managed host whose loopback triple the override points
// at.
func hostForOverride() irc.Profile {
	return irc.Profile{
		Mode:     irc.ModeManaged,
		Provider: irc.ProviderErgo,
		ServerID: irc.ManagedServerID,
		Ports:    irc.Ports{Plain: 6700, Remote: 6701, API: 6702},
	}
}

// guestRoot makes a workspace root, optionally seeded with an existing config.
func guestRoot(t *testing.T, existing string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".wsm"), 0o755); err != nil {
		t.Fatalf("create .wsm: %v", err)
	}
	if existing != "" {
		if err := os.WriteFile(
			filepath.Join(root, ".wsm", "config.yaml"), []byte(existing), 0o644,
		); err != nil {
			t.Fatalf("seed config: %v", err)
		}
	}
	return root
}

// TestWriteGuestOverride_PointsAtTheHostLoopback pins what the guest's config
// contains: the host's loopback listener and API, a namespace of its own, and
// its bot account. Read back through the real loader, not by string matching,
// so the test fails if the shape is right but the keys are wrong.
func TestWriteGuestOverride_PointsAtTheHostLoopback(t *testing.T) {
	root := guestRoot(t, "")
	host := hostForOverride()

	if err := WriteGuestOverride(root, host, "Sami Labs", "wsm-sami-labs"); err != nil {
		t.Fatalf("write override: %v", err)
	}

	cfg, err := irc.LoadWorkspace("", root)
	if err != nil {
		t.Fatalf("load workspace config: %v", err)
	}

	if cfg.Mode != irc.ModeExternal {
		t.Errorf("mode = %q; want external: the guest does not own this server", cfg.Mode)
	}
	if cfg.Provider != irc.ProviderErgo {
		t.Errorf("provider = %q; want ergo", cfg.Provider)
	}
	if cfg.Server.Host != irc.DefaultBind {
		t.Errorf("server.host = %q; want the host's loopback %q",
			cfg.Server.Host, irc.DefaultBind)
	}
	if cfg.Server.Port != host.Ports.Plain {
		t.Errorf("server.port = %d; want the host's plain listener %d",
			cfg.Server.Port, host.Ports.Plain)
	}
	// TLS off is correct and load-bearing: the listener never leaves
	// loopback, and there is no certificate on it to verify.
	if cfg.Server.TLSEnabled() {
		t.Error("server.tls must be false on the host's loopback plaintext listener")
	}
	if want := "http://127.0.0.1:6702"; cfg.Server.ErgoAPI != want {
		t.Errorf("server.ergo_api = %q; want the host's loopback API %q",
			cfg.Server.ErgoAPI, want)
	}
	if cfg.ChannelPrefix != "sami-labs-" {
		t.Errorf("channel_prefix = %q; want sami-labs-", cfg.ChannelPrefix)
	}
	if cfg.Bot.Account != "wsm-sami-labs" {
		t.Errorf("bot.account = %q; want wsm-sami-labs", cfg.Bot.Account)
	}
}

// TestWriteGuestOverride_CarriesNoToken is the file-level half of the trust
// boundary: no token key, and no token reference, may appear.
func TestWriteGuestOverride_CarriesNoToken(t *testing.T) {
	root := guestRoot(t, "")
	if err := WriteGuestOverride(root, hostForOverride(), "Sami Labs", "wsm-sami-labs"); err != nil {
		t.Fatalf("write override: %v", err)
	}

	raw := readFile(t, filepath.Join(root, ".wsm", "config.yaml"))
	for _, forbidden := range []string{"token", "api_token", "ergo_api_token_ref"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("the guest override names %q; the host's token is read at "+
				"runtime from the host's own api.token, never written here:\n%s",
				forbidden, raw)
		}
	}

	cfg, err := irc.LoadWorkspace("", root)
	if err != nil {
		t.Fatalf("load workspace config: %v", err)
	}
	if cfg.Server.ErgoAPITokenRef != "" {
		t.Errorf("ergo_api_token_ref = %q; want empty", cfg.Server.ErgoAPITokenRef)
	}
}

// TestWriteGuestOverride_PreservesTheRestOfTheDocument pins that attaching a
// workspace does not cost the operator the rest of their config. A workspace
// config is hand-edited and holds far more than the irc block.
func TestWriteGuestOverride_PreservesTheRestOfTheDocument(t *testing.T) {
	const existing = `# the operator's own notes
backend: sqlite
data_dir: /home/reed/data

irc:
  enabled: true
  unmanaged_channels: report
  channels:
    - name: "#{{ws}}-dev"
      members: all
`
	root := guestRoot(t, existing)
	if err := WriteGuestOverride(root, hostForOverride(), "Sami Labs", "wsm-sami-labs"); err != nil {
		t.Fatalf("write override: %v", err)
	}

	raw := readFile(t, filepath.Join(root, ".wsm", "config.yaml"))
	for _, keep := range []string{
		"# the operator's own notes",
		"backend: sqlite",
		"data_dir: /home/reed/data",
		"#{{ws}}-dev",
	} {
		if !strings.Contains(raw, keep) {
			t.Errorf("attaching dropped %q from the config:\n%s", keep, raw)
		}
	}

	cfg, err := irc.LoadWorkspace("", root)
	if err != nil {
		t.Fatalf("load workspace config: %v", err)
	}
	if !cfg.Enabled {
		t.Error("irc.enabled was lost")
	}
	if len(cfg.Channels) != 1 || cfg.Channels[0].Name != "#{{ws}}-dev" {
		t.Errorf("the operator's channel templates were lost: %+v", cfg.Channels)
	}
	// And the attachment still landed.
	if cfg.Mode != irc.ModeExternal || cfg.ChannelPrefix != "sami-labs-" {
		t.Errorf("the override did not take: mode=%q prefix=%q",
			cfg.Mode, cfg.ChannelPrefix)
	}
}

// TestWriteGuestOverride_CreatesAConfigWhenNoneExists covers a root that
// carries only the .wsm marker: attach must create the file rather than fail.
func TestWriteGuestOverride_CreatesAConfigWhenNoneExists(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".wsm"), 0o755); err != nil {
		t.Fatalf("create .wsm: %v", err)
	}

	if err := WriteGuestOverride(root, hostForOverride(), "Sami Labs", "wsm-sami-labs"); err != nil {
		t.Fatalf("write override: %v", err)
	}
	if irc.WorkspaceConfigPath(root) == "" {
		t.Fatal("expected an override file to have been created")
	}

	cfg, err := irc.LoadWorkspace("", root)
	if err != nil {
		t.Fatalf("load workspace config: %v", err)
	}
	if cfg.Mode != irc.ModeExternal {
		t.Errorf("mode = %q; want external", cfg.Mode)
	}
}

func TestWriteGuestOverride_RefusesAHostWithoutPorts(t *testing.T) {
	root := guestRoot(t, "")
	host := hostForOverride()
	host.Ports = irc.Ports{}

	if err := WriteGuestOverride(root, host, "Sami Labs", "wsm-sami-labs"); err == nil {
		t.Fatal("a host with no allocated triple has no listener to point at")
	}
}

// TestRemoveDelegates pins the host-side half of detach: only the departing
// guest's entries go, and everyone else's stay.
func TestRemoveDelegates(t *testing.T) {
	const existing = `irc:
  enabled: true
  channels:
    - name: "#{{ws}}-dev"
      members: all
      delegates:
        - workspace: ws-guest
          bot: wsm-sami-labs
          max_role: halfop
        - workspace: ws-other
          bot: wsm-other
          max_role: voice
    - name: "#{{ws}}-ops"
      members: principals
      delegates:
        - workspace: ws-guest
          bot: wsm-sami-labs
          max_role: voice
`
	root := guestRoot(t, existing)

	if err := RemoveDelegates(root, "ws-guest", "Sami Labs"); err != nil {
		t.Fatalf("remove delegates: %v", err)
	}

	cfg, err := irc.LoadWorkspace("", root)
	if err != nil {
		t.Fatalf("load workspace config: %v", err)
	}
	if len(cfg.Channels) != 2 {
		t.Fatalf("channels = %d; want 2 (templates must survive)", len(cfg.Channels))
	}

	for _, ch := range cfg.Channels {
		for _, d := range ch.Delegates {
			if d.Workspace == "ws-guest" {
				t.Errorf("%s still delegates to the departed guest", ch.Name)
			}
		}
	}
	// The other workspace's delegation is none of this detach's business.
	var others int
	for _, ch := range cfg.Channels {
		others += len(ch.Delegates)
	}
	if others != 1 {
		t.Errorf("surviving delegates = %d; want 1 (ws-other's)", others)
	}
}

// TestRemoveDelegates_MatchesByName covers a hand-written config that names the
// workspace rather than its id.
func TestRemoveDelegates_MatchesByName(t *testing.T) {
	const existing = `irc:
  channels:
    - name: "#{{ws}}-dev"
      delegates:
        - workspace: Sami Labs
          bot: wsm-sami-labs
          max_role: voice
`
	root := guestRoot(t, existing)

	if err := RemoveDelegates(root, "ws-guest", "Sami Labs"); err != nil {
		t.Fatalf("remove delegates: %v", err)
	}
	if got := readFile(t, filepath.Join(root, ".wsm", "config.yaml")); strings.Contains(got, "Sami Labs") {
		t.Errorf("a delegate naming the guest by name survived:\n%s", got)
	}
}

func TestRemoveDelegates_NoConfigIsNotAnError(t *testing.T) {
	root := t.TempDir()
	if err := RemoveDelegates(root, "ws-guest", "Sami Labs"); err != nil {
		t.Fatalf("a host with no override has nothing to strip: %v", err)
	}
}

// TestRemoveDelegates_LeavesAnUnrelatedConfigAlone pins that a host which never
// delegated to this guest is not rewritten at all.
func TestRemoveDelegates_LeavesAnUnrelatedConfigAlone(t *testing.T) {
	const existing = `# untouched
irc:
  enabled: true
  channels:
    - name: "#{{ws}}-dev"
      members: all
`
	root := guestRoot(t, existing)
	path := filepath.Join(root, ".wsm", "config.yaml")
	before := readFile(t, path)

	if err := RemoveDelegates(root, "ws-guest", "Sami Labs"); err != nil {
		t.Fatalf("remove delegates: %v", err)
	}
	if after := readFile(t, path); after != before {
		t.Errorf("a config with no matching delegate was rewritten:\nbefore:\n%s\nafter:\n%s",
			before, after)
	}
}
