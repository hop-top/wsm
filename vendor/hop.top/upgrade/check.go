package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

type releaseInfo struct {
	Version string
	URL     string
	Notes   string
}

// ghRelease mirrors the GitHub releases API shape we care about.
type ghRelease struct {
	TagName string      `json:"tag_name"`
	Body    string      `json:"body"`
	Assets  []ghAsset   `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func fetchLatest(ctx context.Context, cfg Config) (*releaseInfo, error) {
	if cfg.ReleaseURL != "" {
		return fetchCustomURL(ctx, cfg)
	}
	if cfg.GitHubRepo != "" {
		return fetchGitHub(ctx, cfg)
	}
	return nil, fmt.Errorf("upgrade: no release source configured")
}

func fetchGitHub(ctx context.Context, cfg Config) (*releaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", cfg.GitHubRepo)
	client := &http.Client{Timeout: cfg.Timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upgrade: github API returned %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("upgrade: decode github response: %w", err)
	}

	version := strings.TrimPrefix(rel.TagName, "v")
	assetURL := selectAsset(rel.Assets)

	notes := rel.Body
	if len(notes) > 1000 {
		notes = notes[:1000] + "…"
	}

	return &releaseInfo{Version: version, URL: assetURL, Notes: notes}, nil
}

// selectAsset picks the best asset for current OS/arch.
func selectAsset(assets []ghAsset) string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// normalise arch names used in goreleaser convention
	archAlias := map[string][]string{
		"amd64": {"amd64", "x86_64"},
		"arm64": {"arm64", "aarch64"},
	}

	aliases := archAlias[goarch]
	if aliases == nil {
		aliases = []string{goarch}
	}

	for _, a := range assets {
		name := strings.ToLower(a.Name)
		if !strings.Contains(name, goos) {
			continue
		}
		for _, arch := range aliases {
			if strings.Contains(name, arch) {
				return a.BrowserDownloadURL
			}
		}
	}
	// fallback: first asset whose name contains OS
	for _, a := range assets {
		if strings.Contains(strings.ToLower(a.Name), goos) {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// customRelease matches the shape expected from a custom release URL.
type customRelease struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Notes   string `json:"notes"`
}

func fetchCustomURL(ctx context.Context, cfg Config) (*releaseInfo, error) {
	client := &http.Client{Timeout: cfg.Timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.ReleaseURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rel customRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("upgrade: decode custom release: %w", err)
	}

	return &releaseInfo{
		Version: strings.TrimPrefix(rel.Version, "v"),
		URL:     rel.URL,
		Notes:   rel.Notes,
	}, nil
}

// isNewer returns true if latest > current (semver, best-effort).
func isNewer(current, latest string) bool {
	if current == "" || latest == "" {
		return false
	}
	c := parseSemver(strings.TrimPrefix(current, "v"))
	l := parseSemver(strings.TrimPrefix(latest, "v"))
	return compareSemver(l, c) > 0
}

// semver holds a parsed semantic version per semver.org 2.0.0.
type semver struct {
	major, minor, patch int
	pre                 string // raw pre-release string, e.g. "alpha.1"
}

// parseSemver parses a semver string (without leading "v").
// Build metadata (+…) is ignored. Pre-release (-…) is retained for ordering.
func parseSemver(s string) semver {
	var v semver

	// strip build metadata
	if idx := strings.Index(s, "+"); idx >= 0 {
		s = s[:idx]
	}

	// split pre-release
	if idx := strings.Index(s, "-"); idx >= 0 {
		v.pre = s[idx+1:]
		s = s[:idx]
	}

	parts := strings.SplitN(s, ".", 3)
	if len(parts) >= 1 {
		fmt.Sscanf(parts[0], "%d", &v.major)
	}
	if len(parts) >= 2 {
		fmt.Sscanf(parts[1], "%d", &v.minor)
	}
	if len(parts) >= 3 {
		fmt.Sscanf(parts[2], "%d", &v.patch)
	}
	return v
}

// compareSemver returns >0 if a > b, <0 if a < b, 0 if equal.
// Follows semver 2.0.0 §11: pre-release < release of same version.
// Pre-release identifiers are compared dot-by-dot (numeric then lexicographic).
func compareSemver(a, b semver) int {
	if d := a.major - b.major; d != 0 {
		return d
	}
	if d := a.minor - b.minor; d != 0 {
		return d
	}
	if d := a.patch - b.patch; d != 0 {
		return d
	}
	// same core version — compare pre-release
	// §11.3: no pre-release > has pre-release
	switch {
	case a.pre == "" && b.pre == "":
		return 0
	case a.pre == "" && b.pre != "":
		return 1 // a is release, b is pre-release → a > b
	case a.pre != "" && b.pre == "":
		return -1 // a is pre-release, b is release → a < b
	default:
		return comparePreRelease(a.pre, b.pre)
	}
}

// comparePreRelease compares two pre-release strings identifier by identifier.
func comparePreRelease(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	n := len(aParts)
	if len(bParts) < n {
		n = len(bParts)
	}
	for i := 0; i < n; i++ {
		d := comparePreIdent(aParts[i], bParts[i])
		if d != 0 {
			return d
		}
	}
	// longer pre-release wins (e.g. "alpha.1" > "alpha")
	return len(aParts) - len(bParts)
}

// comparePreIdent compares a single pre-release identifier.
// Pure numeric identifiers are compared as integers; otherwise lexicographically.
func comparePreIdent(a, b string) int {
	var ai, bi int
	an, _ := fmt.Sscanf(a, "%d", &ai)
	bn, _ := fmt.Sscanf(b, "%d", &bi)
	aNum := an == 1 && fmt.Sprintf("%d", ai) == a
	bNum := bn == 1 && fmt.Sprintf("%d", bi) == b
	switch {
	case aNum && bNum:
		return ai - bi
	case aNum: // numeric < alphanumeric
		return -1
	case bNum:
		return 1
	default:
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}
}

// ensure time import is used (for future cache stamps)
var _ = time.Now
