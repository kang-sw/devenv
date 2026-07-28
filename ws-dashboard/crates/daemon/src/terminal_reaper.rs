// Periodic orphan-reaper backstop (260726 Phase 1 sub-fix 3): owns only the
// interval-tick + spawn/abort glue. The actual sweep decision logic
// (grace-authority eviction, IPC teardown, registry-dir backstop) lives in
// `terminal.rs` next to the private fns it needs (`identity_status`,
// `connect_and_handshake`'s deliberate non-use, `kill_verified_and_delete_
// entry`) - this module fits the established `terminal_*` sibling-module
// split (`terminal_reconcile.rs`, `terminal_registry_file.rs`,
// `terminal_platform.rs`, ...) without growing `terminal.rs` further.

use std::time::Duration;

use crate::terminal::TerminalRegistry;

/// Spawns the periodic sweep task. `interval` is the concrete sweep tick
/// period the caller has chosen (see `server.rs`'s wiring for the value and
/// its rationale). The returned `JoinHandle` must be `.abort()`ed as part of
/// (or strictly before) the daemon's shutdown path - this task otherwise
/// runs forever, ticking against a `TerminalRegistry` clone that outlives
/// the router's own copy.
pub fn spawn(registry: TerminalRegistry, interval: Duration) -> tokio::task::JoinHandle<()> {
    tokio::spawn(async move {
        let mut ticker = tokio::time::interval(interval);
        loop {
            ticker.tick().await;
            registry.sweep_once().await;
        }
    })
}
