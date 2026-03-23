use clap::Parser;
use std::io::{self, Write};
use std::sync::mpsc;
use std::thread;
mod helpers;
mod config;
mod shell;
mod serve;
mod session;

/// Simple Nc wrapper with revshell generation and session management
#[derive(Parser, Debug)]
#[command(version, about, long_about = None)]
struct Args {
    #[arg(short, long)]
    /// Listen Interface. Omit for interactive prompt
    l_host: Option<String>,

    /// Listening Port
    #[arg(short, long)]
    port: Option<u16>,

    /// Reverse shell type to list
    #[arg(default_value = "bash")]
    shell: String,

    /// Downloads reverse shell executables
    #[clap(long, short, action)]
    download: bool,
}

fn main() {
    let _ = config::ensure_exists();

    let args = Args::parse();
    let ip: String;
    let port: u16;

    if let Some(host) = &args.l_host {
        ip = host.clone();
    } else {
        ip = helpers::interface_selector();
    }

    if let Some(p) = args.port {
        port = p;
    } else {
        port = helpers::port_input().unwrap_or(4444);
    }

    helpers::cls();
    println!("ℹ️  {2} Revshells for {0}:{1}\n", ip, port, args.shell);

    // Load config once and clone it for use in threads
    let config = config::load_config();
    let config_clone = config.clone();

    let mut listener_type = String::new();
    let http_port = config_clone["shelly"]["default_http_svr"].as_u16().unwrap_or(8000);

    if config_clone.has_key("shells") {
        let shells = &config_clone["shells"];
        if shells.has_key(&args.shell) {
            let shell_obj = &shells[&args.shell];
            if shell_obj.has_key("listener"){
                listener_type = shell_obj["listener"].as_str().unwrap_or("netcat").to_string();
            }
            if shell_obj.has_key("serve"){
                let serve_files_vec: Vec<String> = shell_obj["serve"]
                    .members()
                    .filter_map(|v| v.as_str().map(|s| s.to_string()))
                    .collect();
                serve::build_from_config(&serve_files_vec, &config_clone, http_port);
            }
            if shell_obj.has_key("templates") && shell_obj["templates"].is_array() {
                for t in shell_obj["templates"].members() {
                    if let Some(s) = t.as_str() {
                        println!("{}", s.replace("{ip}", &ip).replace("{port}", &port.to_string()).replace("{http_port}", &http_port.to_string()));
                    }
                }
            } else {
                eprintln!("templates is missing or not an array for shell '{}'", args.shell);
            }
        } else {
            eprintln!("shell '{}' not found in config", args.shell);
        }
    }
            if shell_obj.has_key("serve"){
                let serve_files_vec: Vec<String> = shell_obj["serve"]
                    .members()
                    .filter_map(|v| v.as_str().map(|s| s.to_string()))
                    .collect();
                serve::build_from_config(&serve_files_vec, &config_clone, http_port);
            }
            if shell_obj.has_key("templates") && shell_obj["templates"].is_array() {
                for t in shell_obj["templates"].members() {
                    if let Some(s) = t.as_str() {
                        println!("{}", s.replace("{ip}", &ip).replace("{port}", &port.to_string()).replace("{http_port}", &http_port.to_string()));
                    }
                }
            } else {
                eprintln!("templates is missing or not an array for shell '{}'", shell);
            }
        } else {
            eprintln!("shell '{}' not found in config", shell);
        }
    }

    // Start the listener in a separate thread
    let (tx, rx) = mpsc::channel();
    let listener_thread = thread::spawn(move || {
        if listener_type.eq("socat_raw"){
            println!("\n Running socat in raw mode");
            let socat = shell::Socat::new(port);
            if let Err(e) = socat.run_with_callback(|session_id: usize| {
                let _ = tx.send(("session_created", session_id.to_string()));
            }) {
                eprintln!("socat error: {}", e);
            }
        } else {
            println!("\nℹ️  Running nc");
            let netcat = shell::Netcat::new(port, config_clone, &ip);
            if let Err(e) = netcat.run_with_callback(|session_id: usize| {
                let _ = tx.send(("session_created", session_id.to_string()));
            }) {
                eprintln!("netcat error: {}", e);
            }
        }
    });

    // Main interactive loop
    loop {
        print!("shelly> ");
        io::stdout().flush().unwrap();

        let mut input = String::new();
        if io::stdin().read_line(&mut input).unwrap() == 0 {
            break; // EOF
        }

        let input = input.trim();
        if input.is_empty() {
            continue;
        }

        match input {
            "sessions" => list_sessions(),
            cmd if cmd.starts_with("switch ") => {
                let parts: Vec<&str> = cmd.splitn(2, ' ').collect();
                if parts.len() == 2 {
                    if let Ok(id) = parts[1].parse::<usize>() {
                        switch_session(id);
                    } else {
                        println!("Invalid session ID");
                    }
                } else {
                    println!("Usage: switch <session_id>");
                }
            }
            cmd if cmd.starts_with("kill ") => {
                let parts: Vec<&str> = cmd.splitn(2, ' ').collect();
                if parts.len() == 2 {
                    if let Ok(id) = parts[1].parse::<usize>() {
                        kill_session(id);
                    } else {
                        println!("Invalid session ID");
                    }
                } else {
                    println!("Usage: kill <session_id>");
                }
            }
            "help" => print_help(),
            "quit" | "exit" => {
                println!("Goodbye!");
                break;
            }
            _ => {
                // Check for messages from listener thread
                while let Ok((msg_type, msg_content)) = rx.try_recv() {
                    if msg_type == "session_created" {
                        if let Ok(id) = msg_content.parse::<usize>() {
                            println!("[*] Session {} created", id);
                        }
                    }
                }
                
                // If not a command, treat as potential session input
                // This would need to be handled differently in a real implementation
                println!("Unknown command: {}", input);
                println!("Type 'help' for available commands");
            }
        }
    }

    // Wait for listener thread to finish
    let _ = listener_thread.join();
}

fn list_sessions() {
    let sessions = crate::session::SESSION_MANAGER.list_sessions();
    if sessions.is_empty() {
        println!("No active sessions");
        return;
    }

    println!("{:<4} {:<12} {:<20} {:<8}", "Id", "Type", "Target", "Status");
    println!("{}", "-".repeat(50));
    for session in sessions {
        let status = if session.is_active { "Active" } else { "Inactive" };
        println!("{:<4} {:<12} {:<20} {:<8}", 
                 session.id, 
                 session.session_type, 
                 format!("{}:{}", session.target_ip, session.listener_port),
                 status);
    }
}

fn switch_session(id: usize) {
    let session = crate::session::SESSION_MANAGER.get_session(id);
    match session {
        Some(s) if s.is_active => {
            println!("[*] Switching to session {}", id);
            // In a real implementation, we would switch the active session here
            // For now, we just acknowledge the switch
        }
        Some(_) => println!("[!] Session {} is inactive", id),
        None => println!("[!] Session {} not found", id),
    }
}

fn kill_session(id: usize) {
    let result = crate::session::SESSION_MANAGER.kill_session(id);
    if result {
        println!("[*] Session {} killed", id);
    } else {
        println!("[!] Failed to kill session {}", id);
    }
}

fn print_help() {
    println!("Available commands:");
    println!("  sessions          - List all active sessions");
    println!("  switch <id>       - Switch to session <id>");
    println!("  kill <id>         - Kill session <id>");
    println!("  help              - Show this help message");
    println!("  quit/exit         - Exit shelly");
    println!("");
    println!("Session commands (when in active session):");
    println!("  Ctrl+Z            - Background current session and return to shelly prompt");
}