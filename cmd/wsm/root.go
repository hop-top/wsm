package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"hop.top/wsm/pkg/backend"
	backendsqlite "hop.top/wsm/pkg/backend/sqlite"
	pkgcli "hop.top/wsm/pkg/cli"
	"hop.top/wsm/pkg/config"
	"hop.top/wsm/pkg/workspace"
)

type globalOptions struct {
	Backend    string
	DataDir    string
	BackendURL string
	JSON       bool
	Quiet      bool
	UI         string
	SessionID  string
	AgentID    string
}

type appContext struct {
	cfg     config.Config
	backend backend.Backend
	manager *workspace.Manager
}

func (a *appContext) Close() error {
	if a.backend != nil {
		return a.backend.Close()
	}
	return nil
}

func newAppContext(opts globalOptions) (*appContext, error) {
	cfg, err := config.Load(config.LoadOptions{
		FlagBackend:    opts.Backend,
		FlagDataDir:    opts.DataDir,
		FlagBackendURL: opts.BackendURL,
	})
	if err != nil {
		return nil, workspace.NewErrorBackend("load config", err)
	}

	if cfg.Backend == "" {
		cfg.Backend = "sqlite"
	}
	if cfg.Backend != "sqlite" {
		return nil, workspace.NewErrorUsage("only sqlite backend is currently implemented")
	}

	dbPath := filepath.Join(cfg.DataDir, "wsm.db")
	b, err := backendsqlite.Open(dbPath)
	if err != nil {
		return nil, workspace.NewErrorBackend("open sqlite backend", err)
	}
	if err := b.Migrate(context.Background()); err != nil {
		_ = b.Close()
		return nil, workspace.NewErrorBackend("run sqlite migrations", err)
	}
	mgr, err := workspace.NewManager(b,
		workspace.WithSessionID(opts.SessionID),
		workspace.WithAgentID(opts.AgentID),
	)
	if err != nil {
		_ = b.Close()
		return nil, workspace.NewErrorBackend("create manager", err)
	}

	return &appContext{cfg: cfg, backend: b, manager: mgr}, nil
}

func stdoutIsTTY() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func noColor() bool {
	v := strings.TrimSpace(os.Getenv("NO_COLOR"))
	return v != ""
}

func shouldJSON(opts globalOptions) bool {
	if opts.JSON {
		return true
	}
	// Pipe-friendly behavior: create defaults to JSON in non-TTY,
	// others default to plain unless explicitly requested.
	return false
}

func validateUIMode(opts globalOptions) error {
	mode := opts.UI
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "auto", "plain", "charm":
	default:
		return workspace.NewErrorUsage("--ui must be one of: auto, plain, charm")
	}
	if mode == "charm" && !isCharmUIBuild() {
		return workspace.NewErrorUsage("--ui=charm requested but this build lacks charmui support; rebuild with -tags charmui -modfile=go.charm.mod")
	}
	return nil
}

func newRootCmd() *cobra.Command {
	opts := globalOptions{UI: "auto"}
	root := &cobra.Command{
		Use:           "wsm",
		Short:         "Workspace & Session Manager",
		Long:          "Workspace & Session Manager (wsm) for IDE-agnostic session storage.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return validateUIMode(opts)
		},
	}

	root.PersistentFlags().StringVar(&opts.Backend, "backend", "", "Backend: sqlite")
	root.PersistentFlags().StringVar(&opts.DataDir, "data-dir", "", "Data directory")
	root.PersistentFlags().StringVar(&opts.BackendURL, "backend-url", "", "Backend URL (reserved for future backends)")
	root.PersistentFlags().BoolVar(&opts.JSON, "json", false, "Output JSON")
	root.PersistentFlags().BoolVarP(&opts.Quiet, "quiet", "q", false, "Minimal output")
	root.PersistentFlags().StringVar(&opts.UI, "ui", "auto", "UI mode: auto|plain|charm")
	root.PersistentFlags().StringVar(&opts.SessionID, "session-id", "", "Session ID for event tracking")
	root.PersistentFlags().StringVar(&opts.AgentID, "agent-id", "user", "Agent ID for event tracking")

	_ = root.RegisterFlagCompletionFunc("ui", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"auto", "plain", "charm"}, cobra.ShellCompDirectiveNoFileComp
	})

	root.RunE = func(cmd *cobra.Command, _ []string) error {
		cmd.Printf("Workspace & Session Manager\n\n")
		cmd.Printf("Quick start:\n")
		cmd.Printf("  wsm create my-project\n")
		cmd.Printf("  wsm list\n")
		cmd.Printf("  wsm inspect <id-or-name>\n")
		cmd.Printf("\nRun 'wsm --help' for full command reference.\n")
		return nil
	}

	root.AddCommand(
		newCreateCmd(&opts),
		newListCmd(&opts),
		newInspectCmd(&opts),
		newEventsGroupCmd(&opts),
		newDevicesGroupCmd(&opts),
		newDeprecatedEventCmd(),
		newSnapshotGroupCmd(&opts),
		newArchiveCmd(&opts),
		newUnarchiveCmd(&opts),
		newDeleteCmd(&opts),
		newConfigCmd(&opts),
		newCompletionCmd(root, &opts),
		newTUICmd(&opts),
	)

	registerWorkspaceCompletion(root, &opts)
	return root
}

func withApp(opts *globalOptions, fn func(ctx context.Context, app *appContext) error) error {
	app, err := newAppContext(*opts)
	if err != nil {
		return err
	}
	defer func() { _ = app.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return fn(ctx, app)
}

func newCreateCmd(opts *globalOptions) *cobra.Command {
	var description string
	var tags []string
	cmd := &cobra.Command{
		Use:     "create <name>",
		Aliases: []string{"new", "c", "+"},
		Short:   "Create a workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.Create(ctx, args[0], workspace.CreateOptions{Description: description, Tags: tags})
				if err != nil {
					return err
				}
				if opts.Quiet {
					fmt.Fprintln(os.Stdout, ws.ID)
					return nil
				}

				asJSON := shouldJSON(*opts) || (!stdoutIsTTY() && !opts.JSON)
				if asJSON {
					payload := map[string]any{"id": ws.ID, "created_at": ws.CreatedAt}
					return writeJSON(os.Stdout, payload)
				}
				fmt.Fprintf(os.Stdout, "Created workspace %q (%s)\n", ws.Name, ws.ID)
				return nil
			})
		},
	}
	cmd.Flags().StringVarP(&description, "description", "d", "", "Workspace description")
	cmd.Flags().StringArrayVarP(&tags, "tag", "t", nil, "Tags (repeatable)")
	return cmd
}

func newListCmd(opts *globalOptions) *cobra.Command {
	var status, sortBy string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls", "all"},
		Short:   "List workspaces",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				sortDir := "desc"
				if strings.EqualFold(sortBy, "name") {
					sortDir = "asc"
				}
				listOpts := backend.ListOptions{SortBy: sortBy, SortDir: sortDir}
				if status != "" {
					s := workspace.Status(status)
					listOpts.Status = &s
				}
				items, err := app.manager.List(ctx, listOpts)
				if err != nil {
					return err
				}
				if len(items) == 0 {
					fmt.Fprintln(os.Stdout, "No workspaces yet.")
					fmt.Fprintln(os.Stdout)
					fmt.Fprintln(os.Stdout, "  Create your first workspace:")
					fmt.Fprintln(os.Stdout, "    wsm create my-project")
					return nil
				}

				if opts.Quiet {
					for _, ws := range items {
						fmt.Fprintln(os.Stdout, ws.ID)
					}
					return nil
				}
				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, items)
				}
				return printWorkspaceTable(ctx, app, items)
			})
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Filter by status: active|archived")
	cmd.Flags().StringVar(&sortBy, "sort", "created", "Sort by: created|updated|name")
	_ = cmd.RegisterFlagCompletionFunc("status", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"active", "archived"}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("sort", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"created", "updated", "name"}, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func printWorkspaceTable(ctx context.Context, app *appContext, items []workspace.Workspace) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tID\tSTATUS\tEVENTS\tLAST ACTIVITY")
	for _, ws := range items {
		count, err := app.manager.EventCount(ctx, ws.ID)
		if err != nil {
			count = 0
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", ws.Name, shortID(ws.ID), ws.Status, count, humanAgo(ws.UpdatedAt))
	}
	return tw.Flush()
}

func newInspectCmd(opts *globalOptions) *cobra.Command {
	var showEvents bool
	var showEventsAll bool
	var showDevices bool
	cmd := &cobra.Command{
		Use:     "inspect <workspace>",
		Aliases: []string{"info", "i"},
		Short:   "Inspect a workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
				}
				eventCount, _ := app.manager.EventCount(ctx, ws.ID)
				snap, _ := app.manager.Snapshot(ctx, ws.ID)

				if shouldJSON(*opts) {
					resp := map[string]any{"workspace": ws, "event_count": eventCount, "snapshot": snap}
					if showEvents || showEventsAll {
						evOpts := backend.EventListOptions{}
						if !showEventsAll {
							evOpts.Limit = 20
						}
						events, _ := app.manager.Events(ctx, ws.ID, evOpts)
						resp["events"] = events
					}
					return writeJSON(os.Stdout, resp)
				}

				fmt.Fprintf(os.Stdout, "Workspace: %s\n", ws.Name)
				fmt.Fprintf(os.Stdout, "ID:        %s\n", ws.ID)
				fmt.Fprintf(os.Stdout, "Status:    %s\n", ws.Status)
				fmt.Fprintf(os.Stdout, "Created:   %s\n", ws.CreatedAt.Format(time.RFC3339))
				fmt.Fprintf(os.Stdout, "Updated:   %s\n", ws.UpdatedAt.Format(time.RFC3339))
				fmt.Fprintf(os.Stdout, "Events:    %d\n", eventCount)
				if snap != nil {
					fmt.Fprintf(os.Stdout, "Devices:   %d\n", len(snap.Devices))
				}
				if ws.Description != "" {
					fmt.Fprintf(os.Stdout, "\nDescription: %s\n", ws.Description)
				}
				if len(ws.Tags) > 0 {
					fmt.Fprintf(os.Stdout, "Tags:        %s\n", strings.Join(ws.Tags, ", "))
				}
				if showDevices && snap != nil {
					fmt.Fprintln(os.Stdout, "\nDEVICES")
					for _, d := range snap.Devices {
						fmt.Fprintln(os.Stdout, " -", d)
					}
				}
				if showEvents || showEventsAll {
					evOpts := backend.EventListOptions{}
					if !showEventsAll {
						evOpts.Limit = 20
					}
					events, err := app.manager.Events(ctx, ws.ID, evOpts)
					if err != nil {
						return err
					}
					fmt.Fprintln(os.Stdout, "\nEVENTS")
					w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
					fmt.Fprintln(w, "SEQ\tTYPE\tDEVICE\tTIMESTAMP")
					for _, ev := range events {
						fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", ev.SeqNum, ev.Type, ev.DeviceID, ev.Timestamp.Format(time.RFC3339))
					}
					_ = w.Flush()
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&showEvents, "events", false, "Show recent events (last 20)")
	cmd.Flags().BoolVar(&showEventsAll, "events-all", false, "Show all events")
	cmd.Flags().BoolVar(&showDevices, "devices", false, "Show attached devices")
	return cmd
}

func newEventsGroupCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Event operations",
	}
	cmd.AddCommand(
		newEventsListCmd(opts),
		newEventsAddCmd(opts),
		newEventsShowCmd(opts),
	)
	return cmd
}

func newEventsListCmd(opts *globalOptions) *cobra.Command {
	var types []string
	var afterSeq, beforeSeq uint64
	var limit int
	cmd := &cobra.Command{
		Use:     "list <workspace>",
		Aliases: []string{"ls", "all"},
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
					if !isKnownEventType(tt) {
						return workspace.NewErrorUsage("unknown event type: " + string(tt))
					}
					evTypes = append(evTypes, tt)
				}
				if beforeSeq > 0 && afterSeq > 0 && beforeSeq <= afterSeq {
					return workspace.NewErrorUsage("--before-seq must be greater than --after-seq")
				}
				events, err := app.manager.Events(ctx, args[0], backend.EventListOptions{
					AfterSeqNum:  afterSeq,
					BeforeSeqNum: beforeSeq,
					Types:        evTypes,
					Limit:        limit,
				})
				if err != nil {
					return err
				}
				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, events)
				}
				if len(events) == 0 {
					fmt.Fprintln(os.Stdout, "No events matched.")
					return nil
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
				fmt.Fprintln(w, "SEQ\tTYPE\tDEVICE\tTIMESTAMP")
				for _, ev := range events {
					fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", ev.SeqNum, ev.Type, ev.DeviceID, ev.Timestamp.Format(time.RFC3339))
				}
				return w.Flush()
			})
		},
	}
	cmd.Flags().StringSliceVar(&types, "type", nil, "Filter by event type (repeatable/comma-separated)")
	cmd.Flags().Uint64Var(&afterSeq, "after-seq", 0, "Only events with seq_num greater than this")
	cmd.Flags().Uint64Var(&beforeSeq, "before-seq", 0, "Only events with seq_num less than this")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of events")
	return cmd
}

func newEventsShowCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show <workspace> <seq-or-id>",
		Aliases: []string{"get", "s"},
		Short:   "Show a single event by seq number or ID",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ev, err := app.manager.GetEvent(ctx, args[0], args[1])
				if err != nil {
					return err
				}
				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, ev)
				}
				fmt.Fprintf(os.Stdout, "Event:     %s\n", ev.ID)
				fmt.Fprintf(os.Stdout, "Type:      %s\n", ev.Type)
				fmt.Fprintf(os.Stdout, "Seq:       %d\n", ev.SeqNum)
				fmt.Fprintf(os.Stdout, "Device:    %s\n", ev.DeviceID)
				if ev.SessionID != "" {
					fmt.Fprintf(os.Stdout, "Session:   %s\n", ev.SessionID)
				}
				if ev.AgentID != "" {
					fmt.Fprintf(os.Stdout, "Agent:     %s\n", ev.AgentID)
				}
				fmt.Fprintf(os.Stdout, "Timestamp: %s\n", ev.Timestamp.Format(time.RFC3339))
				fmt.Fprintf(os.Stdout, "Data:      %s\n", string(ev.Data))
				return nil
			})
		},
	}
	return cmd
}

func newDevicesGroupCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devices",
		Short: "Device operations",
	}
	cmd.AddCommand(
		newDevicesListCmd(opts),
		newDevicesShowCmd(opts),
	)
	return cmd
}

func newDevicesListCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List known devices",
		Aliases: []string{"ls", "all"},
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				devs, err := app.manager.ListDevices(ctx)
				if err != nil {
					return err
				}
				if len(devs) == 0 {
					fmt.Fprintln(os.Stdout, "No devices registered yet.")
					return nil
				}
				if opts.Quiet {
					for _, d := range devs {
						fmt.Fprintln(os.Stdout, d.ID)
					}
					return nil
				}
				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, devs)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tNAME\tOS\tARCH\tLAST SEEN")
				for _, d := range devs {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
						shortID(d.ID), d.Name, d.OS, d.Arch, humanAgo(d.LastSeen))
				}
				return tw.Flush()
			})
		},
	}
	return cmd
}

func newDevicesShowCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show <device-id>",
		Short:   "Show device details",
		Aliases: []string{"get", "s"},
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				dev, err := app.manager.GetDevice(ctx, args[0])
				if err != nil {
					return err
				}
				sessions, _ := app.manager.DeviceSessions(ctx, dev.ID)
				if shouldJSON(*opts) {
					resp := map[string]any{
						"device":   dev,
						"sessions": sessions,
					}
					return writeJSON(os.Stdout, resp)
				}
				fmt.Fprintf(os.Stdout, "Device:     %s\n", dev.Name)
				fmt.Fprintf(os.Stdout, "ID:         %s\n", dev.ID)
				fmt.Fprintf(os.Stdout, "OS:         %s\n", dev.OS)
				fmt.Fprintf(os.Stdout, "Arch:       %s\n", dev.Arch)
				fmt.Fprintf(os.Stdout, "First Seen: %s\n", dev.FirstSeen.Format(time.RFC3339))
				fmt.Fprintf(os.Stdout, "Last Seen:  %s\n", dev.LastSeen.Format(time.RFC3339))
				if len(sessions) > 0 {
					fmt.Fprintln(os.Stdout, "\nSESSIONS")
					tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
					fmt.Fprintln(tw, "SESSION\tAGENT\tWORKSPACE\tEVENTS\tLAST ACTIVITY")
					for _, s := range sessions {
						fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
							shortID(s.SessionID), s.AgentID,
							s.WorkspaceName, s.EventCount,
							humanAgo(s.LastEventAt))
					}
					_ = tw.Flush()
				}
				return nil
			})
		},
	}
	return cmd
}

func newDeprecatedEventCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "event",
		Short:  "Deprecated: use 'wsm events' instead",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return workspace.NewErrorUsage("'wsm event' is deprecated; use 'wsm events add|list|show' instead")
		},
	}
	return cmd
}

func newEventsAddCmd(opts *globalOptions) *cobra.Command {
	var rawData string
	var role, content, format string
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
					format:     format,
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
				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, ev)
				}
				fmt.Fprintf(os.Stdout, "Recorded %s seq=%d id=%s\n", ev.Type, ev.SeqNum, ev.ID)
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&rawData, "data", "", "Raw JSON event payload (overrides typed flags)")

	cmd.Flags().StringVar(&role, "role", "", "message.added: role (user|assistant|system)")
	cmd.Flags().StringVar(&content, "content", "", "message.added: content text")
	cmd.Flags().StringVar(&format, "format", "", "message.added: optional format")

	cmd.Flags().StringVar(&toolName, "tool-name", "", "tool_call.*: tool name")
	cmd.Flags().StringVar(&toolInput, "input", "", "tool_call.*: tool input as JSON")
	cmd.Flags().StringVar(&toolOutput, "output", "", "tool_call.*: tool output as JSON")
	cmd.Flags().StringVar(&toolStatus, "status", "", "tool_call.*: status value")
	cmd.Flags().Int64Var(&durationMS, "duration-ms", 0, "tool_call.*: duration in milliseconds")

	cmd.Flags().StringVar(&path, "path", "", "file.changed: path")
	cmd.Flags().StringVar(&operation, "operation", "", "file.changed: operation (create|modify|delete)")
	cmd.Flags().StringVar(&diff, "diff", "", "file.changed: optional diff")

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
	case workspace.EventMessageAdded:
		if strings.TrimSpace(in.content) == "" {
			return nil, workspace.NewErrorUsage("message.added requires --content (or --data)")
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
	case workspace.EventToolCallStarted, workspace.EventToolCallCompleted:
		if strings.TrimSpace(in.toolName) == "" {
			return nil, workspace.NewErrorUsage(string(eventType) + " requires --tool-name (or --data)")
		}
		input, err := parseOptionalRawJSON("input", in.toolInput)
		if err != nil {
			return nil, err
		}
		output, err := parseOptionalRawJSON("output", in.toolOutput)
		if err != nil {
			return nil, err
		}
		status := strings.TrimSpace(in.toolStatus)
		if status == "" {
			if eventType == workspace.EventToolCallStarted {
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
	case workspace.EventFileChanged:
		if strings.TrimSpace(in.path) == "" {
			return nil, workspace.NewErrorUsage("file.changed requires --path (or --data)")
		}
		op := strings.TrimSpace(in.operation)
		if op == "" {
			op = "modify"
		}
		return workspace.FileChangeData{
			Path:      strings.TrimSpace(in.path),
			Operation: op,
			Diff:      in.diff,
		}, nil
	case workspace.EventDeviceAttached, workspace.EventDeviceDetached:
		if strings.TrimSpace(in.deviceID) == "" {
			return nil, workspace.NewErrorUsage(string(eventType) + " requires --device-id (or --data)")
		}
		return workspace.DeviceData{
			DeviceID:   strings.TrimSpace(in.deviceID),
			DeviceName: strings.TrimSpace(in.deviceName),
		}, nil
	default:
		return nil, workspace.NewErrorUsage("event type " + string(eventType) + " requires --data JSON payload")
	}
}

func parseOptionalRawJSON(field, value string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, workspace.NewErrorUsage("--" + field + " must be valid JSON")
	}
	return json.RawMessage(trimmed), nil
}

func isUserAppendableEventType(t workspace.EventType) bool {
	switch t {
	case workspace.EventMessageAdded,
		workspace.EventToolCallStarted,
		workspace.EventToolCallCompleted,
		workspace.EventFileChanged,
		workspace.EventDeviceAttached,
		workspace.EventDeviceDetached:
		return true
	default:
		return false
	}
}

func manualAppendableEventTypes() []string {
	return []string{
		string(workspace.EventMessageAdded),
		string(workspace.EventToolCallStarted),
		string(workspace.EventToolCallCompleted),
		string(workspace.EventFileChanged),
		string(workspace.EventDeviceAttached),
		string(workspace.EventDeviceDetached),
	}
}

func newSnapshotGroupCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "snapshot",
		Short:   "Snapshot operations",
		Aliases: []string{"snap"},
	}
	cmd.AddCommand(
		newSnapshotGenerateCmd(opts),
		newSnapshotShowCmd(opts),
		newSnapshotContextCmd(opts),
	)
	return cmd
}

func newSnapshotGenerateCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "generate [workspace]",
		Short:   "Generate and persist a workspace snapshot",
		Aliases: []string{"gen", "g"},
		Args:    cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				var ws workspace.Workspace
				if len(args) == 0 {
					items, err := app.manager.List(ctx, backend.ListOptions{
						SortBy: "updated", SortDir: "desc", Limit: 1,
					})
					if err != nil {
						return err
					}
					if len(items) == 0 {
						return workspace.NewErrorNotFound("no workspaces available")
					}
					ws = items[0]
				} else {
					resolved, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
					if err != nil {
						return err
					}
					ws = *resolved
				}

				snap, err := app.manager.Snapshot(ctx, ws.ID)
				if err != nil {
					return err
				}
				if opts.Quiet {
					fmt.Fprintln(os.Stdout, snap.AtSeqNum)
					return nil
				}
				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, snap)
				}
				fmt.Fprintf(os.Stdout, "Snapshot for %s (%s)\n", ws.Name, ws.ID)
				fmt.Fprintf(os.Stdout, "  at_seq_num:    %d\n", snap.AtSeqNum)
				fmt.Fprintf(os.Stdout, "  message_count: %d\n", snap.MessageCount)
				fmt.Fprintf(os.Stdout, "  tool_calls:    %d\n", snap.ToolCalls)
				fmt.Fprintf(os.Stdout, "  files_changed: %d\n", len(snap.FilesChanged))
				fmt.Fprintf(os.Stdout, "  devices:       %d\n", len(snap.Devices))
				fmt.Fprintf(os.Stdout, "  generated_at:  %s\n",
					snap.GeneratedAt.Format(time.RFC3339))
				return nil
			})
		},
	}
	return cmd
}

func newSnapshotShowCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "show <workspace>",
		Short:   "Show latest snapshot",
		Aliases: []string{"get", "s"},
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
				}
				snap, err := app.manager.Snapshot(ctx, ws.ID)
				if err != nil {
					return err
				}
				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, snap)
				}
				fmt.Fprintf(os.Stdout, "Snapshot for %s (%s)\n", ws.Name, ws.ID)
				fmt.Fprintf(os.Stdout, "  at_seq_num:    %d\n", snap.AtSeqNum)
				fmt.Fprintf(os.Stdout, "  message_count: %d\n", snap.MessageCount)
				fmt.Fprintf(os.Stdout, "  tool_calls:    %d\n", snap.ToolCalls)
				fmt.Fprintf(os.Stdout, "  files_changed: %d\n", len(snap.FilesChanged))
				fmt.Fprintf(os.Stdout, "  devices:       %d\n", len(snap.Devices))
				fmt.Fprintf(os.Stdout, "  generated_at:  %s\n",
					snap.GeneratedAt.Format(time.RFC3339))
				return nil
			})
		},
	}
}

func newSnapshotContextCmd(opts *globalOptions) *cobra.Command {
	var recentEvents int
	cmd := &cobra.Command{
		Use:     "context <workspace>",
		Short:   "Generate context handoff brief",
		Aliases: []string{"ctx"},
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
				}
				snap, err := app.manager.Snapshot(ctx, ws.ID)
				if err != nil {
					return err
				}
				events, _ := app.manager.Events(ctx, ws.ID,
					backend.EventListOptions{})
				out := workspace.FormatContext(snap, events,
					workspace.ContextOptions{RecentEvents: recentEvents})
				fmt.Fprint(os.Stdout, out)
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&recentEvents, "events", 5,
		"Number of recent events to include")
	return cmd
}

func newArchiveCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "archive <workspace>",
		Aliases: []string{"arch"},
		Short:   "Archive a workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
				}
				if err := app.manager.Archive(ctx, ws.ID); err != nil {
					return err
				}
				if opts.Quiet {
					return nil
				}
				if shouldJSON(*opts) || !stdoutIsTTY() {
					return writeJSON(os.Stdout, map[string]any{"id": ws.ID, "archived_at": time.Now().UTC()})
				}
				fmt.Fprintf(os.Stdout, "Archived workspace %q\n", ws.Name)
				return nil
			})
		},
	}
	return cmd
}

func newUnarchiveCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "unarchive <workspace>",
		Aliases: []string{"unarch"},
		Short:   "Unarchive a workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
				}
				if err := app.manager.Unarchive(ctx, ws.ID); err != nil {
					return err
				}
				if opts.Quiet {
					return nil
				}
				if shouldJSON(*opts) || !stdoutIsTTY() {
					return writeJSON(os.Stdout, map[string]any{"id": ws.ID, "status": "active"})
				}
				fmt.Fprintf(os.Stdout, "Unarchived workspace %q\n", ws.Name)
				return nil
			})
		},
	}
	return cmd
}

func newDeleteCmd(opts *globalOptions) *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:     "delete <workspace>",
		Aliases: []string{"rm", "del"},
		Short:   "Delete a workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				preview, err := app.manager.DeletePreview(ctx, args[0])
				if err != nil {
					return err
				}
				if dryRun {
					if shouldJSON(*opts) {
						return writeJSON(os.Stdout, map[string]any{"dry_run": true, "preview": preview})
					}
					fmt.Fprintf(os.Stdout, "Would delete workspace %q (%s):\n", preview.WorkspaceName, preview.WorkspaceID)
					fmt.Fprintf(os.Stdout, "  - %d events\n", preview.EventCount)
					fmt.Fprintf(os.Stdout, "  - %d snapshots\n", preview.SnapshotCount)
					return nil
				}

				if !force {
					// WSM_DELETE_CONFIRM is a non-interactive test hook used by E2E automation.
					line := ""
					if runtime.GOOS == "darwin" {
						line = strings.TrimSpace(strings.ToLower(os.Getenv("WSM_DELETE_CONFIRM")))
					}
					if !stdoutIsTTY() && line == "" {
						return workspace.NewErrorUsage("delete in non-TTY requires --force")
					}
					if line == "" {
						fmt.Fprintf(os.Stdout, "Delete workspace %q? [y/N]: ", preview.WorkspaceName)
						reader := bufio.NewReader(os.Stdin)
						line, _ = reader.ReadString('\n')
						line = strings.TrimSpace(strings.ToLower(line))
					}
					if line != "y" && line != "yes" {
						fmt.Fprintln(os.Stdout, "Aborted.")
						return nil
					}
				}

				if err := app.manager.Delete(ctx, preview.WorkspaceID); err != nil {
					return err
				}
				if opts.Quiet {
					return nil
				}
				if shouldJSON(*opts) || !stdoutIsTTY() {
					return writeJSON(os.Stdout, map[string]any{"id": preview.WorkspaceID, "deleted": true})
				}
				fmt.Fprintf(os.Stdout, "Deleted workspace %q\n", preview.WorkspaceName)
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview delete without removing data")
	return cmd
}

func newConfigCmd(opts *globalOptions) *cobra.Command {
	configCmd := &cobra.Command{Use: "config", Short: "Configuration commands"}
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show resolved configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.LoadOptions{FlagBackend: opts.Backend, FlagDataDir: opts.DataDir, FlagBackendURL: opts.BackendURL})
			if err != nil {
				return workspace.NewErrorBackend("load config", err)
			}
			if shouldJSON(*opts) {
				return writeJSON(os.Stdout, cfg)
			}
			fmt.Fprintf(os.Stdout, "backend:     %s\n", cfg.Backend)
			fmt.Fprintf(os.Stdout, "data_dir:    %s\n", cfg.DataDir)
			if cfg.ConfigFileFound {
				fmt.Fprintf(os.Stdout, "config_file: %s\n", cfg.ConfigFile)
			} else {
				fmt.Fprintf(os.Stdout, "config_file: %s (not found)\n", cfg.ConfigFile)
			}
			fmt.Fprintf(os.Stdout, "source:      %s\n", cfg.Source)
			return nil
		},
	}
	configCmd.AddCommand(showCmd)
	return configCmd
}

func newCompletionCmd(root *cobra.Command, _ *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion scripts",
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			shell := args[0]
			switch shell {
			case "bash":
				return root.GenBashCompletion(os.Stdout)
			case "zsh":
				return root.GenZshCompletion(os.Stdout)
			case "fish":
				return root.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return workspace.NewErrorUsage("unsupported shell")
			}
		},
	}
	return cmd
}

func newTUICmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tui",
		Aliases: []string{"ui"},
		Short:   "Launch terminal UI",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runTUI(ctx, app, *opts)
			})
		},
	}
	return cmd
}

func registerWorkspaceCompletion(root *cobra.Command, opts *globalOptions) {
	completer := func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		app, err := newAppContext(*opts)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		defer func() { _ = app.Close() }()
		items, err := app.manager.List(context.Background(), backend.ListOptions{SortBy: "name", SortDir: "asc"})
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return pkgcli.WorkspaceCompletionValues(items), cobra.ShellCompDirectiveNoFileComp
	}

	for _, name := range []string{"inspect", "archive", "unarchive", "delete"} {
		if c, _, err := root.Find([]string{name}); err == nil && c != nil {
			c.ValidArgsFunction = completer
		}
	}
	for _, sub := range []string{"generate", "show", "context"} {
		if c, _, err := root.Find([]string{"snapshot", sub}); err == nil && c != nil {
			c.ValidArgsFunction = completer
		}
	}
	for _, sub := range []string{"list", "show"} {
		if c, _, err := root.Find([]string{"events", sub}); err == nil && c != nil {
			c.ValidArgsFunction = completer
		}
	}
	if c, _, err := root.Find([]string{"events", "add"}); err == nil && c != nil {
		c.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completer(cmd, args, toComplete)
			}
			if len(args) == 1 {
				out := make([]string, 0, len(manualAppendableEventTypes()))
				for _, v := range manualAppendableEventTypes() {
					if strings.HasPrefix(v, toComplete) {
						out = append(out, v)
					}
				}
				return out, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}
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
		workspace.EventMessageAdded,
		workspace.EventToolCallStarted,
		workspace.EventToolCallCompleted,
		workspace.EventFileChanged,
		workspace.EventDeviceAttached,
		workspace.EventDeviceDetached,
		workspace.EventSnapshotTaken:
		return true
	default:
		return false
	}
}
