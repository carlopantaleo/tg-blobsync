package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("7")).
			Background(lipgloss.Color("5")).
			Padding(0, 1)

	logStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(lipgloss.Color("8"))
)

type model struct {
	width  int
	height int

	// Upper area state
	interactiveContent string
	list               list.Model
	showList           bool
	textInput          textinput.Model
	showPrompt         bool
	promptLabel        string

	// Lower area state (logs)
	logs     []string
	viewport viewport.Model
	logMu    *sync.Mutex

	// Interaction state
	userInputChan chan interface{}
	responseChan  chan interface{}

	// Control
	quitting bool
}

func initialModel() model {
	vp := viewport.New(0, 0)
	return model{
		viewport:      vp,
		logs:          []string{},
		logMu:         &sync.Mutex{},
		list:          list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0),
		userInputChan: make(chan interface{}),
		responseChan:  make(chan interface{}),
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		logHeight := 10 // Fixed height for logs for now, or percentage
		if logHeight > m.height/3 {
			logHeight = m.height / 3
		}

		interactiveHeight := m.height - logHeight - 1
		m.list.SetSize(m.width, interactiveHeight)

		m.viewport.Width = m.width
		m.viewport.Height = logHeight
		m.viewport.SetContent(m.wrapLogs())
		m.viewport.GotoBottom()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			close(m.responseChan) // Signal blocking calls to stop
			return m, tea.Quit
		}

		if m.showList {
			if msg.String() == "enter" {
				m.responseChan <- m.list.SelectedItem()
				m.showList = false
				return m, nil
			}
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
		if m.showPrompt {
			if msg.String() == "enter" {
				m.responseChan <- m.textInput.Value()
				m.showPrompt = false
				return m, nil
			}
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}

		if m.quitting {
			return m, nil
		}

	case logMsg:
		m.logMu.Lock()
		cleanMsg := strings.TrimSpace(string(msg))
		if cleanMsg != "" {
			m.logs = append(m.logs, cleanMsg)
			if len(m.logs) > 1000 { // Keep last 1000 logs
				m.logs = m.logs[len(m.logs)-1000:]
			}
			m.viewport.SetContent(m.wrapLogs())
			m.viewport.GotoBottom()
		}
		m.logMu.Unlock()

	case updateContentMsg:
		m.interactiveContent = string(msg)
		m.showList = false
		m.showPrompt = false

	case showListMsg:
		m.list = msg.list
		m.list.SetSize(m.width, m.height-m.viewport.Height-1)
		m.list.KeyMap.Quit.SetEnabled(false) // Disable list's internal quit to handle it in our Update
		m.showList = true
		m.showPrompt = false

	case showPromptMsg:
		m.textInput = msg.input
		m.textInput.Focus()
		m.showPrompt = true
		m.showList = false
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	logHeight := m.viewport.Height
	interactiveHeight := m.height - logHeight - 1 // -1 for border

	var upperContent string
	if m.showList {
		upperContent = m.list.View()
	} else if m.showPrompt {
		upperContent = fmt.Sprintf("%s\n\n%s\n\n%s", m.interactiveContent, m.promptLabel, m.textInput.View())
	} else {
		// Wrap interactive content too if it's plain text
		upperContent = lipgloss.NewStyle().Width(m.width).Render(m.interactiveContent)
	}

	upperArea := lipgloss.NewStyle().
		Height(interactiveHeight).
		Width(m.width).
		Render(upperContent)

	lowerArea := logStyle.
		Width(m.width).
		Render(m.viewport.View())

	return lipgloss.JoinVertical(lipgloss.Left, upperArea, lowerArea)
}

func (m model) wrapLogs() string {
	if m.width <= 0 {
		return strings.Join(m.logs, "\n")
	}
	// Use lipgloss to wrap each log line
	style := lipgloss.NewStyle().Width(m.width)
	var wrapped []string
	for _, l := range m.logs {
		wrapped = append(wrapped, style.Render(l))
	}
	return strings.Join(wrapped, "\n")
}

type logMsg string
type updateContentMsg string
type showListMsg struct {
	list list.Model
}

type showPromptMsg struct {
	input textinput.Model
}

type listItem struct {
	title string
	desc  string
	value interface{}
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return i.desc }
func (i listItem) FilterValue() string { return i.title }

type TUIWriter struct {
	program *tea.Program
}

func (w *TUIWriter) Write(p []byte) (n int, err error) {
	if w.program != nil {
		lines := strings.Split(string(p), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				w.program.Send(logMsg(trimmed))
			}
		}
	}
	return len(p), nil
}
