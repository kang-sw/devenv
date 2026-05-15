use clap::Parser;
use ws_dashboard_daemon::{cli::Cli, logging, server};

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // CONTRACT: `ws-dashboard serve` is the public daemon command; the binary
    // stays a thin adapter over the testable daemon library.
    let cli = Cli::parse();
    logging::init(cli.log_filter())?;
    server::run(cli.into_serve_config()?).await
}
