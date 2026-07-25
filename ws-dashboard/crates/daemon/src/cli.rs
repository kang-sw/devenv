use clap::{CommandFactory, Parser, Subcommand, ValueEnum};

use crate::config::ServeConfig;

#[derive(Debug, Parser)]
#[command(name = "ws-dashboard")]
pub struct Cli {
    #[command(subcommand)]
    command: Option<Command>,

    #[arg(long, default_value = "info")]
    log_filter: String,

    #[arg(long, help = "Print the SSH-tunneled remote deployment guide and exit")]
    remote_guide: bool,
}

#[derive(Debug, Subcommand)]
pub enum Command {
    Serve(ServeArgs),
    // CONTRACT: internal re-exec target for the detached per-terminal helper
    // process (260723 terminal lifetime supervisor decouple). Not documented
    // in `--help`; the daemon spawns this itself, pointed at its own
    // resolved binary path (see `terminal_platform`). Never invoked directly
    // by a human or the remote-deployment guide.
    #[command(hide = true)]
    TerminalHelper(TerminalHelperArgs),
}

#[derive(Debug, Clone, Parser)]
pub struct TerminalHelperArgs {
    #[arg(long)]
    pub registry_dir: std::path::PathBuf,
    #[arg(long)]
    pub terminal_id: String,
    #[arg(long)]
    pub work_root_id: String,
    #[arg(long)]
    pub cwd: std::path::PathBuf,
    #[arg(long)]
    pub cwd_hint: Option<String>,
    #[arg(long)]
    pub title: String,
    #[arg(long)]
    pub columns: u16,
    #[arg(long)]
    pub rows: u16,
    #[arg(long)]
    pub socket_path: std::path::PathBuf,
    // CONTRACT (260725 Phase 1, pty-agent spawn-seam argv/env scrub):
    // explicit command + env overlay passthrough. `command` absent (the
    // default) means "spawn `default_shell()`, unchanged from today" -
    // `command_args`/`env_overlay` are only meaningful when `command` is
    // `Some`. All three default to absent/empty so existing manual-argv
    // fixtures that never pass these flags keep parsing unmodified.
    //
    // `requires = "command"` on the two dependent flags (review cycle 1,
    // finding C2) turns "command-arg/env-overlay passed without --command"
    // into a hard parse failure instead of a silent no-op: both fields
    // previously fed code paths gated on `command.is_some()`, so an absent
    // `command` meant the caller's flags were accepted and then quietly
    // discarded with no error and no log - a natural mistake for a future
    // caller (e.g. Phase 2's profile selector) to make.
    #[arg(long)]
    pub command: Option<String>,
    // `allow_hyphen_values`: forwarded argv commonly includes the target
    // command's own flags (e.g. `--command-arg --flag`), which clap would
    // otherwise reject as an unexpected option rather than accept as this
    // flag's value.
    #[arg(long = "command-arg", allow_hyphen_values = true, requires = "command")]
    pub command_args: Vec<String>,
    // CONTRACT (review cycle 1, finding C3): overlay VALUES land verbatim in
    // the helper's argv, which - like every other `TerminalHelperArgs` field
    // - is world-readable to any local process via `ps`. This flag exists
    // for non-secret overlay values only (e.g. a vendor CLI's base-URL
    // override); it must never carry a credential or token. In particular,
    // the parent ticket's Phase 4 daemon-owned callback token is designed to
    // never touch the helper or the registry precisely because helper argv
    // is world-readable - `--env-overlay` must NOT become the channel that
    // routes it there. See also `terminal.rs`'s `build_helper_command` for
    // the argv-forwarding call site this flag feeds.
    #[arg(long = "env-overlay", value_parser = parse_env_overlay, requires = "command")]
    pub env_overlay: Vec<(String, String)>,
}

// CONTRACT: pure `KEY=VALUE` split on the first `=`; used as the clap
// `value_parser` for `--env-overlay` so malformed overlay flags fail fast at
// argument parsing rather than surfacing later as a silently-dropped or
// misinterpreted env var.
fn parse_env_overlay(raw: &str) -> Result<(String, String), String> {
    match raw.split_once('=') {
        Some((key, value)) => Ok((key.to_owned(), value.to_owned())),
        None => Err(format!("expected KEY=VALUE, got `{raw}` (missing `=`)")),
    }
}

#[derive(Debug, Parser)]
pub struct ServeArgs {
    // CONTRACT: Loopback is the default serving target. Public bind flags stay
    // behind explicit bind-mode and owner-auth guard validation.
    #[arg(long, default_value = "127.0.0.1")]
    pub host: String,

    // CONTRACT: Bind mode is explicit before non-loopback serving is allowed.
    // Local and tunnel modes prefer loopback; public mode is the only mode that
    // may accept public interface binding after guard validation.
    #[arg(long, value_enum, default_value_t = BindMode::Local)]
    pub bind_mode: BindMode,

    #[arg(
        long,
        help = "Disable owner authentication for loopback-only local debug serving"
    )]
    pub no_auth: bool,

    #[arg(long, default_value_t = 0)]
    pub port: u16,

    // CONTRACT: Static UI serving follows the central router auth boundary; the
    // loopback-only no-auth debug profile is the only bypass.
    #[arg(long)]
    pub static_dir: Option<std::path::PathBuf>,

    // CONTRACT: Absent, the file log sink defaults to the daemon's
    // persistent state directory; this override only changes where the
    // rolling file sink writes, not whether it is enabled.
    #[arg(
        long,
        help = "Override the daemon log file path (default: state dir logs/daemon.log)"
    )]
    pub log_file: Option<std::path::PathBuf>,
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

    // CONTRACT: Non-consuming — read before `into_serve_config` consumes
    // `self.command`.
    pub fn log_file(&self) -> Option<&std::path::Path> {
        match self.command.as_ref() {
            Some(Command::Serve(args)) => args.log_file.as_deref(),
            _ => None,
        }
    }

    pub fn wants_remote_guide(&self) -> bool {
        self.remote_guide
    }

    // CONTRACT: Non-consuming, checked before `into_serve_config` consumes
    // `self.command` - mirrors `log_file`'s accessor shape.
    pub fn terminal_helper_args(&self) -> Option<&TerminalHelperArgs> {
        match self.command.as_ref() {
            Some(Command::TerminalHelper(args)) => Some(args),
            _ => None,
        }
    }

    pub fn remote_deployment_guide() -> &'static str {
        REMOTE_DEPLOYMENT_GUIDE
    }

    pub fn into_serve_config(self) -> anyhow::Result<ServeConfig> {
        match self.command {
            Some(Command::Serve(args)) => ServeConfig::from_args(args),
            Some(Command::TerminalHelper(_)) => {
                anyhow::bail!("terminal-helper is an internal re-exec target, not a serve command")
            }
            None => {
                let mut command = Self::command();
                command.print_help()?;
                eprintln!();
                anyhow::bail!("missing ws-dashboard command")
            }
        }
    }
}

const REMOTE_DEPLOYMENT_GUIDE: &str = r#"ws-dashboard remote deployment guide

Purpose:
  Run the browser against the local dashboard daemon while the local daemon
  acts as a gateway to a dashboard daemon on a remote host.

Model:
  browser -> local ws-dashboard daemon -> SSH tunnel -> remote ws-dashboard daemon

Remote daemon:
  - Prefer binding the remote daemon to remote loopback, for example:
      ws-dashboard serve --bind-mode tunnel --host 127.0.0.1 --port 0
  - Do not expose the remote daemon on a public interface for the MVP path.
  - The remote daemon can outlive the local daemon. Stopping the local gateway
    should not be treated as a request to stop the remote process.

SSH tunnel:
  - Create a local forward from the local dashboard host to the remote
    loopback endpoint printed by the remote daemon.
  - Keep SSH as the deploy/start/tunnel transport. Dashboard owner
    authentication remains separate from SSH authentication.

Passphrase and credentials:
  - The remote daemon may print a daemon-lifetime passphrase or pairing secret.
  - The user records that passphrase outside ws-dashboard and enters it in the
    local dashboard UI when linking or reconnecting.
  - The local daemon may hold a remote link token in memory after successful
    link authentication.
  - Credential persistence is disabled in the MVP.
  - Passphrases, link tokens, and active tunnel process details are not
    persisted in the MVP.

Reconnect:
  - Persist only non-secret linked-server metadata such as display name, SSH
    target, endpoint hints, tunnel configuration, last-seen time, and bounded
    capabilities.
  - After local daemon restart, recreate the SSH tunnel from persisted metadata
    when possible, then ask the user to re-enter the passphrase if
    authentication is required.

Troubleshooting checks:
  - Verify SSH connectivity to the remote host before starting the remote
    daemon.
  - Verify the remote daemon is reachable through a local-forwarded loopback
    URL before entering the passphrase.
  - Treat wrong passphrase, stale endpoint, tunnel failure, incompatible
    capability, and non-dashboard HTTP responses as distinct failures.

This guide is documentation for humans and AI agents. It is not a stable
machine protocol, does not persist credentials, and does not start or expose a
remote daemon by itself.
"#;

#[cfg(test)]
mod tests {
    use super::*;
    use clap::Parser;

    #[test]
    fn remote_guide_flag_is_discoverable_from_help() {
        let mut command = Cli::command();
        let mut help = Vec::new();
        command.write_long_help(&mut help).expect("write help");
        let help = String::from_utf8(help).expect("utf8 help");

        assert!(help.contains("--remote-guide"));
        assert!(help.contains("SSH-tunneled remote deployment guide"));
    }

    #[test]
    fn serve_no_auth_flag_is_discoverable_from_help() {
        let mut command = ServeArgs::command();
        let mut help = Vec::new();
        command.write_long_help(&mut help).expect("write help");
        let help = String::from_utf8(help).expect("utf8 help");

        assert!(help.contains("--no-auth"));
        assert!(help.contains("loopback-only local debug serving"));
    }

    #[test]
    fn serve_log_file_flag_is_discoverable_from_help() {
        let mut command = ServeArgs::command();
        let mut help = Vec::new();
        command.write_long_help(&mut help).expect("write help");
        let help = String::from_utf8(help).expect("utf8 help");

        assert!(help.contains("--log-file"));
        assert!(help.contains("state dir logs/daemon.log"));
    }

    #[test]
    fn serve_log_file_flag_parses_to_accessor() {
        let cli = Cli::parse_from(["ws-dashboard", "serve", "--log-file", "/tmp/x.log"]);

        assert_eq!(cli.log_file(), Some(std::path::Path::new("/tmp/x.log")));
    }

    #[test]
    fn serve_log_file_accessor_is_none_when_flag_absent() {
        let cli = Cli::parse_from(["ws-dashboard", "serve"]);

        assert_eq!(cli.log_file(), None);
    }

    #[test]
    fn remote_guide_flag_parses_without_subcommand() {
        let cli = Cli::parse_from(["ws-dashboard", "--remote-guide"]);

        assert!(cli.wants_remote_guide());
    }

    #[test]
    fn terminal_helper_args_parse_command_argv_and_env_overlay_flags() {
        let cli = Cli::parse_from([
            "ws-dashboard",
            "terminal-helper",
            "--registry-dir",
            "/tmp/registry",
            "--terminal-id",
            "t1",
            "--work-root-id",
            "wr1",
            "--cwd",
            "/tmp/cwd",
            "--title",
            "test",
            "--columns",
            "80",
            "--rows",
            "24",
            "--socket-path",
            "/tmp/t1.sock",
            "--command",
            "agent-cli",
            "--command-arg",
            "--flag",
            "--command-arg",
            "value",
            "--env-overlay",
            "FOO=bar",
            "--env-overlay",
            "BASE_URL=http://x",
        ]);

        let args = cli.terminal_helper_args().expect("terminal-helper args");
        assert_eq!(args.command.as_deref(), Some("agent-cli"));
        assert_eq!(args.command_args, vec!["--flag".to_owned(), "value".to_owned()]);
        assert_eq!(
            args.env_overlay,
            vec![
                ("FOO".to_owned(), "bar".to_owned()),
                ("BASE_URL".to_owned(), "http://x".to_owned()),
            ]
        );
    }

    #[test]
    fn terminal_helper_args_command_argv_and_env_overlay_default_absent() {
        let cli = Cli::parse_from([
            "ws-dashboard",
            "terminal-helper",
            "--registry-dir",
            "/tmp/registry",
            "--terminal-id",
            "t1",
            "--work-root-id",
            "wr1",
            "--cwd",
            "/tmp/cwd",
            "--title",
            "test",
            "--columns",
            "80",
            "--rows",
            "24",
            "--socket-path",
            "/tmp/t1.sock",
        ]);

        let args = cli.terminal_helper_args().expect("terminal-helper args");
        assert_eq!(args.command, None);
        assert!(args.command_args.is_empty());
        assert!(args.env_overlay.is_empty());
    }

    // CONTRACT (review cycle 1, finding C2): base args shared by the two
    // rejection tests below, deliberately omitting `--command`.
    fn base_terminal_helper_args() -> Vec<&'static str> {
        vec![
            "ws-dashboard",
            "terminal-helper",
            "--registry-dir",
            "/tmp/registry",
            "--terminal-id",
            "t1",
            "--work-root-id",
            "wr1",
            "--cwd",
            "/tmp/cwd",
            "--title",
            "test",
            "--columns",
            "80",
            "--rows",
            "24",
            "--socket-path",
            "/tmp/t1.sock",
        ]
    }

    #[test]
    fn terminal_helper_args_command_arg_without_command_is_rejected() {
        let mut args = base_terminal_helper_args();
        args.extend(["--command-arg", "--flag"]);

        assert!(
            Cli::try_parse_from(args).is_err(),
            "--command-arg without --command must fail to parse, not silently no-op"
        );
    }

    #[test]
    fn terminal_helper_args_env_overlay_without_command_is_rejected() {
        let mut args = base_terminal_helper_args();
        args.extend(["--env-overlay", "FOO=bar"]);

        assert!(
            Cli::try_parse_from(args).is_err(),
            "--env-overlay without --command must fail to parse, not silently no-op"
        );
    }

    #[test]
    fn parse_env_overlay_splits_on_first_equals() {
        assert_eq!(
            parse_env_overlay("KEY=value=with=equals"),
            Ok(("KEY".to_owned(), "value=with=equals".to_owned()))
        );
    }

    #[test]
    fn parse_env_overlay_rejects_missing_equals() {
        assert!(parse_env_overlay("NOEQUALSSIGN").is_err());
    }

    #[test]
    fn remote_deployment_guide_records_remote_boundaries() {
        let guide = Cli::remote_deployment_guide();

        assert!(guide.contains("browser -> local ws-dashboard daemon"));
        assert!(guide.contains("SSH tunnel"));
        assert!(guide.contains("remote loopback"));
        assert!(guide.contains("daemon-lifetime passphrase"));
        assert!(guide.contains("Credential persistence is disabled"));
        assert!(guide.contains("not a stable\nmachine protocol"));
    }
}
