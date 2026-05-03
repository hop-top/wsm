// Package config exposes wsm runtime configuration. Resolution is a thin
// facade over hop.top/kit/go/core/config.Load: layered YAML merge across
// system / user (XDG) / project files, then env-var overrides via the
// EnvOverride hook, then CLI-flag overrides on top.
//
// Precedence (lowest → highest):
//
//	defaults → /etc/wsm/config.yaml → $XDG_CONFIG_HOME/wsm/config.yaml
//	→ ./.wsm.yaml → --config <path> → WSM_* env vars → --flag values
package config

import (
	"fmt"
	"os"
	"path/filepath"

	kitconfig "hop.top/kit/go/core/config"
	"hop.top/kit/go/core/xdg"
)

// Config is resolved runtime configuration. Public shape is preserved so
// existing call sites (cmd/wsm/commands/*) keep compiling unchanged.
type Config struct {
	Backend    string `json:"backend" yaml:"backend"`
	DataDir    string `json:"data_dir" yaml:"data_dir"`
	CacheDir   string `json:"cache_dir" yaml:"cache_dir"`
	StateDir   string `json:"state_dir" yaml:"state_dir"`
	BackendURL string `json:"backend_url" yaml:"backend_url"`

	ConfigFile      string `json:"config_file" yaml:"-"`
	ConfigFileFound bool   `json:"config_file_found" yaml:"-"`
	Source          string `json:"source" yaml:"-"`
}

// LoadOptions represent explicit CLI flags. FlagConfig, when set, replaces
// the user-scope YAML path (mirrors --config behaviour from CLI conventions).
type LoadOptions struct {
	FlagBackend    string
	FlagDataDir    string
	FlagCacheDir   string
	FlagStateDir   string
	FlagBackendURL string
	FlagConfig     string
}

const (
	systemConfigPath  = "/etc/wsm/config.yaml"
	projectConfigName = ".wsm.yaml"
)

// projectConfigCandidates lists the per-project config paths checked in
// precedence order (first existing file wins). Mirrors what `wsm init`
// can produce: legacy single-file form, hidden dir form, and the .hop/
// nested form when wsm lives inside a hop bare-worktree layout.
var projectConfigCandidates = []string{
	projectConfigName,
	filepath.Join(".wsm", "config.yaml"),
	filepath.Join(".hop", "wsm", "config.yaml"),
}

func defaultDataDir() string {
	if p, err := xdg.DataDir("wsm"); err == nil {
		return p
	}
	return ".wsm"
}

func defaultCacheDir() string {
	if p, err := xdg.CacheDir("wsm"); err == nil {
		return p
	}
	return filepath.Join(".cache", "wsm")
}

func defaultStateDir() string {
	if p, err := xdg.StateDir("wsm"); err == nil {
		return p
	}
	return filepath.Join(".local", "state", "wsm")
}

// userConfigPath returns the XDG-resolved per-user config file path.
func userConfigPath() string {
	if p, err := xdg.ConfigDir("wsm"); err == nil {
		return filepath.Join(p, "config.yaml")
	}
	return filepath.Join(".", "config", "wsm", "config.yaml")
}

// projectConfigPath returns the first existing per-project config file in
// the cwd, walking projectConfigCandidates in order. Returns empty when no
// candidate is present. The init command can produce any of the candidate
// shapes (./.wsm.yaml, ./.wsm/config.yaml, ./.hop/wsm/config.yaml) so all
// three must be honoured here.
func projectConfigPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for _, name := range projectConfigCandidates {
		p := filepath.Join(wd, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// pathExists reports whether path is non-empty and stats successfully.
func pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// envOverride applies WSM_* env vars onto cfg. Mirrors the precedence
// contract: env beats file, flags beat env (flags layered after this).
func envOverride(cfg *Config, source *string) {
	if v := os.Getenv("WSM_BACKEND"); v != "" {
		cfg.Backend = v
		*source = "env"
	}
	if v := os.Getenv("WSM_DATA_DIR"); v != "" {
		cfg.DataDir = v
		*source = "env"
	}
	if v := os.Getenv("WSM_CACHE_DIR"); v != "" {
		cfg.CacheDir = v
		*source = "env"
	}
	if v := os.Getenv("WSM_STATE_DIR"); v != "" {
		cfg.StateDir = v
		*source = "env"
	}
	if v := os.Getenv("WSM_BACKEND_URL"); v != "" {
		cfg.BackendURL = v
		*source = "env"
	}
}

// Load resolves configuration via kit/core/config.Load layered YAML merge
// (system → user/--config → project) followed by env-var and flag overrides.
func Load(opts LoadOptions) (Config, error) {
	cfg := Config{
		Backend:    "sqlite",
		DataDir:    defaultDataDir(),
		CacheDir:   defaultCacheDir(),
		StateDir:   defaultStateDir(),
		BackendURL: "",
		Source:     "defaults",
	}

	// User-scope path: --config overrides XDG location when set.
	userPath := userConfigPath()
	if opts.FlagConfig != "" {
		userPath = opts.FlagConfig
	}
	cfg.ConfigFile = userPath

	source := cfg.Source
	envHook := func(dst any) {
		c, ok := dst.(*Config)
		if !ok {
			return
		}
		envOverride(c, &source)
	}

	projectPath := projectConfigPath()
	loadOpts := kitconfig.Options{
		SystemConfigPath:  systemConfigPath,
		UserConfigPath:    userPath,
		ProjectConfigPath: projectPath,
		EnvOverride:       envHook,
	}

	// kit/core/config silently skips missing files; for explicit --config
	// we want a hard error if the user pointed us at nothing.
	if opts.FlagConfig != "" {
		if _, err := os.Stat(opts.FlagConfig); err != nil {
			return Config{}, fmt.Errorf("config file %s: %w", opts.FlagConfig, err)
		}
	}

	if err := kitconfig.Load(&cfg, loadOpts); err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}

	// Track whether any YAML layer actually existed on disk; that drives
	// the legacy "source: config_file" string the e2e tests assert on.
	// Pick the highest-precedence existing file as the reported source so
	// `wsm config show` points users at the file that actually contributed
	// (project beats user beats system). Fall back to the user/--config
	// path for the "(not found)" case so the hint stays meaningful.
	fileLoaded := pathExists(systemConfigPath) || pathExists(userPath) ||
		pathExists(projectPath)
	switch {
	case pathExists(projectPath):
		cfg.ConfigFile = projectPath
		cfg.ConfigFileFound = true
	case pathExists(userPath):
		cfg.ConfigFile = userPath
		cfg.ConfigFileFound = true
	case pathExists(systemConfigPath):
		cfg.ConfigFile = systemConfigPath
		cfg.ConfigFileFound = true
	}
	if fileLoaded && source == "defaults" {
		source = "config_file"
	}

	if opts.FlagBackend != "" {
		cfg.Backend = opts.FlagBackend
		source = "flags"
	}
	if opts.FlagDataDir != "" {
		cfg.DataDir = opts.FlagDataDir
		source = "flags"
	}
	if opts.FlagCacheDir != "" {
		cfg.CacheDir = opts.FlagCacheDir
		source = "flags"
	}
	if opts.FlagStateDir != "" {
		cfg.StateDir = opts.FlagStateDir
		source = "flags"
	}
	if opts.FlagBackendURL != "" {
		cfg.BackendURL = opts.FlagBackendURL
		source = "flags"
	}

	if cfg.DataDir == "" {
		cfg.DataDir = defaultDataDir()
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = defaultCacheDir()
	}
	if cfg.StateDir == "" {
		cfg.StateDir = defaultStateDir()
	}
	cfg.Source = source

	return cfg, nil
}
