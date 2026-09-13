package irc

import (
	"os"
	"strconv"
	"strings"
)

// Environment variables recognised for the irc config block. Only scalars are
// overridable: lists and per-channel structure belong in YAML.
const (
	EnvEnabled       = "WSM_IRC_ENABLED"
	EnvMode          = "WSM_IRC_MODE"
	EnvProvider      = "WSM_IRC_PROVIDER"
	EnvAutoProvision = "WSM_IRC_AUTO_PROVISION"
	EnvBinary        = "WSM_IRC_BINARY"
	EnvBind          = "WSM_IRC_BIND"
	EnvRemoteHost    = "WSM_IRC_REMOTE_HOST"
	EnvServerHost    = "WSM_IRC_SERVER_HOST"
	EnvServerPort    = "WSM_IRC_SERVER_PORT"
)

// EnvOverride applies WSM_IRC_* environment variables onto cfg. Mirrors the
// wsm precedence contract: env beats file, flags beat env.
//
// Unparseable values for typed fields (bools, ints) are ignored rather than
// fatal, matching how the wider wsm config layer treats env: a malformed
// variable must not make every command unusable.
func EnvOverride(cfg *Config) {
	if cfg == nil {
		return
	}
	if v, ok := envBool(EnvEnabled); ok {
		cfg.Enabled = v
	}
	if v := os.Getenv(EnvMode); v != "" {
		cfg.Mode = Mode(v)
	}
	if v := os.Getenv(EnvProvider); v != "" {
		cfg.Provider = ProviderName(v)
	}
	if v, ok := envBool(EnvAutoProvision); ok {
		cfg.AutoProvision = v
	}
	if v := os.Getenv(EnvBinary); v != "" {
		cfg.Binary = v
	}
	if v := os.Getenv(EnvBind); v != "" {
		cfg.Bind = v
	}
	if v := os.Getenv(EnvRemoteHost); v != "" {
		cfg.Remote.Host = v
	}
	if v := os.Getenv(EnvServerHost); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv(EnvServerPort); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.Server.Port = n
		}
	}
}

// envBool reads a boolean env var. The second result reports whether the
// variable was set to a parseable value.
func envBool(key string) (bool, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return false, false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, false
	}
	return v, true
}
