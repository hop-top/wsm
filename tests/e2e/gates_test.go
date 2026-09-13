package e2e_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// e2eRefRegexp matches a repo-relative reference to an e2e test file as it
// appears in story markdown, e.g. `tests/e2e/workspace_test.go`.
var e2eRefRegexp = regexp.MustCompile(`tests/e2e/[A-Za-z0-9_.-]+\.go`)

func TestStoryCoverageMatrix_001To026(t *testing.T) {
	r := newRunner(t)

	storyFiles, err := filepath.Glob(filepath.Join(r.root, "docs", "stories", "*.md"))
	if err != nil {
		t.Fatalf("list story files: %v", err)
	}
	if len(storyFiles) == 0 {
		t.Fatal("no story files found")
	}

	idRe := regexp.MustCompile(`^0[0-9]{2}$|^0[12][0-9]$`)
	covered := map[string]bool{}
	for _, file := range storyFiles {
		base := filepath.Base(file)
		if strings.EqualFold(base, "README.md") {
			continue
		}
		id := strings.SplitN(base, "-", 2)[0]
		if !idRe.MatchString(id) {
			continue
		}
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}

		// Every e2e path a story names must resolve to a real file. A story
		// that quotes a path which was never created claims coverage it does
		// not have, so resolve each reference against the repo root.
		refs := uniqueStrings(e2eRefRegexp.FindAllString(string(content), -1))
		for _, ref := range refs {
			abs := filepath.Join(r.root, filepath.FromSlash(ref))
			info, err := os.Stat(abs)
			if err != nil {
				t.Errorf("story %s references %s, which does not exist on disk", base, ref)
				continue
			}
			if info.IsDir() {
				t.Errorf("story %s references %s, which is a directory, not a test file", base, ref)
			}
		}
		if len(refs) > 0 {
			covered[id] = true
		}
	}

	// Stories with no end-to-end coverage at all. The TUI ships (see
	// cmd/wsm/commands/tui_charm.go) and is exercised by unit tests in
	// pkg/tui, but nothing drives it end to end. These are exempt from the
	// "must name an e2e file" rule rather than being papered over with a
	// reference to a file that does not exist, or a stub test that asserts
	// nothing. Delete an entry here once real coverage lands.
	noE2ECoverage := map[string]bool{
		"021": true,
		"022": true,
	}

	for i := 1; i <= 26; i++ {
		id := ""
		if i < 10 {
			id = "00" + strconv.Itoa(i)
		} else {
			id = "0" + strconv.Itoa(i)
		}
		if noE2ECoverage[id] {
			continue
		}
		if !covered[id] {
			t.Errorf("story %s is missing an e2e reference in docs/stories", id)
		}
	}
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func TestCIIncludesConformanceGates(t *testing.T) {
	r := newRunner(t)
	ciPath := filepath.Join(r.root, ".github", "workflows", "ci.yml")
	content, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("read ci workflow: %v", err)
	}
	text := string(content)
	for _, needle := range []string{
		"make test",
		"make verify-no-ui-leak",
		// The ergo integration job is gated on a pinned server install, so it
		// is the one gate that can be dropped without any other test noticing.
		"make test-irc",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("ci workflow missing required gate %q", needle)
		}
	}
}
