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
// Returns an error if both exist at the same directory level and are not
// symlinked to each other.
func FindAnyLocalConfig(startDir string) (*LocalConfig, error) {
	lc, err := FindLocalConfig(startDir)
	if err != nil {
		return nil, err
	}
	if lc.Found {
		// Check for conflict at the directory where we found .wsm/config.yaml.
		configDir := filepath.Dir(filepath.Dir(lc.Path)) // .wsm/config.yaml -> project dir
		if err := checkMutualExclusion(configDir); err != nil {
			return nil, err
		}
		return lc, nil
	}

	lc, err = FindHopLocalConfig(startDir)
	if err != nil {
		return nil, err
	}
	if lc.Found {
		// Check for conflict at the directory where we found .hop/wsm/config.yaml.
		configDir := filepath.Dir(filepath.Dir(filepath.Dir(lc.Path))) // .hop/wsm/config.yaml -> project dir
		if err := checkMutualExclusion(configDir); err != nil {
			return nil, err
		}
	}
	return lc, nil
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

// checkMutualExclusion returns an error if both .wsm/ and .hop/wsm/ exist
// at dir as independent (non-symlinked) directories. If one is a symlink
// to the other, they are considered equivalent and no error is returned.
func checkMutualExclusion(dir string) error {
	wsmDir := filepath.Join(dir, ".wsm")
	hopDir := filepath.Join(dir, ".hop", "wsm")

	// Both must exist for a conflict.
	if _, err := os.Lstat(wsmDir); err != nil {
		return nil
	}
	if _, err := os.Lstat(hopDir); err != nil {
		return nil
	}

	// Resolve symlinks and compare real paths. Broken symlinks (dangling
	// targets) cause EvalSymlinks to fail; treat them as non-conflicting
	// since findLocalConfigIn uses os.Stat which also fails on broken
	// symlinks, so they can't be "found" configs.
	wsmReal, err := filepath.EvalSymlinks(wsmDir)
	if err != nil {
		return nil
	}
	hopReal, err := filepath.EvalSymlinks(hopDir)
	if err != nil {
		return nil
	}

	if wsmReal == hopReal {
		return nil // symlinked to same location
	}

	return fmt.Errorf(
		"conflicting local config: both %s and %s exist independently; "+
			"symlink one to the other to resolve",
		wsmDir, hopDir,
	)
}

// InitLocalConfig creates a local config directory with a minimal config.yaml
// and symlinks the other path to it. If hop is true, creates .hop/wsm/ and
// symlinks .wsm/ to it; otherwise creates .wsm/ and symlinks .hop/wsm/ to it.
// Returns an error if either directory already exists (unless force is true).
func InitLocalConfig(dir string, hop bool, force bool) (string, error) {
	wsmDir := filepath.Join(dir, ".wsm")
	hopDir := filepath.Join(dir, ".hop", "wsm")

	// Check for existing directories.
	wsmExists := false
	if _, err := os.Lstat(wsmDir); err == nil {
		wsmExists = true
	}
	hopExists := false
	if _, err := os.Lstat(hopDir); err == nil {
		hopExists = true
	}

	if !force && (wsmExists || hopExists) {
		if wsmExists && hopExists {
			return "", fmt.Errorf("both %s and %s already exist; use --force to overwrite", wsmDir, hopDir)
		}
		existing := wsmDir
		if hopExists {
			existing = hopDir
		}
		return "", fmt.Errorf("%s already exists; use --force to overwrite", existing)
	}

	// Remove existing if force.
	if force {
		if wsmExists {
			_ = os.RemoveAll(wsmDir)
		}
		if hopExists {
			_ = os.RemoveAll(hopDir)
		}
	}

	var primary, symlinkPath string
	if hop {
		primary = hopDir
		symlinkPath = wsmDir
	} else {
		primary = wsmDir
		symlinkPath = hopDir
	}

	// Create primary directory.
	if err := os.MkdirAll(primary, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", primary, err)
	}

	// Write minimal config.
	configPath := filepath.Join(primary, "config.yaml")
	content := []byte("# wsm local configuration\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", configPath, err)
	}

	// Create symlink parent directory if needed.
	symlinkParent := filepath.Dir(symlinkPath)
	if err := os.MkdirAll(symlinkParent, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", symlinkParent, err)
	}

	// Create symlink.
	if err := os.Symlink(primary, symlinkPath); err != nil {
		return "", fmt.Errorf("symlink %s -> %s: %w", symlinkPath, primary, err)
	}

	return configPath, nil
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
