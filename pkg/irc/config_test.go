package irc

import (
	"strings"
	"testing"
)

// managedCfg returns a minimal valid managed config.
func managedCfg() Config {
	c := Defaults()
	c.Mode = ModeManaged
	c.Provider = ProviderErgo
	return c
}

// externalCfg returns a minimal valid external config.
func externalCfg() Config {
	c := Defaults()
	c.Mode = ModeExternal
	c.Server.Host = "irc.example.com"
	c.UnmanagedChannels = UnmanagedIgnore
	return c
}

func TestDefaults(t *testing.T) {
	c := Defaults()

	if c.Enabled {
		t.Error("irc must be off by default")
	}
	if c.Mode != ModeManaged {
		t.Errorf("mode = %q, want %q", c.Mode, ModeManaged)
	}
	if c.Provider != ProviderErgo {
		t.Errorf("provider = %q, want %q", c.Provider, ProviderErgo)
	}
	if c.PortRange != DefaultPortRange {
		t.Errorf("port_range = %v, want %v", c.PortRange, DefaultPortRange)
	}
	if !c.Server.TLSEnabled() {
		t.Error("external TLS must default on")
	}
	if c.Server.Port != DefaultExternalPort {
		t.Errorf("server.port = %d, want %d", c.Server.Port, DefaultExternalPort)
	}
	if c.Bot.Account != DefaultBotAccount {
		t.Errorf("bot.account = %q, want %q", c.Bot.Account, DefaultBotAccount)
	}
	if c.UnmanagedChannels != UnmanagedReport {
		t.Errorf("unmanaged_channels = %q, want %q", c.UnmanagedChannels, UnmanagedReport)
	}

	// Defaults alone must be a valid managed config; otherwise every fresh
	// install starts in a broken state.
	if _, err := c.Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
}

func TestValidate_ModeProviderMatrix(t *testing.T) {
	tests := []struct {
		name     string
		mode     Mode
		provider ProviderName
		wantErr  string
	}{
		{name: "managed ergo ok", mode: ModeManaged, provider: ProviderErgo},
		{
			name: "managed plain rejected", mode: ModeManaged, provider: ProviderPlain,
			wantErr: "mode managed requires provider ergo",
		},
		{name: "external ergo ok", mode: ModeExternal, provider: ProviderErgo},
		{name: "external plain ok", mode: ModeExternal, provider: ProviderPlain},
		{
			name: "unknown mode", mode: "hybrid", provider: ProviderErgo,
			wantErr: "must be managed|external",
		},
		{
			name: "unknown provider", mode: ModeManaged, provider: "inspircd",
			wantErr: "must be ergo|plain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Defaults()
			c.Mode = tt.mode
			c.Provider = tt.provider
			if tt.mode == ModeExternal {
				c.Server.Host = "irc.example.com"
				c.UnmanagedChannels = UnmanagedIgnore
			}

			_, err := c.Validate()
			assertErr(t, err, tt.wantErr)
		})
	}
}

func TestValidate_Requirements(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "external needs host",
			mutate:  func(c *Config) { *c = Defaults(); c.Mode = ModeExternal },
			wantErr: "irc.server.host: required",
		},
		{
			name: "remote needs host",
			mutate: func(c *Config) {
				*c = managedCfg()
				c.Remote.Enabled = true
			},
			wantErr: "irc.remote.host: required",
		},
		{
			name: "remote host set is ok",
			mutate: func(c *Config) {
				*c = managedCfg()
				c.Remote.Enabled = true
				c.Remote.Host = "chat.example.com"
			},
		},
		{
			name: "port range too narrow",
			mutate: func(c *Config) {
				*c = managedCfg()
				c.PortRange = [2]int{6700, 6701}
			},
			wantErr: "must span at least 3 ports",
		},
		{
			name: "port range exactly a triple is ok",
			mutate: func(c *Config) {
				*c = managedCfg()
				c.PortRange = [2]int{6700, 6702}
			},
		},
		{
			name: "port range inverted",
			mutate: func(c *Config) {
				*c = managedCfg()
				c.PortRange = [2]int{6800, 6700}
			},
			wantErr: "start must not exceed end",
		},
		{
			name: "external sweeper needs a prefix",
			mutate: func(c *Config) {
				*c = externalCfg()
				c.UnmanagedChannels = UnmanagedPurge
				c.ChannelPrefix = ""
			},
			wantErr: "sweeper needs a prefix on shared servers",
		},
		{
			name: "external report without prefix also rejected",
			mutate: func(c *Config) {
				*c = externalCfg()
				c.UnmanagedChannels = UnmanagedReport
				c.ChannelPrefix = ""
			},
			wantErr: "sweeper needs a prefix on shared servers",
		},
		{
			name: "external purge with a prefix is ok",
			mutate: func(c *Config) {
				*c = externalCfg()
				c.UnmanagedChannels = UnmanagedPurge
				c.ChannelPrefix = "acme-"
			},
		},
		{
			name: "managed ignores the prefix rule",
			mutate: func(c *Config) {
				*c = managedCfg()
				c.UnmanagedChannels = UnmanagedPurge
			},
		},
		{
			name: "unknown unmanaged policy",
			mutate: func(c *Config) {
				*c = managedCfg()
				c.UnmanagedChannels = "delete"
			},
			wantErr: "must be ignore|report|purge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Config
			tt.mutate(&c)
			_, err := c.Validate()
			assertErr(t, err, tt.wantErr)
		})
	}
}

func TestValidate_Channels(t *testing.T) {
	tests := []struct {
		name    string
		mode    Mode
		ch      ChannelTemplate
		wantErr string
	}{
		{
			name:    "members list needs ids",
			ch:      ChannelTemplate{Name: "#x", Members: MembersList},
			wantErr: "member_list: required and non-empty",
		},
		{
			name: "members list with ids ok",
			ch: ChannelTemplate{
				Name: "#x", Members: MembersList, MemberList: []string{"reed"},
			},
		},
		{
			name:    "unknown members selector",
			ch:      ChannelTemplate{Name: "#x", Members: "some"},
			wantErr: "must be all|principals|list",
		},
		{
			name:    "name required",
			ch:      ChannelTemplate{Members: MembersAll},
			wantErr: "name: required",
		},
		{
			name: "borrowed forbids guests",
			ch: ChannelTemplate{
				Name:     "#x",
				Borrowed: &Borrowed{Host: "a", Channel: "#a", Members: MembersAll},
				Guests:   []Guest{{Profile: "p", Role: RoleVoice}},
			},
			wantErr: "guests: not allowed on a borrowed channel",
		},
		{
			name: "borrowed forbids delegates",
			mode: ModeManaged,
			ch: ChannelTemplate{
				Name:      "#x",
				Borrowed:  &Borrowed{Host: "a", Channel: "#a", Members: MembersAll},
				Delegates: []Delegate{{Workspace: "b", Bot: "wsm-b", MaxRole: RoleVoice}},
			},
			wantErr: "delegates: not allowed on a borrowed channel",
		},
		{
			name: "borrowed alone is ok",
			ch: ChannelTemplate{
				Name:     "#x",
				Borrowed: &Borrowed{Host: "a", Channel: "#a", Members: MembersPrincipals},
			},
		},
		{
			name: "borrowed needs a host",
			ch: ChannelTemplate{
				Name:     "#x",
				Borrowed: &Borrowed{Channel: "#a", Members: MembersAll},
			},
			wantErr: "borrowed.host: required",
		},
		{
			name: "guest role must be voice or op",
			ch: ChannelTemplate{
				Name:   "#x",
				Guests: []Guest{{Profile: "p", Role: "owner"}},
			},
			wantErr: "must be voice|op",
		},
		{
			// A guest entry is a direct grant, not a redistribution
			// ceiling: halfop's lateral moderation powers are not on
			// offer to a visitor.
			name: "guest role rejects halfop",
			ch: ChannelTemplate{
				Name:   "#x",
				Guests: []Guest{{Profile: "p", Role: RoleHalfop}},
			},
			wantErr: "guests[0].role \"halfop\": must be voice|op",
		},
		{
			name: "guest needs a profile",
			ch: ChannelTemplate{
				Name:   "#x",
				Guests: []Guest{{Role: RoleOp}},
			},
			wantErr: "guests[0].profile: required",
		},
		{
			name: "delegate max_role must be a known ceiling",
			mode: ModeManaged,
			ch: ChannelTemplate{
				Name:      "#x",
				Delegates: []Delegate{{Workspace: "b", Bot: "wsm-b", MaxRole: "owner"}},
			},
			wantErr: "must be voice|halfop|op",
		},
		{
			// Halfop is the rung that makes "may grant voice, may not
			// grant op" enforceable by the server itself.
			name: "delegate max_role accepts halfop",
			mode: ModeManaged,
			ch: ChannelTemplate{
				Name:      "#x",
				Delegates: []Delegate{{Workspace: "b", Bot: "wsm-b", MaxRole: RoleHalfop}},
			},
		},
		{
			name: "delegates rejected outside managed mode",
			mode: ModeExternal,
			ch: ChannelTemplate{
				Name:      "#x",
				Delegates: []Delegate{{Workspace: "b", Bot: "wsm-b", MaxRole: RoleOp}},
			},
			wantErr: "only supported when irc.mode is managed",
		},
		{
			name: "delegates allowed in managed mode",
			mode: ModeManaged,
			ch: ChannelTemplate{
				Name:      "#x",
				Delegates: []Delegate{{Workspace: "b", Bot: "wsm-b", MaxRole: RoleOp}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Config
			if tt.mode == ModeExternal {
				c = externalCfg()
			} else {
				c = managedCfg()
			}
			c.Channels = []ChannelTemplate{tt.ch}

			_, err := c.Validate()
			assertErr(t, err, tt.wantErr)
		})
	}
}

func TestValidate_Warnings(t *testing.T) {
	t.Run("plain provider warns on +R", func(t *testing.T) {
		c := externalCfg()
		c.Provider = ProviderPlain
		c.Channels = []ChannelTemplate{{Name: "#x", Modes: "+ntRi"}}

		warns, err := c.Validate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertWarning(t, warns, "irc.channels[0].modes")
	})

	t.Run("ergo provider does not warn on +R", func(t *testing.T) {
		c := externalCfg()
		c.Provider = ProviderErgo
		c.Channels = []ChannelTemplate{{Name: "#x", Modes: "+ntRi"}}

		warns, err := c.Validate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(warns) != 0 {
			t.Fatalf("expected no warnings, got %v", warns)
		}
	})

	t.Run("plain provider without R is quiet", func(t *testing.T) {
		c := externalCfg()
		c.Provider = ProviderPlain
		c.Channels = []ChannelTemplate{{Name: "#x", Modes: "+nti"}}

		warns, err := c.Validate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(warns) != 0 {
			t.Fatalf("expected no warnings, got %v", warns)
		}
	})

	t.Run("insecure_skip_verify warns", func(t *testing.T) {
		c := externalCfg()
		c.Server.InsecureSkipVerify = true

		warns, err := c.Validate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertWarning(t, warns, "irc.server.insecure_skip_verify")
	})
}

func TestServerID(t *testing.T) {
	tests := []struct {
		name string
		cfg  func() Config
		want string
	}{
		{
			name: "managed is fixed",
			cfg:  managedCfg,
			want: ManagedServerID,
		},
		{
			name: "external defaults to host:port",
			cfg: func() Config {
				c := externalCfg()
				c.Server.Port = 6697
				return c
			},
			want: "irc.example.com:6697",
		},
		{
			name: "explicit id wins",
			cfg: func() Config {
				c := externalCfg()
				c.Server.ID = "corp-irc"
				return c
			},
			want: "corp-irc",
		},
		{
			name: "managed ignores an explicit id",
			cfg: func() Config {
				c := managedCfg()
				c.Server.ID = "ignored"
				return c
			},
			want: ManagedServerID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg().ServerID(); got != tt.want {
				t.Errorf("ServerID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMerge(t *testing.T) {
	t.Run("scalars overridden", func(t *testing.T) {
		global := Defaults()
		global.Binary = "/usr/bin/ergo"
		global.Bind = "127.0.0.1"

		ws := Config{Bind: "0.0.0.0", Mode: ModeExternal}
		got := Merge(global, ws)

		if got.Bind != "0.0.0.0" {
			t.Errorf("bind = %q, want the override", got.Bind)
		}
		if got.Mode != ModeExternal {
			t.Errorf("mode = %q, want the override", got.Mode)
		}
		// Untouched scalars survive from the global layer.
		if got.Binary != "/usr/bin/ergo" {
			t.Errorf("binary = %q, want the global value", got.Binary)
		}
	})

	t.Run("channels replaced not appended", func(t *testing.T) {
		global := Defaults()
		global.Channels = []ChannelTemplate{
			{Name: "#{{ws}}"}, {Name: "#{{ws}}-ops"},
		}
		ws := Config{Channels: []ChannelTemplate{{Name: "#only"}}}

		got := Merge(global, ws)

		if len(got.Channels) != 1 || got.Channels[0].Name != "#only" {
			t.Fatalf("channels = %v, want the override list alone", got.Channels)
		}
	})

	t.Run("absent channels keep the global list", func(t *testing.T) {
		global := Defaults()
		global.Channels = []ChannelTemplate{{Name: "#{{ws}}"}}

		got := Merge(global, Config{Bind: "0.0.0.0"})

		if len(got.Channels) != 1 || got.Channels[0].Name != "#{{ws}}" {
			t.Fatalf("channels = %v, want the global list", got.Channels)
		}
	})

	t.Run("inputs not mutated", func(t *testing.T) {
		global := Defaults()
		global.Channels = []ChannelTemplate{{Name: "#a"}}
		ws := Config{Channels: []ChannelTemplate{{Name: "#b"}}}

		got := Merge(global, ws)
		got.Channels[0].Name = "#mutated"

		if global.Channels[0].Name != "#a" {
			t.Error("Merge aliased the global channel slice")
		}
		if ws.Channels[0].Name != "#b" {
			t.Error("Merge aliased the workspace channel slice")
		}
	})

	t.Run("nested structs merged field-wise", func(t *testing.T) {
		global := Defaults()
		global.Server.Host = "global.example.com"
		global.Server.Port = 6697
		global.Bot.Account = "wsm"

		ws := Config{
			Server: Server{Host: "ws.example.com"},
			Bot:    Bot{SecretRef: "irc/bot"},
		}
		got := Merge(global, ws)

		if got.Server.Host != "ws.example.com" {
			t.Errorf("server.host = %q, want the override", got.Server.Host)
		}
		// Port was not in the override, so the global value survives.
		if got.Server.Port != 6697 {
			t.Errorf("server.port = %d, want the global value", got.Server.Port)
		}
		if got.Bot.Account != "wsm" {
			t.Errorf("bot.account = %q, want the global value", got.Bot.Account)
		}
		if got.Bot.SecretRef != "irc/bot" {
			t.Errorf("bot.secret_ref = %q, want the override", got.Bot.SecretRef)
		}
	})

	t.Run("channel prefix arrives from the workspace layer", func(t *testing.T) {
		got := Merge(Defaults(), Config{ChannelPrefix: "acme-"})
		if got.ChannelPrefix != "acme-" {
			t.Errorf("channel_prefix = %q, want %q", got.ChannelPrefix, "acme-")
		}
	})

	t.Run("tls can be turned off by an override", func(t *testing.T) {
		global := Defaults() // TLS on
		got := Merge(global, Config{Server: Server{TLS: boolPtr(false)}})

		if got.Server.TLSEnabled() {
			t.Error("workspace override must be able to disable TLS")
		}
		// And the global value survives an override that omits it.
		if !Merge(global, Config{}).Server.TLSEnabled() {
			t.Error("an absent tls override must keep the global value")
		}
	})
}

// assertErr checks err against a wanted substring; empty want means no error.
func assertErr(t *testing.T, err error, want string) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Fatalf("unexpected error: %v", err)
	case want != "" && err == nil:
		t.Fatalf("expected error containing %q, got nil", want)
	case want != "" && !strings.Contains(err.Error(), want):
		t.Fatalf("error = %q, want it to contain %q", err, want)
	}
}

// assertWarning asserts that exactly one warning names field.
func assertWarning(t *testing.T, warns []Warning, field string) {
	t.Helper()
	for _, w := range warns {
		if w.Field == field {
			if w.Message == "" {
				t.Errorf("warning %q carries no message", field)
			}
			return
		}
	}
	t.Fatalf("expected a warning for %q, got %v", field, warns)
}
