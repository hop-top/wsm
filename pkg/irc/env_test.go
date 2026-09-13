package irc

import "testing"

// clearIRCEnv unsets every WSM_IRC_* variable for the duration of a test, so
// the ambient environment cannot leak into an assertion.
func clearIRCEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		EnvEnabled, EnvMode, EnvProvider, EnvAutoProvision, EnvBinary,
		EnvBind, EnvRemoteHost, EnvServerHost, EnvServerPort,
	} {
		t.Setenv(k, "")
	}
}

func TestEnvOverride(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		check func(Config) bool
	}{
		{
			name: "enabled", key: EnvEnabled, value: "true",
			check: func(c Config) bool { return c.Enabled },
		},
		{
			name: "mode", key: EnvMode, value: "external",
			check: func(c Config) bool { return c.Mode == ModeExternal },
		},
		{
			name: "provider", key: EnvProvider, value: "plain",
			check: func(c Config) bool { return c.Provider == ProviderPlain },
		},
		{
			name: "auto_provision", key: EnvAutoProvision, value: "1",
			check: func(c Config) bool { return c.AutoProvision },
		},
		{
			name: "binary", key: EnvBinary, value: "/opt/ergo",
			check: func(c Config) bool { return c.Binary == "/opt/ergo" },
		},
		{
			name: "bind", key: EnvBind, value: "0.0.0.0",
			check: func(c Config) bool { return c.Bind == "0.0.0.0" },
		},
		{
			name: "remote host", key: EnvRemoteHost, value: "chat.example.com",
			check: func(c Config) bool { return c.Remote.Host == "chat.example.com" },
		},
		{
			name: "server host", key: EnvServerHost, value: "irc.example.com",
			check: func(c Config) bool { return c.Server.Host == "irc.example.com" },
		},
		{
			name: "server port", key: EnvServerPort, value: "6667",
			check: func(c Config) bool { return c.Server.Port == 6667 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearIRCEnv(t)
			t.Setenv(tt.key, tt.value)

			cfg := Defaults()
			EnvOverride(&cfg)

			if !tt.check(cfg) {
				t.Fatalf("%s=%q did not apply: %+v", tt.key, tt.value, cfg)
			}
		})
	}
}

func TestEnvOverride_Unset(t *testing.T) {
	clearIRCEnv(t)

	want := Defaults()
	got := Defaults()
	EnvOverride(&got)

	if got.Mode != want.Mode || got.Provider != want.Provider ||
		got.Binary != want.Binary || got.Server.Port != want.Server.Port {
		t.Fatalf("empty env changed the config: %+v", got)
	}
	if got.Enabled {
		t.Error("empty env must not enable irc")
	}
}

func TestEnvOverride_Malformed(t *testing.T) {
	// A malformed value must be ignored rather than fatal: a typo in a shell
	// profile must not make every wsm irc command unusable.
	tests := []struct {
		name  string
		key   string
		value string
		check func(Config) bool
	}{
		{
			name: "bool", key: EnvEnabled, value: "yes-please",
			check: func(c Config) bool { return !c.Enabled },
		},
		{
			name: "int", key: EnvServerPort, value: "six-thousand",
			check: func(c Config) bool { return c.Server.Port == DefaultExternalPort },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearIRCEnv(t)
			t.Setenv(tt.key, tt.value)

			cfg := Defaults()
			EnvOverride(&cfg)

			if !tt.check(cfg) {
				t.Fatalf("malformed %s=%q was applied: %+v", tt.key, tt.value, cfg)
			}
		})
	}
}

func TestEnvOverride_BoolForms(t *testing.T) {
	// strconv.ParseBool accepts these; "false" must be able to turn a flag
	// off, not merely fail to turn it on.
	for _, tt := range []struct {
		value string
		want  bool
	}{
		{"true", true}, {"1", true}, {"TRUE", true},
		{"false", false}, {"0", false},
	} {
		t.Run(tt.value, func(t *testing.T) {
			clearIRCEnv(t)
			t.Setenv(EnvEnabled, tt.value)

			cfg := Defaults()
			cfg.Enabled = true // start on, so "false" has something to clear
			EnvOverride(&cfg)

			if cfg.Enabled != tt.want {
				t.Fatalf("%s=%q gave enabled=%v, want %v",
					EnvEnabled, tt.value, cfg.Enabled, tt.want)
			}
		})
	}
}

func TestEnvOverride_NilSafe(t *testing.T) {
	// The daemon calls this on an optional config pointer; a nil must not
	// panic.
	EnvOverride(nil)
}
