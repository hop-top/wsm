package irc

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile writes content to path, creating parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearIRCEnv(t)

	// An absent path yields defaults: a conventional location is not a user
	// assertion that the file exists.
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Mode != ModeManaged || cfg.Provider != ProviderErgo {
		t.Errorf("expected defaults, got %+v", cfg)
	}
}

func TestLoad_File(t *testing.T) {
	clearIRCEnv(t)

	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, `
irc:
  enabled: true
  mode: external
  provider: plain
  server:
    host: irc.example.com
    port: 6667
    tls: false
  channels:
    - name: "#{{ws}}"
      members: principals
      modes: "+nt"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if !cfg.Enabled {
		t.Error("enabled not read from file")
	}
	if cfg.Mode != ModeExternal {
		t.Errorf("mode = %q, want external", cfg.Mode)
	}
	if cfg.Provider != ProviderPlain {
		t.Errorf("provider = %q, want plain", cfg.Provider)
	}
	if cfg.Server.Host != "irc.example.com" || cfg.Server.Port != 6667 {
		t.Errorf("server = %+v, want the file values", cfg.Server)
	}
	if cfg.Server.TLSEnabled() {
		t.Error("tls: false in the file must disable TLS")
	}
	if len(cfg.Channels) != 1 || cfg.Channels[0].Members != MembersPrincipals {
		t.Errorf("channels = %+v, want the file list", cfg.Channels)
	}

	// Fields absent from the file keep their defaults.
	if cfg.Binary != DefaultBinary {
		t.Errorf("binary = %q, want the default", cfg.Binary)
	}
	if cfg.Bot.Account != DefaultBotAccount {
		t.Errorf("bot.account = %q, want the default", cfg.Bot.Account)
	}
}

func TestLoad_MemberListKey(t *testing.T) {
	clearIRCEnv(t)

	// The yaml key is member_list, per the documented schema.
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, `
irc:
  channels:
    - name: "#core"
      members: list
      member_list: [reed, noor]
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Channels) != 1 {
		t.Fatalf("channels = %+v", cfg.Channels)
	}
	got := cfg.Channels[0]
	if got.Members != MembersList {
		t.Errorf("members = %q, want list", got.Members)
	}
	if len(got.MemberList) != 2 || got.MemberList[0] != "reed" {
		t.Errorf("member_list = %v, want [reed noor]", got.MemberList)
	}
}

func TestLoad_EnvBeatsFile(t *testing.T) {
	clearIRCEnv(t)

	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "irc:\n  mode: managed\n  bind: 127.0.0.1\n")

	t.Setenv(EnvMode, "external")
	t.Setenv(EnvServerHost, "env.example.com")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Mode != ModeExternal {
		t.Errorf("mode = %q, want the env value", cfg.Mode)
	}
	if cfg.Server.Host != "env.example.com" {
		t.Errorf("server.host = %q, want the env value", cfg.Server.Host)
	}
	// A file value the env does not touch survives.
	if cfg.Bind != "127.0.0.1" {
		t.Errorf("bind = %q, want the file value", cfg.Bind)
	}
}

func TestLoad_Malformed(t *testing.T) {
	clearIRCEnv(t)

	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "irc:\n  mode: [unclosed\n")

	_, err := Load(path)
	assertErr(t, err, "load irc config")
}

func TestLoad_MissingFileIsNotAnError(t *testing.T) {
	clearIRCEnv(t)

	// kit treats the conventional slots as optional.
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("a missing config file must not error: %v", err)
	}
	if cfg.Mode != ModeManaged {
		t.Errorf("expected defaults, got %+v", cfg)
	}
}

func TestWorkspaceConfigPath(t *testing.T) {
	tests := []struct {
		name    string
		create  string
		wantRel string
	}{
		{name: "legacy single file", create: ".wsm.yaml", wantRel: ".wsm.yaml"},
		{
			name:    "hidden dir",
			create:  filepath.Join(".wsm", "config.yaml"),
			wantRel: filepath.Join(".wsm", "config.yaml"),
		},
		{
			name:    "hop nested",
			create:  filepath.Join(".hop", "wsm", "config.yaml"),
			wantRel: filepath.Join(".hop", "wsm", "config.yaml"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, tt.create), "irc: {}\n")

			got := WorkspaceConfigPath(root)
			want := filepath.Join(root, tt.wantRel)
			if got != want {
				t.Errorf("WorkspaceConfigPath = %q, want %q", got, want)
			}
		})
	}

	t.Run("precedence", func(t *testing.T) {
		root := t.TempDir()
		// All three present: the legacy single file wins.
		writeFile(t, filepath.Join(root, ".wsm.yaml"), "irc: {}\n")
		writeFile(t, filepath.Join(root, ".wsm", "config.yaml"), "irc: {}\n")
		writeFile(t, filepath.Join(root, ".hop", "wsm", "config.yaml"), "irc: {}\n")

		if got, want := WorkspaceConfigPath(root), filepath.Join(root, ".wsm.yaml"); got != want {
			t.Errorf("WorkspaceConfigPath = %q, want %q", got, want)
		}
	})

	t.Run("absent", func(t *testing.T) {
		if got := WorkspaceConfigPath(t.TempDir()); got != "" {
			t.Errorf("WorkspaceConfigPath = %q, want empty", got)
		}
		if got := WorkspaceConfigPath(""); got != "" {
			t.Errorf("empty root = %q, want empty", got)
		}
	})
}

func TestLoadWorkspace(t *testing.T) {
	clearIRCEnv(t)

	globalPath := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, globalPath, `
irc:
  enabled: true
  mode: external
  provider: plain
  binary: /usr/bin/ergo
  server:
    host: global.example.com
    port: 6697
  channels:
    - name: "#{{ws}}"
    - name: "#{{ws}}-ops"
`)

	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".wsm", "config.yaml"), `
irc:
  channel_prefix: "acme-"
  server:
    host: ws.example.com
  channels:
    - name: "#only"
`)

	cfg, err := LoadWorkspace(globalPath, root)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}

	// Override wins.
	if cfg.Server.Host != "ws.example.com" {
		t.Errorf("server.host = %q, want the override", cfg.Server.Host)
	}
	if cfg.ChannelPrefix != "acme-" {
		t.Errorf("channel_prefix = %q, want the override", cfg.ChannelPrefix)
	}
	// Channels replaced, not appended.
	if len(cfg.Channels) != 1 || cfg.Channels[0].Name != "#only" {
		t.Errorf("channels = %+v, want the override list alone", cfg.Channels)
	}
	// Global survives where the override is silent.
	if cfg.Server.Port != 6697 {
		t.Errorf("server.port = %d, want the global value", cfg.Server.Port)
	}
	if cfg.Binary != "/usr/bin/ergo" {
		t.Errorf("binary = %q, want the global value", cfg.Binary)
	}
	if cfg.Provider != ProviderPlain {
		t.Errorf("provider = %q, want the global value", cfg.Provider)
	}
}

func TestLoadWorkspace_NoOverride(t *testing.T) {
	clearIRCEnv(t)

	globalPath := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, globalPath, "irc:\n  mode: external\n  server:\n    host: g.example.com\n")

	cfg, err := LoadWorkspace(globalPath, t.TempDir())
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if cfg.Server.Host != "g.example.com" {
		t.Errorf("server.host = %q, want the global value", cfg.Server.Host)
	}
}

func TestLoadWorkspace_EnvBeatsBothLayers(t *testing.T) {
	clearIRCEnv(t)

	globalPath := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, globalPath, "irc:\n  server:\n    host: global.example.com\n")

	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".wsm.yaml"),
		"irc:\n  server:\n    host: ws.example.com\n")

	t.Setenv(EnvServerHost, "env.example.com")

	cfg, err := LoadWorkspace(globalPath, root)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if cfg.Server.Host != "env.example.com" {
		t.Errorf("server.host = %q, want the env value", cfg.Server.Host)
	}
}

func TestLoadWorkspace_MalformedOverride(t *testing.T) {
	clearIRCEnv(t)

	globalPath := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, globalPath, "irc:\n  mode: managed\n")

	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".wsm.yaml"), "irc:\n  mode: [unclosed\n")

	_, err := LoadWorkspace(globalPath, root)
	assertErr(t, err, "load irc workspace override")
}

func TestLoadWorkspace_ValidatesEndToEnd(t *testing.T) {
	clearIRCEnv(t)

	// The resolved product of both layers is what Validate sees: a global
	// external config plus a workspace prefix must pass the sweeper rule.
	globalPath := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, globalPath, `
irc:
  mode: external
  unmanaged_channels: purge
  server:
    host: irc.example.com
`)

	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".wsm.yaml"), "irc:\n  channel_prefix: \"acme-\"\n")

	cfg, err := LoadWorkspace(globalPath, root)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if _, err := cfg.Validate(); err != nil {
		t.Fatalf("merged config must validate: %v", err)
	}

	// Without the prefix the same global config is invalid.
	bare, err := LoadWorkspace(globalPath, t.TempDir())
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if _, err := bare.Validate(); err == nil {
		t.Fatal("purge without a channel prefix must be rejected")
	}
}
