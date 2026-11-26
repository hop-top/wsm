package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hop.top/wsm/pkg/backend"
)

func TestOpenMemory_ReadsEmptyAndTouchesNoDisk(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	b, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer func() { _ = b.Close() }()
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	got, err := b.ListWorkspaces(context.Background(), backend.ListOptions{})
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty store, got %d workspaces", len(got))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, filepath.Base(e.Name()))
		}
		t.Fatalf("in-memory store wrote to disk: %v", names)
	}
}
