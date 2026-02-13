package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"hop.top/wsm/pkg/workspace"
)

// RootModel tracks selectable workspace state for TUI rendering.
type RootModel struct {
	items    []workspace.Workspace
	selected int
}

// SetItems replaces the list while preserving selection where possible.
func (m *RootModel) SetItems(items []workspace.Workspace) {
	selectedID := ""
	if ws, ok := m.SelectedWorkspace(); ok {
		selectedID = ws.ID
	}

	copied := make([]workspace.Workspace, len(items))
	copy(copied, items)
	m.items = copied

	if len(m.items) == 0 {
		m.selected = 0
		return
	}

	if selectedID != "" {
		for i := range m.items {
			if m.items[i].ID == selectedID {
				m.selected = i
				return
			}
		}
	}
	if m.selected >= len(m.items) {
		m.selected = len(m.items) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

// MoveDown advances selection by one row.
func (m *RootModel) MoveDown() bool {
	if len(m.items) == 0 || m.selected >= len(m.items)-1 {
		return false
	}
	m.selected++
	return true
}

// MoveUp moves selection one row up.
func (m *RootModel) MoveUp() bool {
	if len(m.items) == 0 || m.selected <= 0 {
		return false
	}
	m.selected--
	return true
}

// Count returns current workspace list length.
func (m RootModel) Count() int { return len(m.items) }

// SelectedWorkspace returns the currently selected workspace.
func (m RootModel) SelectedWorkspace() (workspace.Workspace, bool) {
	if len(m.items) == 0 || m.selected < 0 || m.selected >= len(m.items) {
		return workspace.Workspace{}, false
	}
	return m.items[m.selected], true
}

// ListLines renders list-pane lines with selection marker.
func (m RootModel) ListLines() []string {
	if len(m.items) == 0 {
		return []string{
			"No workspaces yet.",
			"Press c to create one.",
		}
	}
	lines := make([]string, 0, len(m.items))
	for i, ws := range m.items {
		marker := "  "
		if i == m.selected {
			marker = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s [%s] %s", marker, ws.Name, ws.Status, shortID(ws.ID)))
	}
	return lines
}

// WorkspaceDetail is the summary model shown in the detail pane.
type WorkspaceDetail struct {
	Workspace  workspace.Workspace
	EventCount uint64
	Snapshot   *workspace.Snapshot
}

// DetailLines returns a stable, human-readable detail summary.
func (d WorkspaceDetail) DetailLines() []string {
	lines := []string{
		"Name:      " + d.Workspace.Name,
		"ID:        " + d.Workspace.ID,
		"Status:    " + string(d.Workspace.Status),
		"Created:   " + formatTime(d.Workspace.CreatedAt),
		"Updated:   " + formatTime(d.Workspace.UpdatedAt),
		fmt.Sprintf("Events:    %d", d.EventCount),
	}
	if d.Workspace.ArchivedAt != nil {
		lines = append(lines, "Archived:  "+formatTime(*d.Workspace.ArchivedAt))
	}
	if d.Workspace.Description != "" {
		lines = append(lines, "Description: "+d.Workspace.Description)
	}
	if len(d.Workspace.Tags) > 0 {
		tags := append([]string(nil), d.Workspace.Tags...)
		sort.Strings(tags)
		lines = append(lines, "Tags:      "+strings.Join(tags, ", "))
	}
	if d.Snapshot == nil {
		lines = append(lines, "Snapshot:  unavailable")
		return lines
	}

	files := append([]string(nil), d.Snapshot.FilesChanged...)
	sort.Strings(files)
	devices := append([]string(nil), d.Snapshot.Devices...)
	sort.Strings(devices)

	lines = append(lines,
		fmt.Sprintf("Snapshot:  seq=%d", d.Snapshot.AtSeqNum),
		"Snap Time: "+formatTime(d.Snapshot.AtTimestamp),
		fmt.Sprintf("Messages:  %d", d.Snapshot.MessageCount),
		fmt.Sprintf("ToolCalls: %d", d.Snapshot.ToolCalls),
		fmt.Sprintf("Files:     %d", len(files)),
		fmt.Sprintf("Devices:   %d", len(devices)),
	)
	if len(files) > 0 {
		lines = append(lines, "File Paths: "+strings.Join(files, ", "))
	}
	if len(devices) > 0 {
		lines = append(lines, "Device IDs: "+strings.Join(devices, ", "))
	}
	return lines
}

// EmptyDetailLines is shown when no workspace is selected.
func EmptyDetailLines() []string {
	return []string{
		"No workspace selected.",
		"Navigate with j/k or arrow keys.",
		"Press c to create a workspace.",
	}
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12] + "..."
}

func formatTime(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	return ts.Format(time.RFC3339)
}
