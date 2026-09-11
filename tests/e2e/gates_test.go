package e2e_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

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
		if strings.Contains(string(content), "tests/e2e/") {
			covered[id] = true
		}
	}

	for i := 1; i <= 26; i++ {
		id := ""
		if i < 10 {
			id = "00" + strconv.Itoa(i)
		} else {
			id = "0" + strconv.Itoa(i)
		}
		if !covered[id] {
			t.Fatalf("story %s is missing an e2e reference in docs/stories", id)
		}
	}
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
		// The ergo integration job is gated on a pinned server install, so it
		// is the one gate that can be dropped without any other test noticing.
		"make test-irc",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("ci workflow missing required gate %q", needle)
		}
	}
}
