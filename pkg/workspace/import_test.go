package workspace

import (
	"context"
	"testing"
)

func TestImportWorkspace_Basic(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	data := &ExportData{
		Version: "1",
		Workspace: WorkspaceExport{
			Name:        "imported-ws",
			Description: "from export",
			Tags:        []string{"imported"},
		},
	}

	result, err := m.ImportWorkspace(ctx, data, ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(result.Created) != 1 {
		t.Fatalf("expected 1 created, got %d", len(result.Created))
	}
	if result.Created[0] != "imported-ws" {
		t.Fatalf("expected name imported-ws, got %q", result.Created[0])
	}

	ws, err := m.ResolveWorkspaceRef(ctx, "imported-ws")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ws.Description != "from export" {
		t.Fatalf("expected description, got %q", ws.Description)
	}
}

func TestImportWorkspace_WithSpaces(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	data := &ExportData{
		Version:   "1",
		Workspace: WorkspaceExport{Name: "import-spaces"},
		Spaces: []SpaceExport{
			{
				Type:         "filesystem",
				Boundary:     "/tmp/project",
				Capabilities: []string{"oasf:cap:file.read"},
				TrustLevel:   "local",
			},
		},
	}

	result, err := m.ImportWorkspace(ctx, data, ImportOptions{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(result.Created) != 2 {
		t.Fatalf("expected 2 created (ws + space), got %d: %v",
			len(result.Created), result.Created)
	}
}

func TestImportWorkspace_DryRun(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	data := &ExportData{
		Version:   "1",
		Workspace: WorkspaceExport{Name: "dry-run-ws"},
		Spaces: []SpaceExport{
			{Type: "api", Boundary: "https://api.example.com", TrustLevel: "external"},
		},
	}

	result, err := m.ImportWorkspace(ctx, data, ImportOptions{DryRun: true})
	if err != nil {
		t.Fatalf("import dry-run: %v", err)
	}
	if len(result.Created) != 2 {
		t.Fatalf("expected 2 planned, got %d", len(result.Created))
	}

	// Workspace should NOT actually exist
	_, err = m.ResolveWorkspaceRef(ctx, "dry-run-ws")
	if !IsKind(err, ErrorKindNotFound) {
		t.Fatalf("expected not-found after dry run, got %v", err)
	}
}

func TestImportWorkspace_ConflictSkip(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	if _, err := m.Create(ctx, "existing-ws", CreateOptions{}); err != nil {
		t.Fatalf("create existing: %v", err)
	}

	data := &ExportData{
		Version:   "1",
		Workspace: WorkspaceExport{Name: "existing-ws"},
	}

	result, err := m.ImportWorkspace(ctx, data, ImportOptions{
		OnConflict: ConflictSkip,
	})
	if err != nil {
		t.Fatalf("import skip: %v", err)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped, got %d", len(result.Skipped))
	}
}

func TestImportWorkspace_ConflictRename(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	if _, err := m.Create(ctx, "rename-ws", CreateOptions{}); err != nil {
		t.Fatalf("create existing: %v", err)
	}

	data := &ExportData{
		Version:   "1",
		Workspace: WorkspaceExport{Name: "rename-ws"},
	}

	result, err := m.ImportWorkspace(ctx, data, ImportOptions{
		OnConflict: ConflictRename,
	})
	if err != nil {
		t.Fatalf("import rename: %v", err)
	}
	if len(result.Renamed) != 1 {
		t.Fatalf("expected 1 renamed, got %d", len(result.Renamed))
	}
	if result.Renamed[0] != "rename-ws-imported" {
		t.Fatalf("expected renamed name, got %q", result.Renamed[0])
	}
}

func TestImportWorkspace_ConflictOverwrite(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	if _, err := m.Create(ctx, "overwrite-ws", CreateOptions{
		Description: "old",
	}); err != nil {
		t.Fatalf("create existing: %v", err)
	}

	data := &ExportData{
		Version: "1",
		Workspace: WorkspaceExport{
			Name:        "overwrite-ws",
			Description: "new",
		},
	}

	result, err := m.ImportWorkspace(ctx, data, ImportOptions{
		OnConflict: ConflictOverwrite,
	})
	if err != nil {
		t.Fatalf("import overwrite: %v", err)
	}
	if len(result.Created) != 1 {
		t.Fatalf("expected 1 created, got %d", len(result.Created))
	}

	ws, err := m.ResolveWorkspaceRef(ctx, "overwrite-ws")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ws.Description != "new" {
		t.Fatalf("expected new description, got %q", ws.Description)
	}
}

func TestImportWorkspace_Prefix(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	data := &ExportData{
		Version:   "1",
		Workspace: WorkspaceExport{Name: "project"},
	}

	result, err := m.ImportWorkspace(ctx, data, ImportOptions{
		Prefix: "team-",
	})
	if err != nil {
		t.Fatalf("import prefix: %v", err)
	}
	if result.Created[0] != "team-project" {
		t.Fatalf("expected prefixed name, got %q", result.Created[0])
	}
}

func TestImportWorkspace_NilData(t *testing.T) {
	m := newTestManager(t)
	_, err := m.ImportWorkspace(context.Background(), nil, ImportOptions{})
	if err == nil {
		t.Fatal("expected error for nil data")
	}
}

func TestUnmarshalExportYAML(t *testing.T) {
	input := []byte(`version: "1"
workspace:
  name: test-ws
  description: hello
spaces:
  - type: filesystem
    boundary: /tmp
    trust_level: local
`)
	data, err := UnmarshalExportYAML(input)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Version != "1" {
		t.Fatalf("expected version 1, got %q", data.Version)
	}
	if data.Workspace.Name != "test-ws" {
		t.Fatalf("expected name test-ws, got %q", data.Workspace.Name)
	}
	if len(data.Spaces) != 1 {
		t.Fatalf("expected 1 space, got %d", len(data.Spaces))
	}
}

func TestUnmarshalExportYAML_Invalid(t *testing.T) {
	_, err := UnmarshalExportYAML([]byte(":::bad"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
