package detector

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FnoxDetector detects fnox function/secret binding configuration.
type FnoxDetector struct{}

var _ Detector = (*FnoxDetector)(nil)

func (d *FnoxDetector) Name() string { return "fnox" }

// Detect checks for fnox.toml or .fnox.toml and extracts bindings.
func (d *FnoxDetector) Detect(ctx context.Context, spaceDir string) (*Result, error) {
	candidates := []string{
		filepath.Join(spaceDir, "fnox.toml"),
		filepath.Join(spaceDir, ".fnox.toml"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err != nil {
			continue
		}

		bindings, err := parseFnoxBindings(path)
		if err != nil {
			return nil, fmt.Errorf("fnox: parse %s: %w", path, err)
		}

		return &Result{
			Detected:   true,
			Kind:       "fnox",
			Confidence: 1.0,
			Details: map[string]string{
				"config_path":    path,
				"bindings_count": strconv.Itoa(len(bindings)),
			},
		}, nil
	}

	return &Result{Detected: false, Kind: "fnox"}, nil
}

// parseFnoxBindings extracts function/secret bindings from a fnox TOML file.
// Looks for [bindings.<name>] section headers or key=value under [bindings].
func parseFnoxBindings(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var bindings []string
	inBindingsSection := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Detect [bindings.NAME] subsection headers.
		if strings.HasPrefix(line, "[bindings.") && strings.HasSuffix(line, "]") {
			name := line[len("[bindings.") : len(line)-1]
			name = strings.TrimSpace(name)
			if name != "" {
				bindings = append(bindings, name)
			}
			inBindingsSection = false
			continue
		}

		// Track [bindings] flat section.
		if strings.HasPrefix(line, "[") {
			inBindingsSection = line == "[bindings]"
			continue
		}

		// In flat [bindings] section, each key = value is a binding.
		if inBindingsSection {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[0])
				if name != "" {
					bindings = append(bindings, name)
				}
			}
		}
	}

	return bindings, scanner.Err()
}
