package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"

	"hop.top/wsm/pkg/workspace"
)

func stdoutIsTTY() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func noColor(opts globalOptions) bool {
	if opts.NoColor {
		return true
	}
	v := strings.TrimSpace(os.Getenv("NO_COLOR"))
	return v != ""
}

func shouldJSON(opts globalOptions) bool {
	if opts.JSON {
		return true
	}
	return false
}

func writeJSON(out *os.File, value any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12] + "..."
}

func humanAgo(ts time.Time) string {
	d := time.Since(ts)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d d ago", int(d.Hours()/24))
}

func isKnownEventType(t workspace.EventType) bool {
	switch t {
	case workspace.EventWorkspaceCreated,
		workspace.EventWorkspaceUpdated,
		workspace.EventWorkspaceArchived,
		workspace.EventWorkspaceUnarchived,
		workspace.EventWorkspaceDeleted,
		workspace.EventInteractionMessage,
		workspace.EventInteractionToolCall,
		workspace.EventInteractionToolDone,
		workspace.EventMutationFilePatch,
		workspace.EventDeviceAttached,
		workspace.EventDeviceDetached,
		workspace.EventCheckpointSnapshot,
		workspace.EventSnapshotCreated,
		workspace.EventSnapshotUpdated,
		workspace.EventSnapshotDeleted:
		return true
	default:
		return false
	}
}
