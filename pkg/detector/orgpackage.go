package detector

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// OrgPackageKind classifies org package marker detections.
const OrgPackageKind = "agentcompanies/v1"

// orgPackageMarker is the marker file expected at the space boundary root.
const orgPackageMarker = "COMPANY.md"

// OrgPackageDetector detects an org package marker (COMPANY.md) at the
// space boundary root. It does not recurse into subdirectories and reads
// only the marker file, best-effort extracting name and slug from its
// YAML frontmatter. Parse failures never fail the scan: the detection
// still fires with path and kind only.
type OrgPackageDetector struct{}

var _ Detector = (*OrgPackageDetector)(nil)

func (d *OrgPackageDetector) Name() string { return "org-package" }

// Detect checks for COMPANY.md at the boundary root.
func (d *OrgPackageDetector) Detect(_ context.Context, spaceDir string) (*Result, error) {
	path := filepath.Join(spaceDir, orgPackageMarker)
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return &Result{Detected: false, Kind: OrgPackageKind}, nil
	}

	details := map[string]string{"path": orgPackageMarker}
	if name, slug, ok := parseOrgPackageFrontmatter(path); ok {
		if name != "" {
			details["name"] = name
		}
		if slug != "" {
			details["slug"] = slug
		}
	}

	return &Result{
		Detected:   true,
		Kind:       OrgPackageKind,
		Confidence: 1.0,
		Details:    details,
	}, nil
}

// parseOrgPackageFrontmatter best-effort extracts name and slug from the
// marker's YAML frontmatter (--- delimited block at the top of the file).
// Any read or parse failure returns ok=false; it never errors.
func parseOrgPackageFrontmatter(path string) (name, slug string, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", false
	}

	body := bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	if !bytes.HasPrefix(body, []byte("---\n")) &&
		!bytes.HasPrefix(body, []byte("---\r\n")) {
		return "", "", false
	}

	// Find the closing delimiter after the opening line.
	rest := body[bytes.IndexByte(body, '\n')+1:]
	end := -1
	for _, delim := range []string{"\n---\n", "\n---\r\n"} {
		if i := bytes.Index(rest, []byte(delim)); i >= 0 && (end < 0 || i < end) {
			end = i
		}
	}
	if end < 0 {
		if bytes.HasSuffix(rest, []byte("\n---")) {
			end = len(rest) - len("\n---")
		} else if bytes.HasPrefix(rest, []byte("---\n")) || bytes.HasPrefix(rest, []byte("---\r\n")) {
			end = 0
		} else {
			return "", "", false
		}
	}

	var fm struct {
		Name string `yaml:"name"`
		Slug string `yaml:"slug"`
	}
	if err := yaml.Unmarshal(rest[:end], &fm); err != nil {
		return "", "", false
	}
	return strings.TrimSpace(fm.Name), strings.TrimSpace(fm.Slug), true
}
