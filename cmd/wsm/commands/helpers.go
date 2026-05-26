package commands

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"hop.top/wsm/pkg/workspace"
)

// forceDeprecationOnce gates the one-shot "--force is deprecated" hint to
// stderr per invocation. Package-level state: a single CLI invocation
// owns the binary, so global once is sufficient (tests spawn a fresh
// process via the e2e harness).
var forceDeprecationOnce sync.Once

// bridgeForceToConfirm wires legacy --force booleans on destructive
// commands to kit's global --confirm=yes flag. Called from each
// destructive command's PreRunE. When force is true, the function:
//
//  1. Emits a one-shot deprecation hint to stderr (CLI conv §6.3: hints
//     are non-data → stderr).
//  2. Sets the root-level --confirm persistent flag to "yes" so kit's
//     wrapPolicyRunE → gateConfirm accepts the invocation.
//
// When force is false, this is a no-op: kit's gate falls back to the
// resolved confirm mode (prompt on TTY, no on non-TTY) per §8.6.
//
// Idempotent: re-bridging is safe (Set just rewrites the same value).
func bridgeForceToConfirm(cmd *cobra.Command, force bool) error {
	if !force {
		return nil
	}
	// Find the persistent --confirm flag by walking up to the root.
	// kit's confirmFlag is on the root command via PersistentFlags.
	for c := cmd; c != nil; c = c.Parent() {
		if f := c.PersistentFlags().Lookup("confirm"); f != nil {
			if err := f.Value.Set("yes"); err != nil {
				return fmt.Errorf("bridge --force to --confirm=yes: %w", err)
			}
			f.Changed = true
			break
		}
	}
	forceDeprecationOnce.Do(func() {
		// §6.3: deprecation hint is non-data → stderr.
		fmt.Fprintln(cmd.ErrOrStderr(),
			"warning: --force is deprecated; pass --confirm=yes instead.")
	})
	return nil
}

// resetForceDeprecationForTest clears the one-shot deprecation gate so
// in-process tests can re-exercise the hint. Test-only helper.
func resetForceDeprecationForTest() {
	forceDeprecationOnce = sync.Once{}
}

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
