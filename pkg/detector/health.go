package detector

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// depFile maps a dependency file name to its ecosystem.
type depFile struct {
	name      string
	ecosystem string
}

var knownDepFiles = []depFile{
	{"go.mod", "go"},
	{"package.json", "npm"},
	{"requirements.txt", "pip"},
	{"Gemfile", "ruby"},
}

// NewDependencyHealthDetector detects dependency files and counts deps.
func NewDependencyHealthDetector() Detector {
	return &dependencyHealthDetector{}
}

type dependencyHealthDetector struct{}

func (d *dependencyHealthDetector) Name() string { return "dependency-health" }

func (d *dependencyHealthDetector) Detect(
	_ context.Context, spaceDir string,
) (*Result, error) {
	details := make(map[string]string)
	var found []string
	totalDeps := 0

	for _, df := range knownDepFiles {
		path := filepath.Join(spaceDir, df.name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		found = append(found, df.name)
		details["file_"+df.ecosystem] = df.name

		count := countDeps(path, df.ecosystem)
		details["deps_"+df.ecosystem] = fmt.Sprintf("%d", count)
		totalDeps += count
	}

	if len(found) == 0 {
		return &Result{
			Detected:   false,
			Kind:       "health",
			Confidence: 0.0,
		}, nil
	}

	details["total_files"] = fmt.Sprintf("%d", len(found))
	details["total_deps"] = fmt.Sprintf("%d", totalDeps)

	return &Result{
		Detected:   true,
		Kind:       "health",
		Details:    details,
		Confidence: 1.0,
	}, nil
}

// countDeps does a best-effort line count of dependencies.
func countDeps(path, ecosystem string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	switch ecosystem {
	case "go":
		return countGoModDeps(f)
	case "pip":
		return countLineDeps(f)
	case "ruby":
		return countGemfileDeps(f)
	default:
		// For npm/package.json we just count non-trivial lines as estimate.
		return countLineDeps(f)
	}
}

func countGoModDeps(f *os.File) int {
	scanner := bufio.NewScanner(f)
	count := 0
	inRequire := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "require (") {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}
		if inRequire && line != "" && !strings.HasPrefix(line, "//") {
			count++
		}
	}
	return count
}

func countLineDeps(f *os.File) int {
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			count++
		}
	}
	return count
}

func countGemfileDeps(f *os.File) int {
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "gem ") {
			count++
		}
	}
	return count
}
