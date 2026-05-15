use std::net::SocketAddr;
use std::path::PathBuf;

use crate::cli::ServeArgs;

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ServeConfig {
    // CONTRACT: The daemon defaults to 127.0.0.1 reachability; localhost is not
    // authorization and all browser routes except pairing remain auth-gated.
    pub bind_addr: SocketAddr,
    pub static_dir: Option<PathBuf>,
}

impl ServeConfig {
    pub fn default_loopback() -> Self {
        // HINT: Use port 0 for tests and caller-selected ephemeral startup; CLI
        // may keep the same default until a dedicated port decision is made.
        todo!("construct 127.0.0.1:0")
    }

    pub fn from_args(args: ServeArgs) -> anyhow::Result<Self> {
        // HOLE: Parse host/port with clear errors and reject unsupported public
        // bind behavior until Phase 3 guardrails exist.
        let _ = args;
        todo!("parse serve args into ServeConfig")
    }
}
