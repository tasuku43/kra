package cli

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type agentBoardUITickMsg struct{}

type agentBoardUILoadedMsg struct {
	records []agentRuntimeSessionRecord
	err     error
}

type agentBoardUIStopMsg struct {
	message string
	err     error
}

type agentBoardUIModel struct {
	root    string
	opts    agentBoardOptions
	records []agentRuntimeSessionRecord

	cursor int
	width  int
	height int

	message  string
	msgError bool
}

func (c *CLI) runAgentBoardUI(root string, opts agentBoardOptions, initial []agentRuntimeSessionRecord) int {
	if !isTerminalReader(c.In) || !isTerminalWriter(c.Out) {
		fmt.Fprintln(c.Err, "agent board --ui requires an interactive TTY")
		return exitUsage
	}

	model := agentBoardUIModel{
		root:    root,
		opts:    opts,
		records: sortAgentBoardSessions(initial),
		width:   100,
		height:  30,
	}
	program := tea.NewProgram(
		model,
		tea.WithInput(c.In),
		tea.WithOutput(c.Out),
		tea.WithoutSignalHandler(),
	)
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(c.Err, "run board ui: %v\n", err)
		return exitError
	}
	return exitOK
}

func (m agentBoardUIModel) Init() tea.Cmd {
	return agentBoardUITickCmd(time.Second)
}

func (m agentBoardUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		return m, nil
	case agentBoardUITickMsg:
		return m, tea.Batch(
			m.refreshCmd(),
			agentBoardUITickCmd(time.Second),
		)
	case agentBoardUILoadedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("refresh failed: %v", msg.err)
			m.msgError = true
			return m, nil
		}
		m.records = sortAgentBoardSessions(msg.records)
		if len(m.records) == 0 {
			m.cursor = 0
		} else if m.cursor >= len(m.records) {
			m.cursor = len(m.records) - 1
		}
		if !m.msgError {
			m.message = ""
		}
		return m, nil
	case agentBoardUIStopMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("stop failed: %v", msg.err)
			m.msgError = true
			return m, nil
		}
		m.message = msg.message
		m.msgError = false
		return m, m.refreshCmd()
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case tea.KeyDown:
			if m.cursor < len(m.records)-1 {
				m.cursor++
			}
			return m, nil
		case tea.KeyRunes:
			if len(msg.Runes) == 1 {
				switch strings.ToLower(string(msg.Runes[0])) {
				case "q":
					return m, tea.Quit
				case "k":
					if m.cursor > 0 {
						m.cursor--
					}
					return m, nil
				case "j":
					if m.cursor < len(m.records)-1 {
						m.cursor++
					}
					return m, nil
				case "r":
					return m, m.refreshCmd()
				case "s":
					if rec, ok := m.selected(); ok {
						return m, m.stopCmd(rec)
					}
					return m, nil
				case "i":
					m.message = "send prompt: use non-UI board action (agent board --act send)"
					m.msgError = false
					return m, nil
				}
			}
		}
	}
	return m, nil
}

func (m agentBoardUIModel) View() string {
	maxCols := m.width
	if maxCols <= 0 {
		maxCols = 100
	}
	header := "Agent Board UI  q:quit  ↑↓/j/k:move  s:stop  r:refresh"
	if writerSupportsColor(nil) {
		header = styleBold(header, true)
	}
	lines := []string{truncateDisplay(header, maxCols), ""}

	if len(m.records) == 0 {
		empty := "  (no sessions)"
		lines = append(lines, empty)
		if strings.TrimSpace(m.message) != "" {
			lines = append(lines, "")
			lines = append(lines, m.renderMessage(maxCols))
		}
		return strings.Join(lines, "\n")
	}

	leftWidth := maxCols / 2
	if leftWidth < 40 {
		leftWidth = 40
	}
	rightWidth := maxCols - leftWidth - 3
	if rightWidth < 20 {
		rightWidth = 20
	}

	left := m.renderLeft(leftWidth)
	right := m.renderRight(rightWidth)
	height := len(left)
	if len(right) > height {
		height = len(right)
	}
	for len(left) < height {
		left = append(left, "")
	}
	for len(right) < height {
		right = append(right, "")
	}

	for i := 0; i < height; i++ {
		l := truncateDisplay(left[i], leftWidth)
		padding := leftWidth - displayWidth(l)
		if padding < 0 {
			padding = 0
		}
		lines = append(lines, l+strings.Repeat(" ", padding)+" | "+truncateDisplay(right[i], rightWidth))
	}
	if strings.TrimSpace(m.message) != "" {
		lines = append(lines, "")
		lines = append(lines, m.renderMessage(maxCols))
	}
	return strings.Join(lines, "\n")
}

func (m agentBoardUIModel) renderMessage(maxCols int) string {
	msg := "  " + strings.TrimSpace(m.message)
	if m.msgError {
		return truncateDisplay(styleError(msg, true), maxCols)
	}
	return truncateDisplay(styleMuted(msg, true), maxCols)
}

func (m agentBoardUIModel) renderLeft(width int) []string {
	out := make([]string, 0, len(m.records)+1)
	out = append(out, "Sessions")
	for i, r := range m.records {
		marker := " "
		if i == m.cursor {
			marker = ">"
		}
		line := fmt.Sprintf("%s %s / %s  %s  %s", marker, r.WorkspaceID, locationLabel(r), r.Kind, displayRuntimeStateLabel(r.RuntimeState))
		if i == m.cursor {
			line = styleAccent(line, true)
		}
		out = append(out, truncateDisplay(line, width))
	}
	return out
}

func (m agentBoardUIModel) renderRight(width int) []string {
	rec, ok := m.selected()
	if !ok {
		return []string{"Details", "  (none)"}
	}
	return []string{
		"Details",
		"  workspace: " + rec.WorkspaceID,
		"  location:  " + locationLabel(rec),
		"  kind:      " + rec.Kind,
		"  state:     " + displayRuntimeStateLabel(rec.RuntimeState),
		"  updated:   " + formatRelativeAge(rec.UpdatedAt),
		"  session:   " + rec.SessionID,
	}
}

func (m agentBoardUIModel) selected() (agentRuntimeSessionRecord, bool) {
	if len(m.records) == 0 || m.cursor < 0 || m.cursor >= len(m.records) {
		return agentRuntimeSessionRecord{}, false
	}
	return m.records[m.cursor], true
}

func (m agentBoardUIModel) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		records, err := loadAgentRuntimeSessionsPreferBroker(m.root)
		if err != nil {
			return agentBoardUILoadedMsg{err: err}
		}
		records = filterAgentRuntimeSessions(records, agentRuntimeQueryOptions{
			workspaceID: m.opts.workspaceID,
			state:       m.opts.state,
			location:    m.opts.location,
			kind:        m.opts.kind,
			all:         m.opts.all,
		})
		return agentBoardUILoadedMsg{records: records}
	}
}

func (m agentBoardUIModel) stopCmd(record agentRuntimeSessionRecord) tea.Cmd {
	return func() tea.Msg {
		msg, err := stopAgentSessionFromBoard(m.root, record)
		return agentBoardUIStopMsg{message: msg, err: err}
	}
}

func agentBoardUITickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return agentBoardUITickMsg{}
	})
}

func stopAgentSessionFromBoard(root string, record agentRuntimeSessionRecord) (string, error) {
	if record.RuntimeState == "exited" {
		return fmt.Sprintf("agent already stopped: session=%s", record.SessionID), nil
	}
	stoppedByBroker := false
	if err := stopSessionWithAgentBroker(root, record.SessionID); err == nil {
		stoppedByBroker = true
	} else if err := terminateAgentPID(record.PID); err != nil {
		return "", err
	}
	if stoppedByBroker {
		if _, ok := waitAgentRuntimeSessionState(root, record.SessionID, "exited", 2*time.Second); ok {
			return fmt.Sprintf("agent stopped: session=%s", record.SessionID), nil
		}
	}
	record.RuntimeState = "exited"
	record.UpdatedAt = time.Now().Unix()
	record.Seq++
	record.ExitCode = nil
	if err := saveAgentRuntimeSession(record); err != nil {
		return "", err
	}
	return fmt.Sprintf("agent stopped: session=%s", record.SessionID), nil
}
