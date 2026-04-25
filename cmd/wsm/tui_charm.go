package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"hop.top/wsm/pkg/backend"
	uitui "hop.top/wsm/pkg/tui"
	"hop.top/wsm/pkg/workspace"
)

type loadedMsg struct {
	items []workspace.Workspace
	err   error
}

type detailLoadedMsg struct {
	workspaceID string
	detail      uitui.WorkspaceDetail
	err         error
}

type opDoneMsg struct {
	status string
	err    error
	reload bool
}

type tuiModel struct {
	ctx           context.Context
	manager       *workspace.Manager
	root          uitui.RootModel
	status        string
	detailLines   []string
	confirmDelete bool
	showHelp      bool
}

func newTUIModel(ctx context.Context, manager *workspace.Manager) tuiModel {
	return tuiModel{
		ctx:         ctx,
		manager:     manager,
		status:      "Loading workspaces...",
		detailLines: uitui.EmptyDetailLines(),
	}
}

func (m tuiModel) Init() tea.Cmd {
	return m.loadCmd()
}

func (m tuiModel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := m.manager.List(m.ctx, backend.ListOptions{SortBy: "updated", SortDir: "desc"})
		return loadedMsg{items: items, err: err}
	}
}

func (m tuiModel) loadDetailCmd(ws workspace.Workspace) tea.Cmd {
	return func() tea.Msg {
		detail, err := uitui.LoadWorkspaceDetail(m.ctx, m.manager, ws)
		return detailLoadedMsg{workspaceID: ws.ID, detail: detail, err: err}
	}
}

func (m tuiModel) opCmd(fn func() error, status string) tea.Cmd {
	return func() tea.Msg {
		err := fn()
		return opDoneMsg{status: status, err: err, reload: err == nil}
	}
}

func (m tuiModel) selectedWorkspace() (workspace.Workspace, bool) {
	return m.root.SelectedWorkspace()
}

func (m tuiModel) loadSelectedDetailCmd() tea.Cmd {
	ws, ok := m.selectedWorkspace()
	if !ok {
		return nil
	}
	return m.loadDetailCmd(ws)
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// v2 emits KeyPressMsg/KeyReleaseMsg; treat both as the legacy KeyMsg.
	switch msg := msg.(type) {
	case loadedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Load failed: %v", msg.err)
			return m, nil
		}
		m.root.SetItems(msg.items)
		if m.root.Count() == 0 {
			m.status = "No workspaces yet. Press c to create one."
			m.detailLines = uitui.EmptyDetailLines()
			return m, nil
		}
		m.status = fmt.Sprintf("Loaded %d workspace(s).", m.root.Count())
		return m, m.loadSelectedDetailCmd()
	case detailLoadedMsg:
		ws, ok := m.selectedWorkspace()
		if ok && msg.workspaceID != ws.ID {
			// Drop stale detail responses when selection changed before async load completed.
			return m, nil
		}
		if msg.err != nil {
			m.status = fmt.Sprintf("Detail load failed: %v", msg.err)
			m.detailLines = []string{
				"Detail unavailable.",
				"Try refresh (r) and inspect again.",
			}
			return m, nil
		}
		m.detailLines = msg.detail.DetailLines()
		return m, nil
	case opDoneMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Action failed: %v", msg.err)
			return m, nil
		}
		m.status = msg.status
		m.confirmDelete = false
		if msg.reload {
			return m, m.loadCmd()
		}
		return m, nil
	case tea.KeyPressMsg:
		s := msg.String()
		switch s {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "r":
			m.status = "Refreshing workspaces..."
			return m, m.loadCmd()
		case "down", "j":
			m.confirmDelete = false
			if moved := m.root.MoveDown(); moved {
				return m, m.loadSelectedDetailCmd()
			}
			return m, nil
		case "up", "k":
			m.confirmDelete = false
			if moved := m.root.MoveUp(); moved {
				return m, m.loadSelectedDetailCmd()
			}
			return m, nil
		case "c":
			name := "workspace-" + time.Now().UTC().Format("20060102-150405")
			return m, m.opCmd(func() error {
				_, err := uitui.CreateWorkspace(m.ctx, m.manager, name)
				return err
			}, "Created workspace "+name)
		case "a":
			ws, ok := m.selectedWorkspace()
			if !ok {
				return m, nil
			}
			verb := "Archived "
			if ws.Status == workspace.StatusArchived {
				verb = "Unarchived "
			}
			return m, m.opCmd(func() error {
				_, err := uitui.ToggleArchiveWorkspace(m.ctx, m.manager, ws)
				return err
			}, verb+ws.Name)
		case "d":
			ws, ok := m.selectedWorkspace()
			if !ok {
				return m, nil
			}
			if !m.confirmDelete {
				m.confirmDelete = true
				m.status = "Press d again to confirm delete: " + ws.Name
				return m, nil
			}
			return m, m.opCmd(func() error {
				return uitui.DeleteWorkspace(m.ctx, m.manager, ws)
			}, "Deleted "+ws.Name)
		case "enter":
			ws, ok := m.selectedWorkspace()
			if !ok {
				return m, nil
			}
			m.status = "Inspecting " + ws.Name
			return m, m.loadDetailCmd(ws)
		}
	}
	return m, nil
}

func (m tuiModel) View() tea.View {
	var b strings.Builder
	b.WriteString("wsm TUI\n")
	b.WriteString("--------\n")

	left := append([]string{"WORKSPACES", ""}, m.root.ListLines()...)
	rightBody := m.detailLines
	if len(rightBody) == 0 {
		rightBody = uitui.EmptyDetailLines()
	}
	right := append([]string{"DETAIL", ""}, rightBody...)
	b.WriteString(renderTwoPane(left, right))
	b.WriteString("\n")
	b.WriteString("Keys: j/k or arrows move  enter inspect  c create  a archive/unarchive  d delete  r refresh  ? help  q quit\n")
	if m.showHelp {
		b.WriteString("\nHelp: selection updates the detail pane. Actions persist through core manager APIs.\n")
	}
	if m.status != "" {
		b.WriteString("\nStatus: " + m.status + "\n")
	}
	return tea.NewView(b.String())
}

// viewString renders the model's View as a plain string (used by the
// scripted TUI runner for non-interactive smoke tests).
func (m tuiModel) viewString() string {
	v := m.View()
	return v.Content
}

func renderTwoPane(left, right []string) string {
	const leftWidth = 56
	maxLines := len(left)
	if len(right) > maxLines {
		maxLines = len(right)
	}
	var b strings.Builder
	for i := 0; i < maxLines; i++ {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		b.WriteString(padRight(truncate(l, leftWidth), leftWidth))
		b.WriteString(" | ")
		b.WriteString(r)
		b.WriteByte('\n')
	}
	return b.String()
}

func truncate(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func runTUI(ctx context.Context, app *appContext, _ globalOptions) error {
	if runtime.GOOS == "darwin" {
		if script := os.Getenv("WSM_TUI_SCRIPT"); script != "" {
			return runTUIScript(ctx, app.manager, script)
		}
	}
	p := tea.NewProgram(newTUIModel(ctx, app.manager))
	_, err := p.Run()
	return err
}

func runTUIScript(ctx context.Context, manager *workspace.Manager, script string) error {
	m := newTUIModel(ctx, manager)
	m = applyTUICmd(m, m.Init())

	for _, ch := range script {
		var key tea.KeyPressMsg
		if ch == '\n' {
			key = tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
		} else {
			key = tea.KeyPressMsg(tea.Key{Text: string(ch), Code: ch})
		}
		next, cmd := m.Update(key)
		typed, ok := next.(tuiModel)
		if !ok {
			return fmt.Errorf("unexpected tui model type")
		}
		m = applyTUICmd(typed, cmd)
		if ch == 'q' {
			break
		}
	}

	_, _ = fmt.Fprint(os.Stdout, m.viewString())
	return nil
}

func applyTUICmd(model tuiModel, cmd tea.Cmd) tuiModel {
	for cmd != nil {
		msg := cmd()
		next, nextCmd := model.Update(msg)
		typed, ok := next.(tuiModel)
		if !ok {
			return model
		}
		model = typed
		cmd = nextCmd
	}
	return model
}
