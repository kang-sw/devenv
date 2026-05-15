use tracing_subscriber::EnvFilter;

pub fn init(filter: &str) -> anyhow::Result<()> {
    // CONTRACT: Daemon startup installs structured logging suitable for server
    // and request lifecycle diagnostics without exposing auth secrets.
    let env_filter = EnvFilter::try_new(filter)?;
    let _ = tracing_subscriber::fmt()
        .with_env_filter(env_filter)
        .try_init();
    Ok(())
}
