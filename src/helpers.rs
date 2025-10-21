use std::process::{Command, Stdio};
use std::io;
use pnet::datalink::{self};
use terminal_menu::{menu, label, button, run, mut_menu};

pub fn cls() {
    Command::new("clear")
        .status()
        .expect("Failed to clear the screen");
}

pub fn port_input() -> Result<u16, String> {
    cls();
    let mut input = String::new();
    println!("Listening port (default: 4444):");
    io::stdin()
        .read_line(&mut input)
        .expect("Failed to read line");

    let trimmed_input = input.trim();
    if trimmed_input.is_empty() {
        return Ok(4444);
    }
    
    match trimmed_input.parse::<u16>() {
        Ok(port) if port > 0 && port <= 65535 => Ok(port),
        Ok(_) => Err(format!("Port number {} is out of range (0-65535)", trimmed_input)),
        Err(_) => Err(format!("Invalid input: '{}' is not a valid number", trimmed_input)),
    }
}

pub fn interface_selector() -> String {
    let interfaces = datalink::interfaces();
    let mut menu_items = vec![label("Select Listening Interface")];
    for interface in interfaces {
        if interface.is_loopback() || interface.ips.is_empty() {
            continue;
        }
        for ip in interface.ips {
            if ip.is_ipv4() {
                if let pnet::ipnetwork::IpNetwork::V4(addr) = ip {
                    menu_items.push(button(&addr.ip().to_string()));
                }
            }
        }
    }

    let menu = menu(menu_items);
    run(&menu);
    
    if mut_menu(&menu).canceled() {
        println!("Canceled!");
        std::process::exit(0);
    }
    
    return mut_menu(&menu).selected_item_name().to_string();
}