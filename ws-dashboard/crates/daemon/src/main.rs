use clap::Parser;
use ws_dashboard_daemon::{cli::Cli, logging, server};

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // CONTRACT: `ws-dashboard serve` is the public daemon command; the binary
    // stays a thin adapter over the testable daemon library.
    let cli = Cli::parse();
    if cli.wants_remote_guide() {
        print!("{}", Cli::remote_deployment_guide());
        return Ok(());
    }
    // CONTRACT: `_guard` (not `_`) keeps the file sink's `WorkerGuard` alive
    // for the process lifetime — a bare `_` binding would drop it
    // immediately and silently disable the non-blocking writer's flush.
    let log_file = cli.log_file().map(std::path::Path::to_path_buf);
    let _guard = logging::init(cli.log_filter(), log_file)?;
    server::run(cli.into_serve_config()?).await
}
