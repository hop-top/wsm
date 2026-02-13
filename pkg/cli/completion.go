package cli

import (
	"sort"

	"hop.top/wsm/pkg/workspace"
)

// WorkspaceCompletionValues returns stable completion candidates for workspace refs.
func WorkspaceCompletionValues(items []workspace.Workspace) []string {
	out := make([]string, 0, len(items)*2)
	for _, ws := range items {
		out = append(out, ws.Name)
		out = append(out, ws.ID)
	}
	sort.Strings(out)
	return out
}
