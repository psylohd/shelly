use clap::Parser;
mod helpers;
mod config;
mod shell;
mod serve;

/// Simple Nc wrapper with revshell generation
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
    #[arg(short, long)]
    shell: Option<String>,

    /// Downloads reverse shell executables
    #[clap(long, short, action)]
    download: bool,

}

fn main() {
    // let files = vec![
    //     ("test".to_string(), PathBuf::from("/home/kali/dev/test/test")),
    // ];

    // let server = serve::StaticServer::new("127.0.0.1", 8477, files).expect("server init failed");
    // server.serve().expect("serve failed");

    config::ensure_exists();

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
    let shell = args.shell.as_deref().unwrap_or("");
    println!("ℹ️  {2} Revshells for {0}:{1}\n", ip, port, shell);

    let config = config::load_config();

    let mut listener_type = "";
    let http_port = config["shelly"]["default_http_svr"].as_u16().unwrap();

    if shell.is_empty() {
        println!("No revshell specified");
    } else if config.has_key("shells") {
        let shells = &config["shells"];
        if shells.has_key(shell) {
            let shell_obj = &shells[shell];
            if shell_obj.has_key("listener"){
                listener_type = shell_obj["listener"].as_str().unwrap();
            }
            if shell_obj.has_key("serve"){
                let serve_files_vec: Vec<String> = shell_obj["serve"]
                    .members()
                    .filter_map(|v| v.as_str().map(|s| s.to_string()))
                    .collect();
                serve::build_from_config(&serve_files_vec, &config, http_port);
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
    
    if listener_type.eq("socat_raw"){
        println!("\n Running socat in raw mode");
        let socat: shell::Socat = shell::Socat::new(port);
        socat.run();
    }
    else{
        println!("\nℹ️  Running nc");
        let netcat: shell::Netcat = shell::Netcat::new(port, config, &ip);
        netcat.run();
    }  
}

