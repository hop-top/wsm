package irc

import (
	"fmt"
	"os"
	"path/filepath"

	kitconfig "hop.top/kit/go/core/config"
)

// projectConfigCandidates lists the per-workspace config paths checked in
// precedence order (first existing file wins). Mirrors pkg/config so a
// workspace laid out by `wsm init` is found the same way here.
var projectConfigCandidates = []string{
	".wsm.yaml",
	filepath.Join(".wsm", "config.yaml"),
	filepath.Join(".hop", "wsm", "config.yaml"),
}

// fileConfig is the YAML envelope: the irc block lives under an `irc:` key in
// a wsm config file rather than at the document root.
type fileConfig struct {
	IRC Config `json:"irc" yaml:"irc"`
}

// Load resolves the irc config block from a single wsm config file, then
// applies WSM_IRC_* env overrides. A missing path yields defaults, matching
// kit's "conventional location, not a user assertion" rule; an unreadable or
// malformed file is an error.
//
// Callers that also need a per-workspace override should call LoadWorkspace,
// which layers the workspace file on top via Merge.
func Load(path string) (Config, error) {
	cfg := Defaults()

	if path != "" {
		var envelope fileConfig
		envelope.IRC = cfg
		opts := kitconfig.Options{UserConfigPath: path}
		if err := kitconfig.Load(&envelope, opts); err != nil {
			return Config{}, fmt.Errorf("load irc config %s: %w", path, err)
		}
		cfg = envelope.IRC
	}

	EnvOverride(&cfg)
	return cfg, nil
}

// LoadWorkspace resolves the effective config for one workspace: the global
// file, then the workspace override found under root, then env.
//
// The override is looked up in root using the same candidate list as
// `wsm init` can produce. An absent override is not an error.
func LoadWorkspace(globalPath, root string) (Config, error) {
	global := Defaults()

	if globalPath != "" {
		var envelope fileConfig
		envelope.IRC = global
		if err := kitconfig.Load(&envelope, kitconfig.Options{
			UserConfigPath: globalPath,
		}); err != nil {
			return Config{}, fmt.Errorf("load irc config %s: %w", globalPath, err)
		}
		global = envelope.IRC
	}

	cfg := global
	if overridePath := WorkspaceConfigPath(root); overridePath != "" {
		// The override layer starts from the zero value, not from defaults:
		// Merge must be able to tell "absent" from "set to the default".
		var envelope fileConfig
		if err := kitconfig.Load(&envelope, kitconfig.Options{
			UserConfigPath: overridePath,
		}); err != nil {
			return Config{}, fmt.Errorf(
				"load irc workspace override %s: %w", overridePath, err,
			)
		}
		cfg = Merge(global, envelope.IRC)
	}

	EnvOverride(&cfg)
	return cfg, nil
}

// WorkspaceConfigPath returns the first existing wsm config file under root,
// walking the candidate list in precedence order. Returns empty when root is
// empty or holds no candidate.
func WorkspaceConfigPath(root string) string {
	if root == "" {
		return ""
	}
	for _, name := range projectConfigCandidates {
		p := filepath.Join(root, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
