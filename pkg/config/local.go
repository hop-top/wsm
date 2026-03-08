package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LocalConfig represents project-level configuration found in a local
// directory (.wsm/ or .hop/wsm/).
type LocalConfig struct {
	Path     string            `json:"path"`
	Found    bool              `json:"found"`
	Settings map[string]string `json:"settings"`
}

// FindLocalConfig walks up from startDir looking for .wsm/config.yaml.
// Returns a LocalConfig with Found=false if none exists.
func FindLocalConfig(startDir string) (*LocalConfig, error) {
	return findLocalConfigIn(startDir, filepath.Join(".wsm", "config.yaml"))
}

// FindHopLocalConfig walks up from startDir looking for .hop/wsm/config.yaml.
// Returns a LocalConfig with Found=false if none exists.
func FindHopLocalConfig(startDir string) (*LocalConfig, error) {
	return findLocalConfigIn(startDir, filepath.Join(".hop", "wsm", "config.yaml"))
}

// FindAnyLocalConfig tries .wsm/ first, then .hop/wsm/.
// Returns the first match or Found=false if neither exists.
func FindAnyLocalConfig(startDir string) (*LocalConfig, error) {
	lc, err := FindLocalConfig(startDir)
	if err != nil {
		return nil, err
	}
	if lc.Found {
		return lc, nil
	}
	return FindHopLocalConfig(startDir)
}

// LoadLocalConfig parses a local config file at the given path.
func LoadLocalConfig(path string) (*LocalConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read local config %s: %w", path, err)
	}
	var settings map[string]string
	if err := yaml.Unmarshal(b, &settings); err != nil {
		return nil, fmt.Errorf("parse local config %s: %w", path, err)
	}
	if settings == nil {
		settings = make(map[string]string)
	}
	return &LocalConfig{
		Path:     path,
		Found:    true,
		Settings: settings,
	}, nil
}

func findLocalConfigIn(startDir, relPath string) (*LocalConfig, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("resolve start dir: %w", err)
	}
	for {
		candidate := filepath.Join(dir, relPath)
		if _, err := os.Stat(candidate); err == nil {
			return LoadLocalConfig(candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return &LocalConfig{Found: false}, nil
}
