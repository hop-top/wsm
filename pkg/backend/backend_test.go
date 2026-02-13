package backend

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root with go.mod")
		}
		dir = parent
	}
}

func TestNoUIPresentationLeak(t *testing.T) {
	root := findRepoRoot(t)
	cmd := exec.Command("go", "list", "-deps", "./pkg/...")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, string(out))
	}
	if strings.Contains(string(out), "charmbracelet") {
		t.Fatalf("unexpected charmbracelet dependency in pkg/*:\n%s", string(out))
	}
}

func TestBackendInterface_NoLeakedTypes(t *testing.T) {
	TestNoUIPresentationLeak(t)
}
