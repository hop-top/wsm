// Package skill generates upgrade preamble fragments for AI agent skill files.
//
// The preamble is a markdown snippet injected at the top of a skill file so
// agents know how to self-upgrade the binary before executing a task.
package skill

import (
	"fmt"
	"strings"
)

// SnoozeLevel controls the auto-upgrade aggressiveness for agents.
type SnoozeLevel int

const (
	// SnoozeNever: agent upgrades automatically, never snoozes.
	SnoozeNever SnoozeLevel = iota
	// SnoozeOnce: agent upgrades unless the user snoozed in the last session.
	SnoozeOnce
	// SnoozeAlways: agent always asks before upgrading.
	SnoozeAlways
)

// PreambleOptions configure preamble generation.
type PreambleOptions struct {
	// BinaryName is the CLI being described (e.g. "tlc", "aps").
	BinaryName string
	// UpgradeCommand is the shell invocation for upgrade (default: "<BinaryName> upgrade").
	UpgradeCommand string
	// Snooze controls how aggressively the agent should upgrade.
	Snooze SnoozeLevel
	// WhatsNewSection appends a "What's New" block when provided.
	WhatsNewSection string
}

// Generate returns a markdown preamble fragment for embedding in a skill file.
//
// Example output (with SnoozeNever):
//
//	## Upgrade Preamble
//	Before running any task, ensure `tlc` is up to date:
//	  1. Run `tlc upgrade --auto` silently.
//	  2. If the binary was replaced, re-exec or inform the user to restart.
func Generate(opts PreambleOptions) string {
	if opts.UpgradeCommand == "" {
		opts.UpgradeCommand = opts.BinaryName + " upgrade"
	}

	var b strings.Builder

	b.WriteString("## Upgrade Preamble\n\n")
	b.WriteString(fmt.Sprintf(
		"Before running any task, ensure `%s` is up to date.\n\n", opts.BinaryName,
	))

	switch opts.Snooze {
	case SnoozeNever:
		b.WriteString(fmt.Sprintf(
			"1. Run `%s --auto` silently.\n", opts.UpgradeCommand,
		))
		b.WriteString("2. If upgraded, inform the user to restart before continuing.\n")

	case SnoozeOnce:
		b.WriteString(fmt.Sprintf(
			"1. Run `%s` — accept the upgrade if offered, snooze once if user declines.\n",
			opts.UpgradeCommand,
		))
		b.WriteString("2. Continue with the task regardless of upgrade outcome.\n")

	case SnoozeAlways:
		b.WriteString(fmt.Sprintf(
			"1. Check: `%s --check` and report if an update is available.\n",
			opts.UpgradeCommand,
		))
		b.WriteString("2. Do not install automatically; let the user decide.\n")
	}

	if opts.WhatsNewSection != "" {
		b.WriteString("\n### What's New\n\n")
		b.WriteString(opts.WhatsNewSection)
		b.WriteString("\n")
	}

	return b.String()
}

// InlineFlow returns a compact agent-readable upgrade flow for embedding
// inside running prompts (e.g., REPL hooks or tool-call preambles).
//
// Designed to be parsed by an LLM, not a human.
func InlineFlow(binaryName, upgradeCmd string, snooze SnoozeLevel) string {
	if upgradeCmd == "" {
		upgradeCmd = binaryName + " upgrade --auto"
	}
	switch snooze {
	case SnoozeNever:
		return fmt.Sprintf(
			"[upgrade] run `%s`; if exit 0 and binary changed, notify user to restart.", upgradeCmd,
		)
	case SnoozeOnce:
		return fmt.Sprintf(
			"[upgrade] run `%s`; on decline, snooze; continue task.", upgradeCmd,
		)
	default:
		return fmt.Sprintf(
			"[upgrade] check `%s --check`; report result; do not install.", upgradeCmd,
		)
	}
}
