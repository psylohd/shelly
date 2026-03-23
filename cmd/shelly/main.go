package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/intox/shelly/pkg/config"
	"github.com/intox/shelly/pkg/network"
	"github.com/intox/shelly/pkg/serve"
	"github.com/intox/shelly/pkg/session"
	"github.com/intox/shelly/pkg/shell"
	"github.com/intox/shelly/pkg/toolbox"
	"github.com/intox/shelly/pkg/ui"
)

func main() {
	// 1. Parse Command Line Flags
	lHostFlag := flag.String("i", "", "Interface IP to listen on")
	lPortFlag := flag.Int("p", 4444, "Port to listen on")
	shellFlag := flag.String("s", "bash", "Default shell type")
	flag.Parse()

	// Setup logging
	logFile, err := os.OpenFile("shelly.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("=== Shelly started with args: %v", os.Args)

	// 1.5 Check for elevation if needed
	if *lPortFlag < 1024 && os.Geteuid() != 0 {
		fmt.Printf("[!] Port %d is privileged. Attempting elevation...\n", *lPortFlag)
		exe, err := os.Executable()
		if err != nil {
			fmt.Printf("[!] Could not find executable: %v\n", err)
			os.Exit(1)
		}

		// Re-run with sudo
		cmd := exec.Command("sudo", append([]string{exe}, os.Args[1:]...)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Printf("[!] Elevation failed or cancelled: %v\n", err)
			os.Exit(1)
		}
		return // Exit the original non-root process
	}

	// 2. Load Config
	configPath := "shelly.json"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = "shelly-rs/shelly.json"
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 3. Selection or Automatic Configuration
	var lhost string
	isAuto := *lHostFlag != ""
	if isAuto {
		lhost = *lHostFlag
	} else {
		ifaces, err := network.GetInterfaces()
		if err != nil || len(ifaces) == 0 {
			fmt.Println("No suitable network interfaces found.")
			os.Exit(1)
		}

		fmt.Println("Select Listening Interface:")
		for i, iface := range ifaces {
			fmt.Printf("[%d] %s (%s)\n", i, iface.Name, iface.IP)
		}
		var choice int
		fmt.Print("Choice [0]: ")
		fmt.Scanln(&choice)
		if choice < 0 || choice >= len(ifaces) {
			choice = 0
		}
		lhost = ifaces[choice].IP
		log.Printf("[MAIN] Selected interface: %s", lhost)
	}

	// 4. Initialize Components
	tbDir := "toolbox_data"
	tb := toolbox.NewToolbox(tbDir, cfg)
	_ = tb.EnsureDir()

	httpPort := cfg.Shelly.DefaultHTTPSvr
	if httpPort == 0 {
		httpPort = 8080
	}
	srv := serve.NewServer(httpPort, tbDir)
	if err := srv.Start(); err != nil {
		fmt.Printf("Error starting HTTP server: %v\n", err)
		os.Exit(1)
	}
	defer srv.Stop()

	sm := session.NewSessionManager()

	// 5. Setup TUI Model
	m := ui.NewModel(cfg, sm, tb, lhost, *lPortFlag, httpPort, *shellFlag)

	// Bridge to allow UI to trigger listener start
	var program *tea.Program

	if isAuto {
		// Immediate run
		m.ListenerRunning = true
	} else {
		// Interactive setup
		m.Terminal += "\n[*] Interactive setup complete."
		m.Terminal += "\n[*] Options set based on your choice."
		m.Terminal += "\n[*] Use the 'run' command to start the listener."
		m.PrintPayloads()
	}

	program = tea.NewProgram(m, tea.WithAltScreen())

	// Set up listener start callback after program is created
	m.OnStartListener = func(host string, port int) {
		log.Printf("[MAIN] Starting listener on %s:%d", host, port)
		// Rollback to specific interface binding
		listener := shell.NewNetcatListener(host, port, sm)
		err := listener.Start(func(id int) {
			if s, ok := sm.GetSession(id); ok {
				program.Send(ui.NewSessionMsg{ID: id, TargetIP: s.TargetIP})
				go ui.WaitForSessionData(id, s.RW, program)
			}
		})
		if err != nil {
			log.Printf("[MAIN] Listener error: %v", err)
			program.Send(ui.StatusMsg(fmt.Sprintf("[!] Error starting listener: %v (Note: Ports < 1024 require sudo)", err)))
		} else {
			log.Printf("[MAIN] Listener started successfully")
			program.Send(ui.StatusMsg(fmt.Sprintf("[*] Listener successfully started on %s:%d", host, port)))
		}
	}

	if isAuto {
		go func() {
			m.OnStartListener(lhost, *lPortFlag)
		}()
	}

	// 6. Start Program
	log.Printf("Starting bubbletea program")
	if _, err := program.Run(); err != nil {
		log.Printf("Bubbletea program error: %v", err)
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
	log.Printf("Bubbletea program ended")
}
