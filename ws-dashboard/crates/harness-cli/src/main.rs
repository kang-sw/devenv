use ws_dashboard_harness_core::HarnessCapabilities;

fn main() {
    let mut args = std::env::args().skip(1);
    match args.next().as_deref() {
        Some("--capabilities") => print_capabilities(HarnessCapabilities::default()),
        Some("--version") => println!("{}", env!("CARGO_PKG_VERSION")),
        Some("-h") | Some("--help") | None => print_help(),
        Some(other) => {
            eprintln!("unknown argument: {other}");
            std::process::exit(2);
        }
    }
}

fn print_help() {
    println!("ws-dashboard-harness");
    println!();
    println!("Usage:");
    println!("  ws-dashboard-harness --capabilities");
    println!("  ws-dashboard-harness --version");
}

fn print_capabilities(capabilities: HarnessCapabilities) {
    println!(
        "{{\"supports_mcp\":{},\"supports_skills\":{},\"supports_api_models\":{},\"supports_secret_filtering\":{}}}",
        capabilities.supports_mcp,
        capabilities.supports_skills,
        capabilities.supports_api_models,
        capabilities.supports_secret_filtering
    );
}
