package detector

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MiseDetector detects mise tool management configuration.
type MiseDetector struct{}

var _ Detector = (*MiseDetector)(nil)

func (d *MiseDetector) Name() string { return "mise" }

// Detect checks for .mise.toml or .mise/config.toml and extracts tool info.
func (d *MiseDetector) Detect(ctx context.Context, spaceDir string) (*Result, error) {
	candidates := []string{
		filepath.Join(spaceDir, ".mise.toml"),
		filepath.Join(spaceDir, ".mise", "config.toml"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err != nil {
			continue
		}

		tools, err := parseMiseTools(path)
		if err != nil {
			return nil, fmt.Errorf("mise: parse %s: %w", path, err)
		}

		return &Result{
			Detected:   true,
			Kind:       "mise",
			Confidence: 1.0,
			Details: map[string]string{
				"config_path": path,
				"tools":       strings.Join(tools, ","),
			},
		}, nil
	}

	return &Result{Detected: false, Kind: "mise"}, nil
}

// parseMiseTools extracts tool names and versions from a mise TOML file.
// It looks for lines under [tools] sections with "name = version" patterns.
func parseMiseTools(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tools []string
	inToolsSection := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Track TOML sections.
		if strings.HasPrefix(line, "[") {
			inToolsSection = line == "[tools]"
			continue
		}

		if !inToolsSection {
			continue
		}

		// Parse "tool = version" lines.
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		version := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		if name != "" && version != "" {
			tools = append(tools, name+"@"+version)
		}
	}

	return tools, scanner.Err()
}
