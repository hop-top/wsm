package commands

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"hop.top/kit/go/console/cli"
	kittui "hop.top/kit/go/console/tui"
	"hop.top/kit/go/console/tui/styles"
	"hop.top/wsm/pkg/backend"
	uitui "hop.top/wsm/pkg/tui"
	"hop.top/wsm/pkg/workspace"
)

// detailPaneWidth bounds the left list column; right pane gets the rest.
const detailPaneWidth = 56

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

// workspaceItem renders a single workspace row for kit/tui's List.
type workspaceItem struct {
	ws       workspace.Workspace
	selected bool
	styles   styles.Styles
}

func (i workspaceItem) Render(width int) string {
	marker := "  "
	label := fmt.Sprintf("%s [%s] %s", i.ws.Name, i.ws.Status, shortID(i.ws.ID))
	if i.selected {
		marker = "> "
		label = i.styles.Accent.Render(label)
	}
	out := marker + label
	if width > 0 && lipgloss.Width(out) > width {
		out = truncate(out, width)
	}
	return out
}

type tuiModel struct {
	ctx           context.Context
	manager       *workspace.Manager
	root          uitui.RootModel
	items         []workspace.Workspace // mirror for kit/tui List rendering
	list          kittui.List
	styles        styles.Styles
	status        string
	detailLines   []string
	confirmDelete bool
	showHelp      bool
}

func newTUIModel(ctx context.Context, manager *workspace.Manager) tuiModel {
	return tuiModel{
		ctx:         ctx,
		manager:     manager,
		styles:      styles.NewStyles(defaultTheme()),
		list:        kittui.NewList(20),
		status:      "Loading workspaces...",
		detailLines: uitui.EmptyDetailLines(),
	}
}

// defaultTheme builds a Theme from the kit Neon palette so all kit/tui
// components share consistent styling without depending on cli.Root state.
func defaultTheme() cli.Theme {
	white := lipgloss.Color("#FFFFFF")
	muted := lipgloss.Color("#6B7280")
	return cli.Theme{
		Palette:   cli.Neon,
		Accent:    cli.Neon.Command,
		Secondary: cli.Neon.Flag,
		Muted:     muted,
		Error:     lipgloss.Color("#EF4444"),
		Success:   lipgloss.Color("#10B981"),
		Title:     lipgloss.NewStyle().Bold(true).Foreground(white),
		Subtle:    lipgloss.NewStyle().Foreground(muted),
		Bold:      lipgloss.NewStyle().Bold(true),
	}
}

func (m tuiModel) Init() tea.Cmd { return m.loadCmd() }

func (m tuiModel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := m.manager.List(m.ctx,
			backend.ListOptions{SortBy: "updated", SortDir: "desc"})
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

// rebuildList syncs the kit/tui List with the current items + selection.
func (m tuiModel) rebuildList() tuiModel {
	selectedID := ""
	if ws, ok := m.root.SelectedWorkspace(); ok {
		selectedID = ws.ID
	}
	items := make([]kittui.Item, 0, len(m.items))
	for _, w := range m.items {
		items = append(items, workspaceItem{
			ws: w, selected: w.ID == selectedID, styles: m.styles,
		})
	}
	m.list = m.list.SetItems(items)
	return m
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Load failed: %v", msg.err)
			return m, nil
		}
		m.items = msg.items
		m.root.SetItems(msg.items)
		m = m.rebuildList()
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
			// Drop stale detail responses when selection changed mid-load.
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
		return m.handleKey(msg)
	}
	return m, nil
}

func (m tuiModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
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
			m = m.rebuildList()
			return m, m.loadSelectedDetailCmd()
		}
		return m, nil
	case "up", "k":
		m.confirmDelete = false
		if moved := m.root.MoveUp(); moved {
			m = m.rebuildList()
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
	return m, nil
}

func (m tuiModel) View() tea.View {
	return tea.NewView(m.render())
}

// render builds the string view used by View() and the script harness.
func (m tuiModel) render() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("wsm TUI"))
	b.WriteString("\n--------\n")

	// Left pane: title + list (rendered via kit/tui List).
	leftHeader := m.styles.Bold.Render("WORKSPACES")
	var leftBody string
	if m.root.Count() == 0 {
		leftBody = strings.Join(m.styledEmptyList(), "\n")
	} else {
		// Resize list height to match items so the script harness sees
		// every row without requiring scroll input.
		l := m.list.SetHeight(maxInt(m.root.Count(), 1))
		leftBody = l.View(detailPaneWidth)
	}
	left := append([]string{leftHeader, ""}, strings.Split(leftBody, "\n")...)

	// Right pane: title + detail lines (or empty placeholder).
	rightBody := m.detailLines
	if len(rightBody) == 0 {
		rightBody = uitui.EmptyDetailLines()
	}
	right := append([]string{m.styles.Bold.Render("DETAIL"), ""}, rightBody...)

	b.WriteString(renderTwoPane(left, right))
	b.WriteString("\n")
	b.WriteString(m.styles.Muted.Render(
		"Keys: j/k or arrows move  enter inspect  c create  " +
			"a archive/unarchive  d delete  r refresh  ? help  q quit"))
	b.WriteByte('\n')
	if m.showHelp {
		b.WriteString("\nHelp: selection updates the detail pane. " +
			"Actions persist through core manager APIs.\n")
	}
	if m.status != "" {
		b.WriteString("\nStatus: ")
		b.WriteString(m.status)
		b.WriteByte('\n')
	}
	return b.String()
}

// styledEmptyList mirrors RootModel.ListLines empty-state copy, styled.
func (m tuiModel) styledEmptyList() []string {
	return []string{
		m.styles.Muted.Render("No workspaces yet."),
		m.styles.Muted.Render("Press c to create one."),
	}
}

func renderTwoPane(left, right []string) string {
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
		b.WriteString(padRight(truncate(l, detailPaneWidth), detailPaneWidth))
		b.WriteString(" | ")
		b.WriteString(r)
		b.WriteByte('\n')
	}
	return b.String()
}

func truncate(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

func padRight(s string, width int) string {
	visible := lipgloss.Width(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
		key := keyFromRune(ch)
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

	_, _ = fmt.Fprint(os.Stdout, m.render())
	return nil
}

// keyFromRune converts a script byte into a bubbletea/v2 KeyPressMsg.
// Bubbletea v2 dispatches on Key.Code/Text; rune keys carry both Text
// and a Code populated from the rune itself.
func keyFromRune(r rune) tea.KeyPressMsg {
	if r == '\n' {
		return tea.KeyPressMsg{Code: tea.KeyEnter, Text: "\n"}
	}
	return tea.KeyPressMsg{Code: r, Text: string(r)}
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
