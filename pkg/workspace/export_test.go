package workspace

import (
	"context"
	"testing"
)

func TestExportWorkspace_Basic(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "export-test", CreateOptions{
		Description: "test workspace",
		Tags:        []string{"go", "cli"},
		Metadata:    map[string]string{"team": "alpha"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	data, err := m.ExportWorkspace(ctx, ws.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if data.Version != "1" {
		t.Fatalf("expected version 1, got %q", data.Version)
	}
	if data.Workspace.Name != "export-test" {
		t.Fatalf("expected name export-test, got %q", data.Workspace.Name)
	}
	if data.Workspace.Description != "test workspace" {
		t.Fatalf("expected description, got %q", data.Workspace.Description)
	}
	if len(data.Workspace.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(data.Workspace.Tags))
	}
	if data.Workspace.Metadata["team"] != "alpha" {
		t.Fatalf("expected metadata team=alpha, got %q", data.Workspace.Metadata["team"])
	}
}

func TestExportWorkspace_WithSpaces(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "export-spaces", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = m.CreateSpace(ctx, ws.ID, Space{
		Type:         SpaceFilesystem,
		Boundary:     "/home/user/project",
		Capabilities: []string{"oasf:cap:file.read", "oasf:cap:file.write"},
		TrustLevel:   TrustLocal,
	})
	if err != nil {
		t.Fatalf("create space: %v", err)
	}

	data, err := m.ExportWorkspace(ctx, ws.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(data.Spaces) != 1 {
		t.Fatalf("expected 1 space, got %d", len(data.Spaces))
	}
	sp := data.Spaces[0]
	if sp.Type != "filesystem" {
		t.Fatalf("expected type filesystem, got %q", sp.Type)
	}
	if sp.Boundary != "/home/user/project" {
		t.Fatalf("expected boundary, got %q", sp.Boundary)
	}
	if sp.TrustLevel != "local" {
		t.Fatalf("expected trust local, got %q", sp.TrustLevel)
	}
}

func TestExportWorkspace_NotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.ExportWorkspace(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent workspace")
	}
}

func TestMarshalExportYAML(t *testing.T) {
	data := &ExportData{
		Version: "1",
		Workspace: WorkspaceExport{
			Name:        "test",
			Description: "desc",
			Tags:        []string{"a"},
		},
		Spaces: []SpaceExport{
			{
				Type:       "filesystem",
				Boundary:   "/tmp",
				TrustLevel: "local",
			},
		},
	}
	b, err := MarshalExportYAML(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("expected non-empty YAML output")
	}
	// Verify round-trip
	rt, err := UnmarshalExportYAML(b)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rt.Workspace.Name != "test" {
		t.Fatalf("round-trip name mismatch: %q", rt.Workspace.Name)
	}
	if len(rt.Spaces) != 1 {
		t.Fatalf("round-trip spaces count: %d", len(rt.Spaces))
	}
}
