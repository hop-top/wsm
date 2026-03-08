package detector

import (
	"context"
	"os"
	"path/filepath"
)

// fileExistsDetector is a base for detectors that check file existence.
type fileExistsDetector struct {
	name       string
	kind       string
	candidates []string
}

func (d *fileExistsDetector) Name() string { return d.name }

func (d *fileExistsDetector) Detect(
	_ context.Context, spaceDir string,
) (*Result, error) {
	for _, c := range d.candidates {
		path := filepath.Join(spaceDir, c)
		if _, err := os.Stat(path); err == nil {
			return &Result{
				Detected:   true,
				Kind:       d.kind,
				Details:    map[string]string{"file": c, "path": path},
				Confidence: 1.0,
			}, nil
		}
	}
	return &Result{
		Detected:   false,
		Kind:       d.kind,
		Confidence: 0.0,
	}, nil
}

// NewMakefileDetector detects Makefile presence.
func NewMakefileDetector() Detector {
	return &fileExistsDetector{
		name:       "makefile",
		kind:       "build",
		candidates: []string{"Makefile", "makefile", "GNUmakefile"},
	}
}

// NewPackageJSONDetector detects package.json presence.
func NewPackageJSONDetector() Detector {
	return &fileExistsDetector{
		name:       "package-json",
		kind:       "runtime",
		candidates: []string{"package.json"},
	}
}

// NewDockerComposeDetector detects docker-compose files.
func NewDockerComposeDetector() Detector {
	return &fileExistsDetector{
		name: "docker-compose",
		kind: "runtime",
		candidates: []string{
			"docker-compose.yml",
			"docker-compose.yaml",
			"compose.yml",
			"compose.yaml",
		},
	}
}

// NewDevcontainerDetector detects devcontainer configuration.
func NewDevcontainerDetector() Detector {
	return &fileExistsDetector{
		name:       "devcontainer",
		kind:       "runtime",
		candidates: []string{".devcontainer/devcontainer.json"},
	}
}

// NewEnvExampleDetector detects .env.example files.
func NewEnvExampleDetector() Detector {
	return &fileExistsDetector{
		name:       "env-example",
		kind:       "build",
		candidates: []string{".env.example", ".env.sample"},
	}
}

// RegisterBuiltins adds all built-in detectors to the registry.
func RegisterBuiltins(r *Registry) {
	r.Register(NewMakefileDetector())
	r.Register(NewPackageJSONDetector())
	r.Register(NewDockerComposeDetector())
	r.Register(NewDevcontainerDetector())
	r.Register(NewEnvExampleDetector())
}
