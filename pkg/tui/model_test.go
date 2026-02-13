package tui

import (
	"testing"
	"time"

	"hop.top/wsm/pkg/workspace"
)

func TestRootNavigation(t *testing.T) {
	now := time.Now().UTC()
	model := RootModel{}
	model.SetItems([]workspace.Workspace{
		{ID: "w1", Name: "alpha", CreatedAt: now, UpdatedAt: now, Status: workspace.StatusActive},
		{ID: "w2", Name: "bravo", CreatedAt: now, UpdatedAt: now, Status: workspace.StatusActive},
		{ID: "w3", Name: "charlie", CreatedAt: now, UpdatedAt: now, Status: workspace.StatusArchived},
	})

	selected, ok := model.SelectedWorkspace()
	if !ok || selected.Name != "alpha" {
		t.Fatalf("expected initial selection alpha, got ok=%v ws=%+v", ok, selected)
	}

	if moved := model.MoveDown(); !moved {
		t.Fatalf("expected move-down to succeed")
	}
	selected, _ = model.SelectedWorkspace()
	if selected.Name != "bravo" {
		t.Fatalf("expected selection to move to bravo, got %s", selected.Name)
	}

	if moved := model.MoveDown(); !moved {
		t.Fatalf("expected move-down to succeed at second row")
	}
	if moved := model.MoveDown(); moved {
		t.Fatalf("expected move-down at bottom row to be ignored")
	}
	selected, _ = model.SelectedWorkspace()
	if selected.Name != "charlie" {
		t.Fatalf("expected selection to remain charlie at bottom, got %s", selected.Name)
	}

	if moved := model.MoveUp(); !moved {
		t.Fatalf("expected move-up to succeed")
	}
	selected, _ = model.SelectedWorkspace()
	if selected.Name != "bravo" {
		t.Fatalf("expected selection to return to bravo, got %s", selected.Name)
	}

	lines := model.ListLines()
	if len(lines) != 3 {
		t.Fatalf("expected list rendering for 3 items, got %d", len(lines))
	}
	if lines[1] == "" || lines[1][0:1] != ">" {
		t.Fatalf("expected selected row marker in rendered lines, got %q", lines[1])
	}
}
