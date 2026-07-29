// Cross-platform daemon<->helper IPC transport: Unix domain sockets on Unix,
// Windows named pipes on Windows, type-erased behind a small `IpcStream`
// trait object so `terminal.rs`/`terminal_helper_process.rs` stay platform-
// neutral (they only ever see `BoxedIpcStream`/`IpcReadHalf`/`IpcWriteHalf`).
// `terminal_helper_ipc.rs`'s NDJSON framing was already written generic over
// `AsyncRead`/`AsyncWrite`; this module is what lets that genericity actually
// pay off across platforms instead of only ever being instantiated with
// `tokio::net::unix::{OwnedReadHalf, OwnedWriteHalf}`.
//
// STAGE 2 SCOPE NOTE (260723 Phase 1): the Windows leg here is cross-compile-
// checked and given a real (non-stub) implementation, but has not been
// exercised against a live Windows host in this session - only the Unix leg
// has real end-to-end process coverage (`terminal_lifetime.rs`).

use std::io;
use std::path::Path;

use tokio::io::{AsyncRead, AsyncWrite};

/// Marker trait erasing the platform-specific stream type. Blanket-
/// implemented for anything that is already a valid async duplex stream.
pub trait IpcStream: AsyncRead + AsyncWrite + Send + Unpin {}
impl<T: AsyncRead + AsyncWrite + Send + Unpin> IpcStream for T {}

pub type BoxedIpcStream = Box<dyn IpcStream>;
pub type IpcReadHalf = tokio::io::ReadHalf<BoxedIpcStream>;
pub type IpcWriteHalf = tokio::io::WriteHalf<BoxedIpcStream>;

pub fn split(stream: BoxedIpcStream) -> (IpcReadHalf, IpcWriteHalf) {
    tokio::io::split(stream)
}

#[cfg(unix)]
pub async fn connect(socket_path: &Path) -> io::Result<BoxedIpcStream> {
    let stream = tokio::net::UnixStream::connect(socket_path).await?;
    Ok(Box::new(stream))
}

#[cfg(windows)]
pub async fn connect(socket_path: &Path) -> io::Result<BoxedIpcStream> {
    use tokio::net::windows::named_pipe::ClientOptions;
    let pipe_name = windows_pipe_name(socket_path);
    // Named pipe connects can transiently fail with ERROR_PIPE_BUSY while a
    // previous client is still being accepted; the daemon side already
    // retries `connect` in a poll loop (see `terminal.rs::connect_and_
    // handshake`), so a single attempt here is sufficient - a failure simply
    // makes that outer loop retry.
    let client = ClientOptions::new().open(&pipe_name)?;
    Ok(Box::new(client))
}

#[cfg(unix)]
pub struct IpcListener(tokio::net::UnixListener);

#[cfg(unix)]
impl IpcListener {
    pub fn bind(socket_path: &Path) -> io::Result<Self> {
        Ok(Self(tokio::net::UnixListener::bind(socket_path)?))
    }

    pub async fn accept(&mut self) -> io::Result<BoxedIpcStream> {
        let (stream, _addr) = self.0.accept().await?;
        Ok(Box::new(stream))
    }
}

// Windows named pipes have no single "listen socket" accepting many
// connections the way a Unix domain socket does: each `NamedPipeServer`
// instance represents exactly one pending connection slot, and a client that
// finds no UNCONNECTED instance gets `ERROR_PIPE_BUSY` rather than being
// queued.
//
// CONTRACT (260729 review round 3, finding E - and this is a correctness fix,
// not a tuning knob): this used to keep exactly ONE armed instance, re-armed
// only inside `accept()`. That made "armed" conditional on `accept()` being
// POLLED, and `terminal_helper_process::serve_session` deliberately stops
// polling it while all `MAX_CONCURRENT_PROBE_CONNECTIONS` probe slots are
// taken - that is its backpressure design, which on Unix parks the surplus in
// the kernel backlog. On Windows it instead left at most one unconnected
// instance, so the next daemon to connect got `ERROR_PIPE_BUSY`, its 400ms
// connect budget expired, and the outcome inverted into exactly the false
// death this ticket exists to remove:
//   - `connect_and_handshake` -> `NoListener` -> `KillVerified` (unconditional:
//     positive absence needs no probe verdict), or
//   - `probe_helper` -> `Unanswered` -> `KillVerified`.
// A healthy helper, SIGKILLed for being popular.
//
// The fix is a real BACKLOG: `PIPE_BACKLOG` instances are armed up front and
// the count is restored on every accept, so unconnected instances exist
// whether or not anyone is polling. `accept()` polls all of them concurrently
// rather than one at a time - polling a single instance while a client sits
// connected on another would be head-of-line blocking, i.e. the same "nobody
// answered" signature by a different route. `NamedPipeServer::connect` is
// documented cancel-safe, so the losing futures being dropped each time (and
// on the caller's own `timeout(..., accept())`) loses no connection.
//
// Sized above `MAX_CONCURRENT_PROBE_CONNECTIONS` (8) plus the attached
// session, so the whole in-flight probe cap can be saturated and a further
// daemon can still connect and be queued.
#[cfg(windows)]
const PIPE_BACKLOG: usize = 16;

#[cfg(windows)]
pub struct IpcListener {
    pipe_name: String,
    /// Always exactly `PIPE_BACKLOG` armed (unconnected-or-connected-but-not-
    /// yet-accepted) instances. Never empty - `accept()` re-arms before it
    /// returns.
    pending: Vec<tokio::net::windows::named_pipe::NamedPipeServer>,
}

#[cfg(windows)]
impl IpcListener {
    pub fn bind(socket_path: &Path) -> io::Result<Self> {
        use tokio::net::windows::named_pipe::ServerOptions;
        let pipe_name = windows_pipe_name(socket_path);
        let mut pending = Vec::with_capacity(PIPE_BACKLOG);
        // `first_pipe_instance` is what makes a second helper binding the same
        // terminal id fail loudly instead of silently sharing the pipe; only
        // the very first instance may claim it.
        pending.push(
            ServerOptions::new()
                .first_pipe_instance(true)
                .create(&pipe_name)?,
        );
        while pending.len() < PIPE_BACKLOG {
            pending.push(ServerOptions::new().create(&pipe_name)?);
        }
        Ok(Self { pipe_name, pending })
    }

    pub async fn accept(&mut self) -> io::Result<BoxedIpcStream> {
        use tokio::net::windows::named_pipe::ServerOptions;
        let (result, index) = {
            let connects = self
                .pending
                .iter_mut()
                .map(|server| Box::pin(server.connect()));
            let (result, index, _rest) = futures_util::future::select_all(connects).await;
            (result, index)
        };
        result?;
        let connected = self.pending.swap_remove(index);
        // Re-arm BEFORE handing the connected instance back, so the backlog is
        // never short even for the duration of this return.
        self.pending
            .push(ServerOptions::new().create(&self.pipe_name)?);
        Ok(Box::new(connected))
    }
}

/// Derives a stable Windows named-pipe name from the same `<registry_dir>/
/// <terminal_id>.sock` path the Unix leg binds as a real filesystem socket.
/// Only the terminal-id-derived file stem is used - `terminal_id` is already
/// a per-terminal random 18-character suffix (see `opaque_terminal_id`), so
/// collision risk across concurrent terminals/users on one machine is
/// negligible without also encoding the full registry directory.
#[cfg(windows)]
fn windows_pipe_name(socket_path: &Path) -> String {
    let stem = socket_path
        .file_stem()
        .and_then(|stem| stem.to_str())
        .unwrap_or("terminal");
    format!(r"\\.\pipe\ws-dashboard-terminal-{stem}")
}

#[cfg(all(test, windows))]
mod windows_tests {
    use super::*;

    #[test]
    fn windows_pipe_name_is_derived_from_the_socket_path_stem() {
        let name = windows_pipe_name(Path::new(r"C:\state\terminals\term_abc123.sock"));
        assert_eq!(name, r"\\.\pipe\ws-dashboard-terminal-term_abc123");
    }
}
