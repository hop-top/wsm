package detector

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// NewPackageRegistryDetector checks for npm/pypi/go module markers.
func NewPackageRegistryDetector() Detector {
	return &packageRegistryDetector{}
}

type packageRegistryDetector struct{}

func (d *packageRegistryDetector) Name() string { return "package-registry" }

func (d *packageRegistryDetector) Detect(
	_ context.Context, spaceDir string,
) (*Result, error) {
	markers := map[string]string{
		"package.json":   "npm",
		"setup.py":       "pypi",
		"pyproject.toml": "pypi",
		"go.mod":         "go",
	}

	found := make(map[string]string)
	for file, registry := range markers {
		path := filepath.Join(spaceDir, file)
		if _, err := os.Stat(path); err == nil {
			found[registry] = file
		}
	}

	if len(found) == 0 {
		return &Result{
			Detected:   false,
			Kind:       "knowledge",
			Confidence: 0.0,
		}, nil
	}

	details := make(map[string]string)
	var registries []string
	for reg, file := range found {
		details["registry_"+reg] = file
		registries = append(registries, reg)
	}
	details["registries"] = strings.Join(registries, ",")

	return &Result{
		Detected:   true,
		Kind:       "knowledge",
		Details:    details,
		Confidence: 1.0,
	}, nil
}

// NewLocalDocsDetector checks for local documentation files/dirs.
func NewLocalDocsDetector() Detector {
	return &localDocsDetector{}
}

type localDocsDetector struct{}

func (d *localDocsDetector) Name() string { return "local-docs" }

func (d *localDocsDetector) Detect(
	_ context.Context, spaceDir string,
) (*Result, error) {
	prefixes := []string{"README", "CONTRIBUTING", "CHANGELOG"}
	dirs := []string{"docs"}

	details := make(map[string]string)
	count := 0

	// Check directories
	for _, dir := range dirs {
		path := filepath.Join(spaceDir, dir)
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			details["dir_"+dir] = "true"
			count++
		}
	}

	// Check file prefixes
	entries, err := os.ReadDir(spaceDir)
	if err != nil {
		return &Result{
			Detected:   false,
			Kind:       "knowledge",
			Confidence: 0.0,
		}, nil
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		for _, prefix := range prefixes {
			if strings.HasPrefix(strings.ToUpper(name), prefix) {
				details["file_"+strings.ToLower(prefix)] = name
				count++
				break
			}
		}
	}

	if count == 0 {
		return &Result{
			Detected:   false,
			Kind:       "knowledge",
			Confidence: 0.0,
		}, nil
	}

	return &Result{
		Detected:   true,
		Kind:       "knowledge",
		Details:    details,
		Confidence: float64(count) / float64(len(prefixes)+len(dirs)),
	}, nil
}
