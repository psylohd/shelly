use std::fs;
use std::io::prelude::*;
use std::net::{TcpListener, TcpStream};
use std::path::{PathBuf};
use std::sync::Arc;
use std::thread;
use std::sync::mpsc::{self, Sender};
use dirs::home_dir;
use std::io::{self, Write};
use indicatif::{ProgressBar, ProgressStyle};
use reqwest::blocking::Client;
use std::time::Duration;
use std::fs::File;


#[derive(Clone)]
pub struct StaticServer {
    host: String,
    port: u16,
    file_map: Arc<Vec<(String, PathBuf)>>,
}

impl StaticServer {
    pub fn new<I, S, P>(host: &str, port: u16, files: I) -> Result<Self, String>
    where
        I: IntoIterator<Item = (S, P)>,
        S: Into<String>,
        P: Into<PathBuf>,
    {
        let cwd = std::env::current_dir()
            .map_err(|e| format!("failed to get current dir: {}", e))?
            .canonicalize()
            .map_err(|e| format!("failed to canonicalize cwd: {}", e))?;

        let mut map = Vec::new();
        for (name, p) in files {
            let name = name.into();
            if name.is_empty() || name.contains('/') || name.contains('\\') {
                return Err(format!("invalid public filename: {}", name));
            }
            let mut path = p.into();
            // canonicalize to get absolute path
            path = path
                .canonicalize()
                .map_err(|e| format!("failed to canonicalize {:?}: {}", path, e))?;

            // Must be a file
            if !path.is_file() {
                return Err(format!("path is not a file: {:?}", path));
            }

            map.push((name, path));
        }

        Ok(StaticServer {
            host: host.to_string(),
            port,
            file_map: Arc::new(map),
        })
    }

    /// Start serving. This call blocks the current thread and spawns a new thread
    /// per connection. After one file has been fully served successfully (200 OK),
    /// the server shuts down (oneshot).
    pub fn serve(&self) -> Result<(), String> {
        let endpoint = format!("{}:{}", self.host, self.port);
        let listener = TcpListener::bind(&endpoint)
            .map_err(|e| format!("failed to bind {}: {}", endpoint, e))?;
        println!("StaticServer listening on {}", endpoint);

        // channel to receive shutdown signal from worker thread
        let (tx, rx) = mpsc::channel::<()>();

        // set nonblocking so we can poll rx between accepts without blocking forever
        listener
            .set_nonblocking(true)
            .map_err(|e| format!("failed to set nonblocking: {}", e))?;

        loop {
            // check for any incoming connection
            match listener.accept() {
                Ok((stream, _addr)) => {
                    let file_map = self.file_map.clone();
                    let tx_clone: Sender<()> = tx.clone();
                    thread::spawn(move || {
                        // handle_connection returns true if it served a file (200 OK).
                        if let Ok(served) = handle_connection(stream, &file_map) {
                            if served {
                                // signal main thread to shutdown
                                let _ = tx_clone.send(());
                            }
                        }
                    });
                }
                Err(ref e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                    // No incoming right now; check if a worker signaled shutdown.
                    if rx.try_recv().is_ok() {
                        // shutdown requested after a successful serve
                        println!("Shutdown signal received; stopping server.");
                        break;
                    }
                    // small sleep to avoid busy loop
                    std::thread::sleep(std::time::Duration::from_millis(50));
                    continue;
                }
                Err(e) => {
                    return Err(format!("accept error: {}", e));
                }
            }

            // also check channel in case it was signaled while we processed an accept
            if rx.try_recv().is_ok() {
                println!("Shutdown signal received; stopping server.");
                break;
            }
        }




        Ok(())
    }
}

fn handle_connection(mut stream: TcpStream, file_map: &[(String, PathBuf)]) -> Result<bool, String> {
    let mut buffer = [0u8; 2048];
    let n = stream
        .read(&mut buffer)
        .map_err(|e| format!("failed to read from stream: {}", e))?;
    if n == 0 {
        return Ok(false);
    }
    let request_str = String::from_utf8_lossy(&buffer[..n]);
    let request_path = parse_request_path(&request_str);

    serve_requested_file(&request_path, &mut stream, file_map)
}

fn parse_request_path(request: &str) -> String {
    // HTTP request line is like: GET /path HTTP/1.1
    request
        .split_whitespace()
        .nth(1)
        .map(|s| s.to_string())
        .unwrap_or_else(|| "/".to_string())
}

/// Example: mapping ("logo.png", "/var/www/images/logo.png") => GET /logo.png serves that file.
/// Returns Ok(true) if a 200 response was written, Ok(false) otherwise.
fn serve_requested_file(
    request_path: &str,
    stream: &mut TcpStream,
    file_map: &[(String, PathBuf)],
) -> Result<bool, String> {
    // normalize incoming path: strip leading slash
    let normalized = if request_path == "/" {
        "".to_string()
    } else {
        request_path.trim_start_matches('/').to_string()
    };

    // find file: exact match with normalized. If "/" was requested, look for "index"
    let target = if normalized.is_empty() {
        file_map.iter().find(|(name, _)| name == "index")
    } else {
        file_map.iter().find(|(name, _)| name == &normalized)
    };

    if let Some((_, path)) = target {
        match fs::read(path) {
            Ok(bytes) => {
                let header = format!(
                    "HTTP/1.1 200 OK\r\nContent-Length: {}\r\n\r\n",
                    bytes.len()
                );
                let mut resp = Vec::with_capacity(header.len() + bytes.len());
                resp.extend_from_slice(header.as_bytes());
                resp.extend_from_slice(&bytes);
                stream
                    .write_all(&resp)
                    .map_err(|e| format!("write error: {}", e))?;
                stream.flush().map_err(|e| format!("flush error: {}", e))?;
                return Ok(true);
            }
            Err(_) => {
                let resp = http_404_response("404 Not Found.");
                stream
                    .write_all(resp.as_bytes())
                    .map_err(|e| format!("write error: {}", e))?;
                stream.flush().map_err(|e| format!("flush error: {}", e))?;
                return Ok(false);
            }
        }
    } else {
        let resp = http_404_response("404 Not Found.");
        stream
            .write_all(resp.as_bytes())
            .map_err(|e| format!("write error: {}", e))?;
        stream.flush().map_err(|e| format!("flush error: {}", e))?;
        Ok(false)
    }
}

fn http_404_response(body: &str) -> String {
    format!(
        "HTTP/1.1 404 NOT FOUND\r\nContent-Length: {}\r\n\r\n{}",
        body.len(),
        body
    )
}

pub fn build_from_config(
    serve_files: &[String],
    config: &json::JsonValue,
    port: u16,
) {
    let home = home_dir().expect("Could not determine home directory");
    let mut toolbox_path = PathBuf::from(&home);
    toolbox_path.push(".shelly");
    toolbox_path.push("toolbox");

    let toolbox_config = &config["toolbox"];
    let mut files: Vec<(String, PathBuf)> = Vec::new();

    // Ensure toolbox dir exists
    if let Err(e) = fs::create_dir_all(&toolbox_path) {
        eprintln!("Failed to create toolbox directory {}: {}", toolbox_path.display(), e);
        return;
    }

    let client = Client::builder()
        .timeout(Duration::from_secs(300))
        .build()
        .expect("Failed to build HTTP client");

        for serve_file in serve_files {
            let serve_file = serve_file.as_str();
    
            if toolbox_config.has_key(serve_file) {
                let entry = &toolbox_config[serve_file];
                if entry.is_object() {
                    for (arch_key, arch_val) in entry.entries() {
                        if arch_val.is_object() {
                            if let Some(filename_val) = arch_val["filename"].as_str() {
                                let mut full_path = toolbox_path.clone();
                                full_path.push(filename_val);
    
                                if full_path.exists() {
                                    files.push((filename_val.to_string(), full_path));
                                } else {
                                    if let Some(download_url) = arch_val["download"].as_str() {
                                        println!(
                                            "Toolbox file missing for '{}', arch '{}': {}",
                                            serve_file,
                                            arch_key,
                                            full_path.display()
                                        );
                                        print!("Download {} from {}? [Y/n]: ", filename_val, download_url);
                                        io::stdout().flush().ok();
                                        let mut input = String::new();
                                        if io::stdin().read_line(&mut input).is_ok() {
                                            let resp = input.trim();
                                            if resp.is_empty() || resp.eq_ignore_ascii_case("y") || resp.eq_ignore_ascii_case("yes") {
                                                if let Err(e) = download_to_path_blocking(&client, download_url, &full_path) {
                                                    eprintln!("Failed to download {}: {}", download_url, e);
                                                } else {
                                                    println!("Downloaded to {}", full_path.display());
                                                    #[cfg(unix)]
                                                    {
                                                        use std::os::unix::fs::PermissionsExt;
                                                        if let Ok(mut perms) = fs::metadata(&full_path).map(|m| m.permissions()) {
                                                            perms.set_mode(0o755);
                                                            let _ = fs::set_permissions(&full_path, perms);
                                                        }
                                                    }
                                                    files.push((filename_val.to_string(), full_path));
                                                }
                                            } else {
                                                println!("Skipping download for {}", filename_val);
                                            }
                                        }
                                    } else {
                                        println!(
                                            "Toolbox file missing for '{}', arch '{}': {} (no download URL)",
                                            serve_file,
                                            arch_key,
                                            full_path.display()
                                        );
                                    }
                                }
                            } else {
                                println!(
                                    "No filename field for toolbox entry '{}', arch '{}'",
                                    serve_file, arch_key
                                );
                            }
                        }
                    }
                } else {
                    println!("Toolbox entry for '{}' is not an object", serve_file);
                }
            } else {
                println!("Serve file '{}' not found in toolbox config", serve_file);
            }
        }

    let server = StaticServer::new("0.0.0.0", port, files).expect("server init failed");
    let handle = thread::spawn(move || {
        server.serve().expect("serve failed");
    });
}

fn download_to_path_blocking(client: &Client, url: &str, dest: &PathBuf) -> Result<(), Box<dyn std::error::Error>> {
    // Create parent directory if needed
    if let Some(parent) = dest.parent() {
        fs::create_dir_all(parent)?;
    }

    // Send GET
    let mut resp = client.get(url).send()?;
    if !resp.status().is_success() {
        return Err(format!("HTTP error: {}", resp.status()).into());
    }

    // Try to obtain content length for progress
    let total_size = resp
        .headers()
        .get(reqwest::header::CONTENT_LENGTH)
        .and_then(|v| v.to_str().ok())
        .and_then(|s| s.parse::<u64>().ok());

    // Temporary file
    let tmp_path = dest.with_extension("download");
    let mut tmp_file = File::create(&tmp_path)?;

    let pb = match total_size {
        Some(len) => {
            let pb = ProgressBar::new(len);
            pb.set_style(
                ProgressStyle::with_template("{spinner:.green} [{elapsed_precise}] [{bar:40.cyan/blue}] {bytes}/{total_bytes} ({eta})")?
                    .progress_chars("#>-"),
            );
            Some(pb)
        }
        None => {
            let pb = ProgressBar::new_spinner();
            pb.set_style(
                ProgressStyle::with_template("{spinner:.green} {bytes} downloaded ({elapsed_precise})")?
            );
            pb.enable_steady_tick(Duration::from_millis(100));
            Some(pb)
        }
    };

    // Read response in chunks and write to file, updating progress bar
    let mut buf = [0u8; 8 * 1024];
    let mut downloaded: u64 = 0;
    loop {
        let n = resp.read(&mut buf)?;
        if n == 0 { break; }
        tmp_file.write_all(&buf[..n])?;
        downloaded += n as u64;
        if let Some(ref pb) = pb {
            pb.set_position(downloaded);
        }
    }

    if let Some(ref pb) = pb {
        pb.finish_and_clear();
    }

    fs::rename(&tmp_path, dest)?;

    Ok(())
}
