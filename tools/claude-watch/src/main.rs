mod session;

fn main() {
    let sessions = session::discover_sessions();
    if sessions.is_empty() {
        println!("No sessions found.");
        return;
    }
    println!("{} session(s) found:", sessions.len());
    for s in &sessions {
        println!(
            "  [{}] {} — {}",
            if s.active { "LIVE" } else { "    " },
            s.label,
            s.modified.format("%Y-%m-%d %H:%M:%S"),
        );
    }
}
