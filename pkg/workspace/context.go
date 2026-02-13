package workspace

import (
	"fmt"
	"strings"
	"time"

	"hop.top/wsm/pkg/model"
)

// ContextOptions controls context output formatting.
type ContextOptions struct {
	RecentEvents int
}

// FormatContext renders a snapshot as a context handoff brief.
func FormatContext(
	snap *model.Snapshot,
	events []model.Event,
	opts ContextOptions,
) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Workspace: %s\n", snap.Workspace.Name)
	fmt.Fprintf(&b, "# Snapshot at seq %d | %s\n\n",
		snap.AtSeqNum, snap.GeneratedAt.Format(time.RFC3339))

	fmt.Fprintf(&b, "## Summary\n")
	fmt.Fprintf(&b, "- %d messages\n", snap.MessageCount)
	fmt.Fprintf(&b, "- %d tool calls\n", snap.ToolCalls)
	fmt.Fprintf(&b, "- %d files changed\n", len(snap.FilesChanged))
	fmt.Fprintf(&b, "- %d devices\n", len(snap.Devices))

	if len(snap.FilesChanged) > 0 {
		fmt.Fprintf(&b, "\n## Files Changed\n")
		for _, f := range snap.FilesChanged {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}

	if len(events) > 0 {
		n := opts.RecentEvents
		if n <= 0 {
			n = 5
		}
		start := 0
		if len(events) > n {
			start = len(events) - n
		}
		recent := events[start:]
		fmt.Fprintf(&b, "\n## Last %d Events\n", len(recent))
		for _, ev := range recent {
			fmt.Fprintf(&b, "- [seq %d] %s -- %s\n",
				ev.SeqNum, ev.Type, ev.Timestamp.Format(time.RFC3339))
		}
	}

	return b.String()
}
