package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"hop.top/kit/go/core/xdg"
)

// Config is resolved runtime configuration.
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

// LoadOptions represent explicit CLI flags.
type LoadOptions struct {
	FlagBackend    string
	FlagDataDir    string
	FlagCacheDir   string
	FlagStateDir   string
	FlagBackendURL string
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

func resolveConfigFile() string {
	if p, err := xdg.ConfigDir("wsm"); err == nil {
		return filepath.Join(p, "config.yaml")
	}
	return filepath.Join(".", "config", "wsm", "config.yaml")
}

// Load resolves configuration with precedence flags > env > file > defaults.
func Load(opts LoadOptions) (Config, error) {
	cfg := Config{
		Backend:    "sqlite",
		DataDir:    defaultDataDir(),
		CacheDir:   defaultCacheDir(),
		StateDir:   defaultStateDir(),
		BackendURL: "",
		Source:     "defaults",
	}
	cfg.ConfigFile = resolveConfigFile()

	if b, err := os.ReadFile(cfg.ConfigFile); err == nil {
		cfg.ConfigFileFound = true
		var fileCfg Config
		if err := yaml.Unmarshal(b, &fileCfg); err != nil {
			return Config{}, fmt.Errorf("parse config file %s: %w", cfg.ConfigFile, err)
		}
		if fileCfg.Backend != "" {
			cfg.Backend = fileCfg.Backend
		}
		if fileCfg.DataDir != "" {
			cfg.DataDir = fileCfg.DataDir
		}
		if fileCfg.CacheDir != "" {
			cfg.CacheDir = fileCfg.CacheDir
		}
		if fileCfg.StateDir != "" {
			cfg.StateDir = fileCfg.StateDir
		}
		if fileCfg.BackendURL != "" {
			cfg.BackendURL = fileCfg.BackendURL
		}
		cfg.Source = "config_file"
	}

	if v := os.Getenv("WSM_BACKEND"); v != "" {
		cfg.Backend = v
		cfg.Source = "env"
	}
	if v := os.Getenv("WSM_DATA_DIR"); v != "" {
		cfg.DataDir = v
		cfg.Source = "env"
	}
	if v := os.Getenv("WSM_CACHE_DIR"); v != "" {
		cfg.CacheDir = v
		cfg.Source = "env"
	}
	if v := os.Getenv("WSM_STATE_DIR"); v != "" {
		cfg.StateDir = v
		cfg.Source = "env"
	}
	if v := os.Getenv("WSM_BACKEND_URL"); v != "" {
		cfg.BackendURL = v
		cfg.Source = "env"
	}

	if opts.FlagBackend != "" {
		cfg.Backend = opts.FlagBackend
		cfg.Source = "flags"
	}
	if opts.FlagDataDir != "" {
		cfg.DataDir = opts.FlagDataDir
		cfg.Source = "flags"
	}
	if opts.FlagCacheDir != "" {
		cfg.CacheDir = opts.FlagCacheDir
		cfg.Source = "flags"
	}
	if opts.FlagStateDir != "" {
		cfg.StateDir = opts.FlagStateDir
		cfg.Source = "flags"
	}
	if opts.FlagBackendURL != "" {
		cfg.BackendURL = opts.FlagBackendURL
		cfg.Source = "flags"
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

	return cfg, nil
}
