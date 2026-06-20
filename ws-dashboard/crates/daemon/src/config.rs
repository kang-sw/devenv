use std::net::{IpAddr, Ipv4Addr, SocketAddr};
use std::path::PathBuf;

use anyhow::{ensure, Context};

use crate::cli::{BindMode, ServeArgs};

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ServeConfig {
    // CONTRACT: The daemon defaults to 127.0.0.1 reachability; localhost is not
    // authorization and all browser routes except pairing remain auth-gated.
    pub bind_addr: SocketAddr,
    pub static_dir: Option<PathBuf>,
    // CONTRACT: Bind mode records the caller's reachability intent so startup
    // and tests can distinguish accidental public exposure from explicit
    // public serving.
    pub bind_mode: BindMode,
    // CONTRACT: Disabled owner authentication is a loopback-only debug serving
    // profile. Public serving requires owner authentication.
    pub owner_auth_enabled: bool,
}

impl ServeConfig {
    pub fn default_loopback() -> Self {
        Self {
            bind_addr: SocketAddr::from((Ipv4Addr::LOCALHOST, 0)),
            static_dir: None,
            bind_mode: BindMode::Local,
            owner_auth_enabled: true,
        }
    }

    pub fn from_args(args: ServeArgs) -> anyhow::Result<Self> {
        let ip = parse_bind_host(&args.host)
            .with_context(|| format!("invalid --host value {:?}", args.host))?;

        let owner_auth_enabled = !args.no_auth;
        validate_bind_guard(args.bind_mode, ip, owner_auth_enabled)?;
        Ok(Self {
            bind_addr: SocketAddr::new(ip, args.port),
            static_dir: args.static_dir,
            bind_mode: args.bind_mode,
            owner_auth_enabled,
        })
    }
}

pub fn validate_bind_guard(
    mode: BindMode,
    ip: IpAddr,
    owner_auth_enabled: bool,
) -> anyhow::Result<()> {
    if !owner_auth_enabled {
        ensure!(
            mode != BindMode::Public,
            "no-auth debug mode cannot use --bind-mode public"
        );
        ensure!(
            ip.is_loopback(),
            "no-auth debug mode requires a loopback bind address"
        );
        return Ok(());
    }

    // CONTRACT: Accidental public interface exposure fails unless public mode is
    // explicit and owner authentication is enabled.
    ensure!(
        mode != BindMode::Public || owner_auth_enabled,
        "public bind mode requires owner authentication"
    );

    if ip.is_loopback() {
        return Ok(());
    }

    ensure!(
        mode == BindMode::Public,
        "public bind address {ip} requires --bind-mode public"
    );

    Ok(())
}

fn parse_bind_host(host: &str) -> anyhow::Result<IpAddr> {
    if host.eq_ignore_ascii_case("localhost") {
        return Ok(IpAddr::V4(Ipv4Addr::LOCALHOST));
    }

    host.parse::<IpAddr>()
        .with_context(|| "expected an IP address or localhost")
}
