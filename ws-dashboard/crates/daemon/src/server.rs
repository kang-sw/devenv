use std::net::SocketAddr;

use crate::auth::OwnerAuthState;
use crate::config::ServeConfig;

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StartupInfo {
    pub bound_addr: SocketAddr,
    pub pairing_url: String,
}

pub async fn run(config: ServeConfig) -> anyhow::Result<()> {
    // CONTRACT: Server startup prints or otherwise exposes exactly one owner
    // pairing URL once the bound address is known.
    // CONTRACT: Graceful shutdown is part of the daemon shell; request logging
    // must avoid leaking pairing query strings.
    let _ = config;
    todo!("bind listener, emit startup info, serve router, and await shutdown")
}

pub fn startup_info(bound_addr: SocketAddr, auth: &OwnerAuthState) -> StartupInfo {
    // HINT: Keep URL construction testable without opening a socket.
    let _ = (bound_addr, auth);
    todo!("construct owner pairing URL")
}
