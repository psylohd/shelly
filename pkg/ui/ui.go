package ui

import (
	"fmt"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/intox/shelly/pkg/config"
	"github.com/intox/shelly/pkg/session"
	"github.com/intox/shelly/pkg/toolbox"
)

type Mode int

const (
	ModeShelly Mode = iota
	ModeSession
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4"))

	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)

	sessionPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF5F87")).
				Bold(true)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")).
			Italic(true)

	highlightStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F1FA8C"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")).
			Bold(true)
)

type sessionMsg string
type StartListenerMsg struct {
	Host string
	Port int
}
type StatusMsg string
type NewSessionMsg struct {
	ID       int
	TargetIP string
}

type Model struct {
	Config          *config.Config
	Sessions        *session.SessionManager
	Toolbox         *toolbox.Toolbox
	Textinput       textinput.Model
	Viewport        viewport.Model
	Terminal        string
	Mode            Mode
	ActiveSessionID int
	Ready           bool
	Width           int
	Height          int

	// Metasploit-like options
	Options         map[string]string
	ListenerRunning bool

	// Callback to start listener from main
	OnStartListener func(string, int)
}

func NewModel(cfg *config.Config, sm *session.SessionManager, tb *toolbox.Toolbox, lhost string, lport int, httpport int, shell string) Model {
	ti := textinput.New()
	ti.Placeholder = "Type a command..."
	ti.Focus()
	ti.Prompt = promptStyle.Render("shelly> ")

	options := map[string]string{
		"LHOST":    lhost,
		"LPORT":    strconv.Itoa(lport),
		"HTTPPORT": strconv.Itoa(httpport),
		"SHELL":    shell,
	}

	return Model{
		Config:    cfg,
		Sessions:  sm,
		Toolbox:   tb,
		Textinput: ti,
		Mode:      ModeShelly,
		Options:   options,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
		cmds  []tea.Cmd
	)

	switch msg := msg.(type) {
	case sessionMsg:
		// Log first 100 chars of received data
		data := string(msg)
		if len(data) > 100 {
			log.Printf("[UI] Session data (%d bytes): %s...", len(data), data[:100])
		} else {
			log.Printf("[UI] Session data (%d bytes): %s", len(data), data)
		}
		m.Terminal += data

	case StatusMsg:
		m.Terminal += "\n" + string(msg)
		if strings.Contains(string(msg), "[!] Error") {
			m.ListenerRunning = false
		}

	case NewSessionMsg:
		log.Printf("[UI] New session %d from %s", msg.ID, msg.TargetIP)
		m.Terminal = fmt.Sprintf("[*] New session %d connected from %s\n", msg.ID, msg.TargetIP)
		m.ActiveSessionID = msg.ID
		m.Mode = ModeSession
		m.Textinput.Prompt = sessionPromptStyle.Render(fmt.Sprintf("session[%d]> ", msg.ID))

	case StartListenerMsg:
		log.Printf("[UI] StartListenerMsg received: host=%s port=%d", msg.Host, msg.Port)
		if m.OnStartListener != nil {
			m.OnStartListener(msg.Host, msg.Port)
		} else {
			log.Printf("[UI] OnStartListener is nil")
		}

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.Mode == ModeShelly {
				return m, tea.Quit
			}
			m.switchToShelly()

		case tea.KeyCtrlZ:
			if m.Mode == ModeSession {
				m.switchToShelly()
			}

		case tea.KeyEnter:
			input := m.Textinput.Value()
			if m.Mode == ModeShelly {
				if input != "" {
					m.Terminal += fmt.Sprintf("\n%s %s", promptStyle.Render("shelly>"), input)
					var cmd tea.Cmd
					m, cmd = m.handleShellyCommand(input)
					m.Textinput.SetValue("")
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
			} else {
				// ModeSession: check for escape sequence "!"
				if strings.HasPrefix(input, "!") {
					// One-shot shelly command
					m.Terminal += fmt.Sprintf("\n%s %s", promptStyle.Render("shelly [session-escape]>"), input[1:])
					var cmd tea.Cmd
					m, cmd = m.handleShellyCommand(input[1:])
					m.Textinput.SetValue("")
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				} else {
					// Regular session input
					if s, ok := m.Sessions.GetSession(m.ActiveSessionID); ok && s.RW != nil {
						log.Printf("[UI] Session %d input: %s", m.ActiveSessionID, input)
						fmt.Fprintf(s.RW, "%s\n", input)
						// Echo the command to terminal
						if input != "" {
							m.Terminal += fmt.Sprintf("\n%s %s", sessionPromptStyle.Render(fmt.Sprintf("session[%d]$ ", m.ActiveSessionID)), input)
						}
					}
					m.Textinput.SetValue("")
				}
			}
		}

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height

		headerHeight := 1
		footerHeight := 1
		verticalMarginHeight := headerHeight + footerHeight

		if !m.Ready {
			m.Viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.Viewport.SetContent(m.Terminal)
			m.Ready = true
		} else {
			m.Viewport.Width = msg.Width
			m.Viewport.Height = msg.Height - verticalMarginHeight
		}
	}

	m.Textinput, tiCmd = m.Textinput.Update(msg)
	m.Viewport, vpCmd = m.Viewport.Update(msg)
	m.Viewport.SetContent(m.Terminal)
	m.Viewport.GotoBottom()
	cmds = append(cmds, tiCmd, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) switchToShelly() {
	m.Mode = ModeShelly
	m.Textinput.Prompt = promptStyle.Render("shelly> ")
	m.Terminal += statusStyle.Render("\n[*] Returned to shelly prompt. Session still active in background.")
}

func (m Model) handleShellyCommand(input string) (Model, tea.Cmd) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return m, nil
	}

	switch strings.ToLower(parts[0]) {
	case "help":
		m.Terminal += "\n" + highlightStyle.Render("Core Commands:")
		m.Terminal += "\n  help              - Show this help"
		m.Terminal += "\n  sessions          - List active sessions"
		m.Terminal += "\n  use <id>          - Interact with session"
		m.Terminal += "\n  run               - Start the listener"
		m.Terminal += "\n  exit/quit         - Exit shelly"
		m.Terminal += "\n\n" + highlightStyle.Render("Configuration:")
		m.Terminal += "\n  options           - Show current configuration"
		m.Terminal += "\n  set <var> <val>   - Set a variable (LHOST, LPORT, SHELL, etc.)"
		m.Terminal += "\n\n" + highlightStyle.Render("Payloads & Tools:")
		m.Terminal += "\n  payloads          - Show payloads for current SHELL"
		m.Terminal += "\n  toolbox           - List available tools in toolbox"

	case "options":
		m.Terminal += "\n" + highlightStyle.Render("Current Options:")
		keys := make([]string, 0, len(m.Options))
		for k := range m.Options {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			m.Terminal += fmt.Sprintf("\n  %-10s : %s", k, m.Options[k])
		}

	case "set":
		if len(parts) < 3 {
			m.Terminal += "\nUsage: set <variable> <value>"
		} else {
			key := strings.ToUpper(parts[1])
			val := parts[2]
			m.Options[key] = val
			m.Terminal += fmt.Sprintf("\n%s => %s", key, val)
		}

	case "run":
		if m.ListenerRunning {
			m.Terminal += "\n[!] Listener is already running."
		} else {
			m.ListenerRunning = true
			m.Terminal += "\n[*] Starting listener..."
			log.Printf("[UI] Run command: LHOST=%s, LPORT=%s", m.Options["LHOST"], m.Options["LPORT"])
			port, err := strconv.Atoi(m.Options["LPORT"])
			if err != nil {
				log.Printf("[UI] Invalid LPORT: %v", err)
				m.Terminal += "\n[!] Invalid LPORT"
				break
			}
			return m, func() tea.Msg { return StartListenerMsg{Host: m.Options["LHOST"], Port: port} }
		}

	case "sessions":
		sessions := m.Sessions.ListSessions()
		if len(sessions) == 0 {
			m.Terminal += "\nNo active sessions."
		} else {
			m.Terminal += "\nID   Type     Target"
			for _, s := range sessions {
				m.Terminal += fmt.Sprintf("\n%-4d %-8s %s", s.ID, s.Type, s.TargetIP)
			}
		}

	case "toolbox":
		tools, err := m.Toolbox.List()
		if err != nil {
			m.Terminal += fmt.Sprintf("\nError listing toolbox: %v", err)
		} else if len(tools) == 0 {
			m.Terminal += "\nToolbox is empty."
		} else {
			m.Terminal += "\n" + highlightStyle.Render("Available tools in toolbox:")
			for _, t := range tools {
				m.Terminal += fmt.Sprintf("\n  - %s (http://%s:%s/%s)", t, m.Options["LHOST"], m.Options["HTTPPORT"], t)
			}
		}

	case "payloads":
		m.PrintPayloads()
	case "debug":
		m.Terminal += "\n" + highlightStyle.Render("Debug Options:")
		for k, v := range m.Options {
			m.Terminal += fmt.Sprintf("\n  %s = %s", k, v)
		}
		m.Terminal += fmt.Sprintf("\n  ListenerRunning = %t", m.ListenerRunning)
		m.Terminal += fmt.Sprintf("\n  ActiveSessionID = %d", m.ActiveSessionID)
		m.Terminal += fmt.Sprintf("\n  Mode = %v", m.Mode)

	case "use", "interact":
		if len(parts) < 2 {
			m.Terminal += "\nUsage: use <session_id>"
		} else {
			id, err := strconv.Atoi(parts[1])
			if err != nil {
				m.Terminal += "\nInvalid session ID"
			} else if s, ok := m.Sessions.GetSession(id); ok {
				m.ActiveSessionID = id
				m.Mode = ModeSession
				m.Textinput.Prompt = sessionPromptStyle.Render(fmt.Sprintf("session[%d]> ", id))
				m.Terminal += fmt.Sprintf("\n[*] Interacting with session %d (%s)", id, s.TargetIP)
			} else {
				m.Terminal += "\nSession not found"
			}
		}

	case "exit", "quit":
		return m, tea.Quit

	case "clear":
		m.Terminal = ""
		m.Viewport.SetContent(m.Terminal)
		m.Viewport.GotoBottom()

	default:
		m.Terminal += fmt.Sprintf("\nUnknown command: %s", parts[0])
	}
	return m, nil
}

func (m Model) View() string {
	if !m.Ready {
		return "\n  Initializing..."
	}

	modeStr := ""
	if m.Mode == ModeSession {
		modeStr = fmt.Sprintf("SESSION %d", m.ActiveSessionID)
	}

	status := "Stopped"
	if m.ListenerRunning {
		status = "Listening"
	}

	header := headerStyle.Render(fmt.Sprintf("%s v0.1.0 | %s | %s | %s:%s",
		titleStyle.Render(" SHELLY "), modeStr, status, m.Options["LHOST"], m.Options["LPORT"]))

	helpStr := "Ctrl+C: Quit • 'help' for commands"
	if m.Mode == ModeSession {
		target := "unknown"
		if s, ok := m.Sessions.GetSession(m.ActiveSessionID); ok {
			target = s.TargetIP
		}
		helpStr = fmt.Sprintf("Session %d (%s) • Ctrl+Z: Shelly • !<cmd>: Shelly command • Ctrl+C: Escape", m.ActiveSessionID, target)
	}

	footer := fmt.Sprintf("%s | %s",
		m.Textinput.View(),
		statusStyle.Render(helpStr))

	return fmt.Sprintf("%s\n%s\n%s", header, m.Viewport.View(), footer)
}

func (m *Model) PrintPayloads() {
	shellName := m.Options["SHELL"]
	if s, ok := m.Config.Shells[shellName]; ok {
		m.Terminal += "\n" + highlightStyle.Render(fmt.Sprintf("Payloads for %s (%s:%s):", shellName, m.Options["LHOST"], m.Options["LPORT"]))
		for _, t := range s.Templates {
			p := strings.ReplaceAll(t, "{ip}", m.Options["LHOST"])
			p = strings.ReplaceAll(p, "{port}", m.Options["LPORT"])
			p = strings.ReplaceAll(p, "{http_port}", m.Options["HTTPPORT"])
			m.Terminal += fmt.Sprintf("\n%s", p)
		}
	} else {
		m.Terminal += fmt.Sprintf("\nShell '%s' not found in config", shellName)
	}
}

func WaitForSessionData(id int, rw io.Reader, p *tea.Program) {
	buf := make([]byte, 1024)
	for {
		n, err := rw.Read(buf)
		if n > 0 {
			log.Printf("[DEBUG] Session %d read %d bytes", id, n)
			p.Send(sessionMsg(string(buf[:n])))
		}
		if err != nil {
			log.Printf("[DEBUG] Session %d read error: %v", id, err)
			p.Send(sessionMsg(fmt.Sprintf("\n[!] Session %d connection closed: %v", id, err)))
			return
		}
	}
}
