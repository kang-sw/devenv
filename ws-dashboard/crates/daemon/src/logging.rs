pub fn init(filter: &str) -> anyhow::Result<()> {
    // CONTRACT: Daemon startup installs structured logging suitable for server
    // and request lifecycle diagnostics without exposing auth secrets.
    let _ = filter;
    todo!("initialize tracing subscriber")
}
