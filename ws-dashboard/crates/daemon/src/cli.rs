use clap::{Parser, Subcommand, ValueEnum};

use crate::config::ServeConfig;

#[derive(Debug, Parser)]
#[command(name = "ws-dashboard")]
pub struct Cli {
    #[command(subcommand)]
    command: Command,

    #[arg(long, default_value = "info")]
    log_filter: String,
}

#[derive(Debug, Subcommand)]
pub enum Command {
    Serve(ServeArgs),
}

#[derive(Debug, Parser)]
pub struct ServeArgs {
    // CONTRACT: Loopback is the default serving target. Public bind flags are
    // Phase 3 work and must fail closed if introduced before guard logic exists.
    #[arg(long, default_value = "127.0.0.1")]
    pub host: String,

    // CONTRACT: Bind mode is explicit before non-loopback serving is allowed.
    // Local and tunnel modes prefer loopback; public mode is the only mode that
    // may accept public interface binding after guard validation.
    #[arg(long, value_enum, default_value_t = BindMode::Local)]
    pub bind_mode: BindMode,

    #[arg(long, default_value_t = 0)]
    pub port: u16,

    // CONTRACT: Static UI serving is behind owner auth even when this points at
    // a placeholder or absent frontend build directory.
    #[arg(long)]
    pub static_dir: Option<std::path::PathBuf>,
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, ValueEnum)]
pub enum BindMode {
    /// Local browser access; defaults to loopback reachability.
    #[default]
    Local,
    /// Caller intends to place a separate tunnel in front of loopback serving.
    Tunnel,
    /// Caller explicitly intends non-loopback/public interface serving.
    Public,
}

impl Cli {
    pub fn log_filter(&self) -> &str {
        &self.log_filter
    }

    pub fn into_serve_config(self) -> anyhow::Result<ServeConfig> {
        match self.command {
            Command::Serve(args) => ServeConfig::from_args(args),
        }
    }
}
