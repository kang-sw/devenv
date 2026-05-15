use std::net::{IpAddr, Ipv4Addr, SocketAddr};
use std::path::PathBuf;

use anyhow::{bail, Context};

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
        Self {
            bind_addr: SocketAddr::from((Ipv4Addr::LOCALHOST, 0)),
            static_dir: None,
        }
    }

    pub fn from_args(args: ServeArgs) -> anyhow::Result<Self> {
        let ip = parse_phase_one_host(&args.host)
            .with_context(|| format!("invalid --host value {:?}", args.host))?;
        if !ip.is_loopback() {
            bail!("unsupported public bind address {ip}; Phase 1 only supports loopback hosts");
        }

        Ok(Self {
            bind_addr: SocketAddr::new(ip, args.port),
            static_dir: args.static_dir,
        })
    }
}

fn parse_phase_one_host(host: &str) -> anyhow::Result<IpAddr> {
    if host.eq_ignore_ascii_case("localhost") {
        return Ok(IpAddr::V4(Ipv4Addr::LOCALHOST));
    }

    host.parse::<IpAddr>()
        .with_context(|| "expected an IP address or localhost")
}
