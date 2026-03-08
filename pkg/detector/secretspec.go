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

// SecretSpecDetector detects SecretSpec configuration files.
type SecretSpecDetector struct{}

var _ Detector = (*SecretSpecDetector)(nil)

func (d *SecretSpecDetector) Name() string { return "secretspec" }

// Detect checks for secretspec.toml or .secretspec.toml and extracts secrets.
func (d *SecretSpecDetector) Detect(ctx context.Context, spaceDir string) (*Result, error) {
	candidates := []string{
		filepath.Join(spaceDir, "secretspec.toml"),
		filepath.Join(spaceDir, ".secretspec.toml"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err != nil {
			continue
		}

		keys, err := parseSecretSpecKeys(path)
		if err != nil {
			return nil, fmt.Errorf("secretspec: parse %s: %w", path, err)
		}

		return &Result{
			Detected:   true,
			Kind:       "secretspec",
			Confidence: 1.0,
			Details: map[string]string{
				"config_path":   path,
				"secrets_count": strconv.Itoa(len(keys)),
				"secret_keys":   strings.Join(keys, ","),
			},
		}, nil
	}

	return &Result{Detected: false, Kind: "secretspec"}, nil
}

// parseSecretSpecKeys extracts secret key names from a secretspec TOML file.
// Looks for [secrets.<name>] section headers or key = value lines under
// [secrets].
func parseSecretSpecKeys(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var keys []string
	inSecretsSection := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Detect [secrets.NAME] subsection headers.
		if strings.HasPrefix(line, "[secrets.") && strings.HasSuffix(line, "]") {
			name := line[len("[secrets.") : len(line)-1]
			name = strings.TrimSpace(name)
			if name != "" {
				keys = append(keys, name)
			}
			inSecretsSection = false
			continue
		}

		// Track [secrets] flat section.
		if strings.HasPrefix(line, "[") {
			inSecretsSection = line == "[secrets]"
			continue
		}

		// In flat [secrets] section, each key = value is a secret.
		if inSecretsSection {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[0])
				if name != "" {
					keys = append(keys, name)
				}
			}
		}
	}

	return keys, scanner.Err()
}
