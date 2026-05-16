package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/tasuku43/kra/internal/app/wstask"
)

type wsTaskTUIOptions struct {
	target      wsTaskTargetOptions
	todoOnly    bool
	includeDone bool
	noColor     bool
	refresh     time.Duration
}

type wsTaskTUIRow struct {
	WorkspaceID string
	TaskID      string
	Item        wstask.Item
	Y           int
}

type wsTaskTUITickMsg time.Time
type wsTaskTUIMode string

const (
	wsTaskTUIModeRead  wsTaskTUIMode = "read"
	wsTaskTUIModeWrite wsTaskTUIMode = "write"
)

type wsTaskTUIModel struct {
	root    string
	target  wsTaskTarget
	opts    wsTaskTUIOptions
	service *wstask.Service

	model   wstask.ViewModel
	rows    []wsTaskTUIRow
	cursor  int
	scroll  int
	width   int
	height  int
	message string
	err     error
	quit    bool
	mode    wsTaskTUIMode

	useColor bool
}

func (c *CLI) runWSTaskTUI(args []string) int {
	opts, err := parseWSTaskTUIOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printWSTaskTUIUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printWSTaskTUIUsage(c.Err)
		return exitUsage
	}

	root := ""
	target := wsTaskTarget{}
	if opts.target.useAll {
		resolvedRoot, code := c.resolveRootForRootCommand("human", "ws.task.tui")
		if code != exitOK {
			return code
		}
		root = resolvedRoot
	} else {
		resolvedTarget, resolvedRoot, _, code := c.resolveWSTaskTarget(opts.target, "tui", "human", false, "active")
		if code != exitOK {
			return code
		}
		target = resolvedTarget
		root = resolvedRoot
	}

	inFile, ok := c.In.(*os.File)
	if !ok || !isatty.IsTerminal(inFile.Fd()) {
		fmt.Fprintln(c.Err, "interactive task TUI requires a TTY")
		return exitError
	}
	useColor := writerSupportsColor(c.Out) && !opts.noColor
	err = runWSTaskTUI(inFile, c.Out, root, target, opts, useColor)
	if err != nil {
		fmt.Fprintf(c.Err, "task tui: %v\n", err)
		return exitError
	}
	return exitOK
}

func runWSTaskTUI(in *os.File, out io.Writer, root string, target wsTaskTarget, opts wsTaskTUIOptions, useColor bool) error {
	model := newWSTaskTUIModel(root, target, opts, useColor)
	program := tea.NewProgram(
		model,
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithoutSignalHandler(),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := program.Run()
	return err
}

func newWSTaskTUIModel(root string, target wsTaskTarget, opts wsTaskTUIOptions, useColor bool) wsTaskTUIModel {
	if opts.refresh <= 0 {
		opts.refresh = 2 * time.Second
	}
	m := wsTaskTUIModel{
		root:     root,
		target:   target,
		opts:     opts,
		service:  newWorkspaceTaskService(),
		width:    80,
		height:   24,
		mode:     wsTaskTUIModeRead,
		useColor: useColor,
	}
	m.reload("")
	return m
}

func (m wsTaskTUIModel) Init() tea.Cmd {
	return wsTaskTUITick(m.opts.refresh)
}

func wsTaskTUITick(refresh time.Duration) tea.Cmd {
	return tea.Tick(refresh, func(t time.Time) tea.Msg {
		return wsTaskTUITickMsg(t)
	})
}

func (m wsTaskTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		return m, nil
	case wsTaskTUITickMsg:
		before := renderWSTaskViewModelKey(m.model)
		m.reload(m.message)
		after := renderWSTaskViewModelKey(m.model)
		if before == after {
			return m, wsTaskTUITick(m.opts.refresh)
		}
		return m, wsTaskTUITick(m.opts.refresh)
	case tea.MouseMsg:
		switch msg.Type {
		case tea.MouseWheelUp:
			m.scrollBy(-3)
			return m, nil
		case tea.MouseWheelDown:
			m.scrollBy(3)
			return m, nil
		}
		if msg.Type == tea.MouseLeft || msg.Type == tea.MouseRelease {
			idx := m.rowIndexAtY(msg.Y)
			if idx < 0 {
				return m, nil
			}
			m.cursor = idx
			if m.mode == wsTaskTUIModeWrite && msg.Type == tea.MouseLeft {
				return m.toggleRow(idx)
			}
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quit = true
			return m, tea.Quit
		case tea.KeyEsc:
			if m.mode == wsTaskTUIModeWrite {
				m.mode = wsTaskTUIModeRead
				m.message = "read mode"
				return m, nil
			}
			return m, nil
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
			m.ensureCursorVisible()
			return m, nil
		case tea.KeyDown:
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
			m.ensureCursorVisible()
			return m, nil
		case tea.KeyPgUp:
			m.scrollBy(-m.viewportHeight())
			return m, nil
		case tea.KeyPgDown:
			m.scrollBy(m.viewportHeight())
			return m, nil
		case tea.KeyHome:
			m.cursor = 0
			m.scroll = 0
			return m, nil
		case tea.KeyEnd:
			if len(m.rows) > 0 {
				m.cursor = len(m.rows) - 1
			}
			m.scroll = m.maxScroll()
			return m, nil
		case tea.KeyEnter, tea.KeySpace:
			if m.mode != wsTaskTUIModeWrite {
				m.message = "read mode: press i to edit"
				return m, nil
			}
			return m.toggleRow(m.cursor)
		}
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'q':
				m.quit = true
				return m, tea.Quit
			case 'i':
				m.mode = wsTaskTUIModeWrite
				m.message = "write mode"
				return m, nil
			case 'd':
				if m.mode != wsTaskTUIModeWrite {
					m.message = "read mode: press i to edit"
					return m, nil
				}
				return m.setRowStatus(m.cursor, wstask.StatusDone)
			case 't':
				if m.mode != wsTaskTUIModeWrite {
					m.message = "read mode: press i to edit"
					return m, nil
				}
				return m.setRowStatus(m.cursor, wstask.StatusTodo)
			case 'g':
				if m.mode != wsTaskTUIModeWrite {
					m.message = "read mode: press i to edit"
					return m, nil
				}
				return m.setRowStatus(m.cursor, wstask.StatusDoing)
			case 'b':
				if m.mode != wsTaskTUIModeWrite {
					m.message = "read mode: press i to edit"
					return m, nil
				}
				return m.setRowStatus(m.cursor, wstask.StatusBlocked)
			}
		}
	}
	return m, nil
}

func (m wsTaskTUIModel) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  %s\n\n", styleBold("TASKS", m.useColor), styleAccent(m.model.WorkspaceID, m.useColor), styleMuted("mode: "+string(m.mode), m.useColor))
	if m.err != nil {
		fmt.Fprintf(&b, "%s\n\n", styleError(fmt.Sprintf("error: %v", m.err), m.useColor))
	} else if m.model.Empty {
		fmt.Fprintf(&b, "%s\n\n", styleMuted("No structured tasks.", m.useColor))
	} else if len(m.model.Workspaces) > 0 {
		fullY := 2
		for _, workspace := range m.model.Workspaces {
			fullY++
			var line string
			if strings.TrimSpace(workspace.Title) == "" {
				line = styleAccent(workspace.ID, m.useColor)
			} else {
				line = fmt.Sprintf("%s  %s", styleAccent(workspace.ID, m.useColor), workspace.Title)
			}
			m.writeVisibleLine(&b, fullY, line)
			for _, row := range m.rows {
				if row.WorkspaceID != workspace.ID {
					continue
				}
				m.writeVisibleLine(&b, row.Y, m.renderRow(row))
				fullY = row.Y
			}
			fullY++
			m.writeVisibleLine(&b, fullY, "")
		}
	} else {
		for _, row := range m.rows {
			m.writeVisibleLine(&b, row.Y, m.renderRow(row))
		}
	}
	b.WriteByte('\n')
	if strings.TrimSpace(m.message) != "" {
		fmt.Fprintf(&b, "%s\n", styleMuted(m.message, m.useColor))
	}
	fmt.Fprintf(&b, "%s\n", styleMuted("source: "+m.model.Path, m.useColor))
	if m.mode == wsTaskTUIModeWrite {
		fmt.Fprintf(&b, "%s\n", styleMuted("write: wheel scroll  click/space toggles done  d/t/g/b set status  esc read  q quit", m.useColor))
	} else {
		fmt.Fprintf(&b, "%s\n", styleMuted("read: wheel scroll  click selects  i write  q quit", m.useColor))
	}
	return b.String()
}

func (m wsTaskTUIModel) writeVisibleLine(b *strings.Builder, fullY int, line string) {
	start := 2 + m.scroll
	end := start + m.viewportHeight()
	if fullY < start || fullY >= end {
		return
	}
	b.WriteString(line)
	b.WriteByte('\n')
}

func (m wsTaskTUIModel) renderRow(row wsTaskTUIRow) string {
	marker := " "
	if m.mode == wsTaskTUIModeWrite && m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].WorkspaceID == row.WorkspaceID && m.rows[m.cursor].TaskID == row.TaskID {
		marker = ">"
	}
	return fmt.Sprintf("%s %s %s  %s", styleAccent(marker, m.useColor), renderWSTaskStatusMarker(row.Item.Status, m.useColor), styleAccent(row.Item.ID, m.useColor), row.Item.Title)
}

func (m *wsTaskTUIModel) reload(message string) {
	model, err := buildWSTaskTUIModel(m.root, m.target, m.opts, m.service)
	if err != nil {
		m.err = err
		m.model = wstask.ViewModel{WorkspaceID: m.target.workspaceID, Path: m.root, Empty: true}
		m.rows = nil
		m.message = message
		m.cursor = 0
		return
	}
	m.err = nil
	m.model = model
	m.rows = buildWSTaskTUIRows(model)
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.scroll > m.maxScroll() {
		m.scroll = m.maxScroll()
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
	m.ensureCursorVisible()
	m.message = message
}

func buildWSTaskTUIModel(root string, target wsTaskTarget, opts wsTaskTUIOptions, service *wstask.Service) (wstask.ViewModel, error) {
	if opts.target.useAll {
		rows, err := listRowsFromFilesystem(context.Background(), root, "active", false)
		if err != nil {
			return wstask.ViewModel{}, err
		}
		model := wstask.ViewModel{
			WorkspaceID: "KRA_ROOT",
			Path:        root,
		}
		for _, row := range rows {
			result, err := service.View(root, row.ID, "active")
			if err != nil {
				return wstask.ViewModel{}, fmt.Errorf("%s: %w", row.ID, err)
			}
			workspace := wstask.ViewWorkspace{ID: row.ID, Title: row.Title}
			for _, item := range result.Items {
				if shouldSkipWSTaskTUIItem(item, opts) {
					continue
				}
				workspace.Items = append(workspace.Items, item)
			}
			if len(workspace.Items) > 0 {
				model.Workspaces = append(model.Workspaces, workspace)
				model.Items = append(model.Items, workspace.Items...)
			}
		}
		model.Empty = len(model.Items) == 0
		return model, nil
	}
	result, err := service.View(root, target.workspaceID, target.scope)
	if err != nil {
		return wstask.ViewModel{}, err
	}
	result.Items = filterWSTaskTUIItems(result.Items, opts)
	result.Empty = len(result.Items) == 0
	return result, nil
}

func buildWSTaskTUIRows(model wstask.ViewModel) []wsTaskTUIRow {
	rows := make([]wsTaskTUIRow, 0, len(model.Items))
	y := 2
	if len(model.Workspaces) > 0 {
		for _, workspace := range model.Workspaces {
			y++
			for _, item := range workspace.Items {
				rows = append(rows, wsTaskTUIRow{WorkspaceID: workspace.ID, TaskID: item.ID, Item: item, Y: y})
				y++
			}
			y++
		}
		return rows
	}
	for _, item := range model.Items {
		rows = append(rows, wsTaskTUIRow{WorkspaceID: model.WorkspaceID, TaskID: item.ID, Item: item, Y: y})
		y++
	}
	return rows
}

func filterWSTaskTUIItems(items []wstask.Item, opts wsTaskTUIOptions) []wstask.Item {
	out := make([]wstask.Item, 0, len(items))
	for _, item := range items {
		if shouldSkipWSTaskTUIItem(item, opts) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func shouldSkipWSTaskTUIItem(item wstask.Item, opts wsTaskTUIOptions) bool {
	return opts.todoOnly && !opts.includeDone && item.Status == wstask.StatusDone
}

func (m wsTaskTUIModel) rowIndexAtY(y int) int {
	fullY := y + m.scroll
	for i, row := range m.rows {
		if row.Y == fullY {
			return i
		}
	}
	return -1
}

func (m wsTaskTUIModel) viewportHeight() int {
	if m.height <= 0 {
		return 18
	}
	height := m.height - 6
	if strings.TrimSpace(m.message) != "" {
		height--
	}
	if height < 1 {
		return 1
	}
	return height
}

func (m wsTaskTUIModel) contentBottomY() int {
	bottom := 2
	for _, row := range m.rows {
		if row.Y > bottom {
			bottom = row.Y
		}
	}
	if len(m.model.Workspaces) > 0 {
		bottom += len(m.model.Workspaces) * 2
	}
	return bottom
}

func (m wsTaskTUIModel) maxScroll() int {
	maxScroll := m.contentBottomY() - 2 - m.viewportHeight() + 1
	if maxScroll < 0 {
		return 0
	}
	return maxScroll
}

func (m *wsTaskTUIModel) scrollBy(delta int) {
	m.scroll += delta
	if m.scroll < 0 {
		m.scroll = 0
	}
	if maxScroll := m.maxScroll(); m.scroll > maxScroll {
		m.scroll = maxScroll
	}
}

func (m *wsTaskTUIModel) ensureCursorVisible() {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	rowY := m.rows[m.cursor].Y
	top := 2 + m.scroll
	bottom := top + m.viewportHeight() - 1
	if rowY < top {
		m.scroll = rowY - 2
	}
	if rowY > bottom {
		m.scroll = rowY - 2 - m.viewportHeight() + 1
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
	if maxScroll := m.maxScroll(); m.scroll > maxScroll {
		m.scroll = maxScroll
	}
}

func (m wsTaskTUIModel) toggleRow(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.rows) {
		return m, nil
	}
	next := wstask.StatusDone
	if m.rows[idx].Item.Status == wstask.StatusDone {
		next = wstask.StatusTodo
	}
	return m.setRowStatus(idx, next)
}

func (m wsTaskTUIModel) setRowStatus(idx int, next wstask.Status) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.rows) {
		return m, nil
	}
	row := m.rows[idx]
	result, err := m.service.Status(m.root, row.WorkspaceID, row.TaskID, next)
	if err != nil {
		m.message = fmt.Sprintf("error: %v", err)
		return m, nil
	}
	m.reload(fmt.Sprintf("%s -> %s", result.Task.ID, result.Task.Status))
	return m, nil
}

func parseWSTaskTUIOptions(args []string) (wsTaskTUIOptions, error) {
	opts := wsTaskTUIOptions{refresh: 2 * time.Second}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := strings.TrimSpace(rest[0])
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return wsTaskTUIOptions{}, errHelpRequested
		case arg == "--current":
			opts.target.useCurrent = true
			rest = rest[1:]
		case arg == "--select":
			opts.target.useSelect = true
			rest = rest[1:]
		case arg == "--all":
			opts.target.useAll = true
			rest = rest[1:]
		case arg == "--todo-only":
			opts.todoOnly = true
			rest = rest[1:]
		case arg == "--include-done":
			opts.includeDone = true
			rest = rest[1:]
		case arg == "--no-color":
			opts.noColor = true
			rest = rest[1:]
		case arg == "--refresh":
			if len(rest) < 2 {
				return wsTaskTUIOptions{}, fmt.Errorf("--refresh requires a value")
			}
			refresh, err := parseWSTaskViewRefresh(rest[1])
			if err != nil {
				return wsTaskTUIOptions{}, err
			}
			opts.refresh = refresh
			rest = rest[2:]
		case arg == "--id":
			if len(rest) < 2 {
				return wsTaskTUIOptions{}, fmt.Errorf("--id requires a value")
			}
			opts.target.workspaceID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--id="):
			opts.target.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--id="))
			rest = rest[1:]
		case strings.HasPrefix(arg, "--refresh="):
			refresh, err := parseWSTaskViewRefresh(strings.TrimPrefix(arg, "--refresh="))
			if err != nil {
				return wsTaskTUIOptions{}, err
			}
			opts.refresh = refresh
			rest = rest[1:]
		case strings.HasPrefix(arg, "--current="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--current=")) != "" {
				return wsTaskTUIOptions{}, fmt.Errorf("--current does not take a value")
			}
			opts.target.useCurrent = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--select="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--select=")) != "" {
				return wsTaskTUIOptions{}, fmt.Errorf("--select does not take a value")
			}
			opts.target.useSelect = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--all="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--all=")) != "" {
				return wsTaskTUIOptions{}, fmt.Errorf("--all does not take a value")
			}
			opts.target.useAll = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--todo-only="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--todo-only=")) != "" {
				return wsTaskTUIOptions{}, fmt.Errorf("--todo-only does not take a value")
			}
			opts.todoOnly = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--include-done="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--include-done=")) != "" {
				return wsTaskTUIOptions{}, fmt.Errorf("--include-done does not take a value")
			}
			opts.includeDone = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--no-color="):
			if strings.TrimSpace(strings.TrimPrefix(arg, "--no-color=")) != "" {
				return wsTaskTUIOptions{}, fmt.Errorf("--no-color does not take a value")
			}
			opts.noColor = true
			rest = rest[1:]
		default:
			return wsTaskTUIOptions{}, fmt.Errorf("unknown flag for ws task tui: %q", arg)
		}
	}
	if len(rest) > 0 {
		return wsTaskTUIOptions{}, fmt.Errorf("unexpected args for ws task tui: %q", strings.Join(rest, " "))
	}
	if opts.target.useAll {
		if opts.target.workspaceID != "" {
			return wsTaskTUIOptions{}, fmt.Errorf("--id and --all cannot be used together")
		}
		if opts.target.useCurrent {
			return wsTaskTUIOptions{}, fmt.Errorf("--current and --all cannot be used together")
		}
		if opts.target.useSelect {
			return wsTaskTUIOptions{}, fmt.Errorf("--select and --all cannot be used together")
		}
		return opts, nil
	}
	if err := validateWSTaskTargetOptions(opts.target); err != nil {
		return wsTaskTUIOptions{}, err
	}
	return opts, nil
}
