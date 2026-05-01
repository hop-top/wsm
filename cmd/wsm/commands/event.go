package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/output"
	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/workspace"
)

// eventListRow is the table projection for `event list`. Pre-truncates
// device/session UUIDs via shortID for table mode (JSON/YAML use the full
// model.Event).
type eventListRow struct {
	Seq       uint64 `table:"SEQ,priority=9"       json:"seq"`
	Type      string `table:"TYPE,priority=8"      json:"type"`
	Timestamp string `table:"TIMESTAMP,priority=6" json:"timestamp"`
	Device    string `table:"DEVICE,priority=4"    json:"device"`
	Session   string `table:"SESSION,priority=3"   json:"session"`
}

func newEventGroupCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "event",
		Aliases: []string{"events", "ev", "e"},
		Short:   "Event operations",
	}
	cmd.AddCommand(
		newEventListCmd(opts),
		newEventAddCmd(opts),
		newEventShowCmd(opts),
	)
	return cmd
}

func newEventListCmd(opts *globalOptions) *cobra.Command {
	var types []string
	var afterSeq, beforeSeq uint64
	var limit int
	cmd := &cobra.Command{
		Use:     "list <workspace>",
		Aliases: []string{"ls", "all", "l"},
		Short:   "List workspace events with filtering",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				evTypes := make([]workspace.EventType, 0, len(types))
				for _, t := range types {
					tt := workspace.EventType(strings.TrimSpace(t))
					if tt == "" {
						continue
					}
					evTypes = append(evTypes, tt)
				}

				events, err := app.manager.Events(ctx, args[0], backend.EventListOptions{
					Types:        evTypes,
					AfterSeqNum:  afterSeq,
					BeforeSeqNum: beforeSeq,
					Limit:        limit,
				})
				if err != nil {
					return err
				}

				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, events)
				}

				if len(events) == 0 {
					return workspace.NewErrorUsage("No events found.")
				}

				rows := make([]eventListRow, len(events))
				for i, ev := range events {
					rows[i] = eventListRow{
						Seq:       ev.SeqNum,
						Type:      string(ev.Type),
						Timestamp: ev.Timestamp.Format("2006-01-02 15:04:05"),
						Device:    shortID(ev.DeviceID),
						Session:   shortID(ev.SessionID),
					}
				}
				return output.Render(os.Stdout, output.Table, rows)
			})
		},
	}
	cmd.Flags().StringSliceVarP(&types, "type", "t", nil, "Filter by event types (comma-separated)")
	cmd.Flags().Uint64Var(&afterSeq, "after-seq", 0, "Only events after this sequence number")
	cmd.Flags().Uint64Var(&beforeSeq, "before-seq", 0, "Only events before this sequence number")
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "Maximum number of events to list")
	return cmd
}

func newEventShowCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "show <workspace> <seq-or-id>",
		Aliases: []string{"get", "inspect", "info", "s"},
		Short:   "Show full event details including payload",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ev, err := app.manager.GetEvent(ctx, args[0], args[1])
				if err != nil {
					return err
				}

				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, ev)
				}

				fmt.Fprintf(os.Stdout, "Event %s (Seq: %d)\n", ev.ID, ev.SeqNum)
				fmt.Fprintf(os.Stdout, "  Workspace: %s\n", ev.WorkspaceID)
				fmt.Fprintf(os.Stdout, "  Type:      %s\n", ev.Type)
				fmt.Fprintf(os.Stdout, "  Timestamp: %s\n", ev.Timestamp.Format("2006-01-02 15:04:05.000"))
				if ev.DeviceID != "" {
					fmt.Fprintf(os.Stdout, "  Device:    %s\n", ev.DeviceID)
				}
				if ev.SessionID != "" {
					fmt.Fprintf(os.Stdout, "  Session:   %s\n", ev.SessionID)
				}
				if ev.AgentID != "" {
					fmt.Fprintf(os.Stdout, "  Agent:     %s\n", ev.AgentID)
				}

				fmt.Fprintln(os.Stdout, "\nPayload:")
				var pretty bytes.Buffer
				if err := json.Indent(&pretty, ev.Data, "  ", "  "); err == nil {
					fmt.Fprintf(os.Stdout, "  %s\n", pretty.String())
				} else {
					fmt.Fprintf(os.Stdout, "  %s\n", string(ev.Data))
				}
				return nil
			})
		},
	}
}

func newEventAddCmd(opts *globalOptions) *cobra.Command {
	var rawData string
	var role, content, contentFormat string
	var toolName, toolInput, toolOutput, toolStatus string
	var durationMS int64
	var path, operation, diff string
	var deviceID, deviceName string

	cmd := &cobra.Command{
		Use:     "add <workspace> <type>",
		Aliases: []string{"create", "a", "+"},
		Short:   "Append an event to a workspace",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				eventType := workspace.EventType(strings.TrimSpace(args[1]))
				if !isKnownEventType(eventType) {
					return workspace.NewErrorUsage("unknown event type: " + string(eventType))
				}
				if !isUserAppendableEventType(eventType) {
					return workspace.NewErrorUsage("event type " + string(eventType) + " is reserved for system lifecycle operations")
				}

				payload, err := buildEventAddPayload(eventType, eventAddPayloadInput{
					rawData:    rawData,
					role:       role,
					content:    content,
					format:     contentFormat,
					toolName:   toolName,
					toolInput:  toolInput,
					toolOutput: toolOutput,
					toolStatus: toolStatus,
					durationMS: durationMS,
					path:       path,
					operation:  operation,
					diff:       diff,
					deviceID:   deviceID,
					deviceName: deviceName,
				})
				if err != nil {
					return err
				}

				ev, err := app.manager.RecordEvent(ctx, args[0], eventType, payload)
				if err != nil {
					return err
				}
				if opts.Quiet {
					fmt.Fprintln(os.Stdout, ev.SeqNum)
					return nil
				}
				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, ev)
				}
				// §6.3: ack message (non-data) → stderr.
				fmt.Fprintf(os.Stderr, "Recorded %s seq=%d id=%s\n", ev.Type, ev.SeqNum, ev.ID)
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&rawData, "data", "", "Raw JSON event payload (overrides typed flags)")

	cmd.Flags().StringVar(&role, "role", "", "interaction.message: role (user|assistant|system)")
	cmd.Flags().StringVar(&content, "content", "", "interaction.message: content text")
	cmd.Flags().StringVar(&contentFormat, "content-format", "",
		"interaction.message: optional content format")

	cmd.Flags().StringVar(&toolName, "tool-name", "", "interaction.tool_*: tool name")
	cmd.Flags().StringVar(&toolInput, "input", "", "interaction.tool_*: tool input as JSON")
	cmd.Flags().StringVar(&toolOutput, "output", "", "interaction.tool_*: tool output as JSON")
	cmd.Flags().StringVar(&toolStatus, "status", "", "interaction.tool_*: status value")
	cmd.Flags().Int64Var(&durationMS, "duration-ms", 0, "interaction.tool_*: duration in milliseconds")

	cmd.Flags().StringVar(&path, "path", "", "mutation.file_patch: path")
	cmd.Flags().StringVar(&operation, "operation", "", "mutation.file_patch: operation (create|modify|delete)")
	cmd.Flags().StringVar(&diff, "diff", "", "mutation.file_patch: optional diff")

	cmd.Flags().StringVar(&deviceID, "device-id", "", "device.*: device identifier")
	cmd.Flags().StringVar(&deviceName, "device-name", "", "device.*: device name")

	return cmd
}

type eventAddPayloadInput struct {
	rawData    string
	role       string
	content    string
	format     string
	toolName   string
	toolInput  string
	toolOutput string
	toolStatus string
	durationMS int64
	path       string
	operation  string
	diff       string
	deviceID   string
	deviceName string
}

func buildEventAddPayload(eventType workspace.EventType, in eventAddPayloadInput) (any, error) {
	if strings.TrimSpace(in.rawData) != "" {
		var decoded any
		if err := json.Unmarshal([]byte(in.rawData), &decoded); err != nil {
			return nil, workspace.NewErrorUsage("--data must be valid JSON")
		}
		return decoded, nil
	}

	switch eventType {
	case workspace.EventInteractionMessage:
		if strings.TrimSpace(in.content) == "" {
			return nil, workspace.NewErrorUsage("interaction.message requires --content (or --data)")
		}
		role := strings.TrimSpace(in.role)
		if role == "" {
			role = "user"
		}
		return workspace.MessageData{
			Role:    role,
			Content: in.content,
			Format:  strings.TrimSpace(in.format),
		}, nil
	case workspace.EventInteractionToolCall, workspace.EventInteractionToolDone:
		if strings.TrimSpace(in.toolName) == "" {
			return nil, workspace.NewErrorUsage(string(eventType) + " requires --tool-name (or --data)")
		}
		input, _ := parseOptionalRawJSON("input", in.toolInput)
		output, _ := parseOptionalRawJSON("output", in.toolOutput)
		status := strings.TrimSpace(in.toolStatus)
		if status == "" {
			if eventType == workspace.EventInteractionToolCall {
				status = "started"
			} else {
				status = "completed"
			}
		}
		return workspace.ToolCallData{
			ToolName: strings.TrimSpace(in.toolName),
			Input:    input,
			Output:   output,
			Status:   status,
			Duration: in.durationMS,
		}, nil
	case workspace.EventMutationFilePatch:
		if strings.TrimSpace(in.path) == "" {
			return nil, workspace.NewErrorUsage("mutation.file_patch requires --path")
		}
		op := strings.TrimSpace(in.operation)
		if op == "" {
			op = "modify"
		}
		return workspace.FileChangeData{
			Path:      in.path,
			Operation: op,
			Diff:      in.diff,
		}, nil
	case workspace.EventMutationArtifact:
		if strings.TrimSpace(in.path) == "" {
			return nil, workspace.NewErrorUsage("mutation.artifact requires --path")
		}
		return workspace.ArtifactData{
			Path: in.path,
		}, nil
	case workspace.EventDeviceAttached, workspace.EventDeviceDetached:
		if strings.TrimSpace(in.deviceID) == "" {
			return nil, workspace.NewErrorUsage(string(eventType) + " requires --device-id")
		}
		return workspace.DeviceData{
			DeviceID:   in.deviceID,
			DeviceName: in.deviceName,
		}, nil
	default:
		return nil, workspace.NewErrorUsage("automated payload builder not implemented for " + string(eventType) + "; use --data")
	}
}

func parseOptionalRawJSON(field, value string) (json.RawMessage, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if !json.Valid([]byte(value)) {
		return nil, workspace.NewErrorUsage("--" + field + " must be valid JSON")
	}
	return json.RawMessage(value), nil
}

func isUserAppendableEventType(t workspace.EventType) bool {
	for _, allowed := range manualAppendableEventTypes() {
		if string(t) == allowed {
			return true
		}
	}
	return false
}

func manualAppendableEventTypes() []string {
	return []string{
		string(workspace.EventInteractionMessage),
		string(workspace.EventInteractionToolCall),
		string(workspace.EventInteractionToolDone),
		string(workspace.EventMutationFilePatch),
		string(workspace.EventMutationArtifact),
		string(workspace.EventDeviceAttached),
		string(workspace.EventDeviceDetached),
	}
}
