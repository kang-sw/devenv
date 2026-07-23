// Pure boot-reconcile row classification (no I/O), kept separate from the
// async driver in `terminal.rs` so the decision table itself is directly
// unit-testable without a real registry directory, socket, or OS process.
//
// 6-row table (ticket "Boot reconcile policy"):
//   1. IPC reachable, identity verified-ours, helper reports the shell
//      still alive           -> Adopt (live).
//   2. IPC reachable, identity verified-ours, helper reports the shell
//      already exited (still inside its grace window) -> Adopt (grace).
//   3. Identity check fails because no process with the registered PID
//      exists at all (stale/crashed-before-ever-running entry) -> Drop
//      the entry only. NEVER attempt a kill - there is nothing to verify
//      it against.
//   4. Identity verified-ours, but IPC is unreachable (helper socket gone
//      or not answering within the connect timeout) -> Kill through a
//      verified handle, then drop the entry.
//   5. Identity check fails because the registered PID now belongs to a
//      *different* process (start-time mismatch = PID reuse) -> Drop the
//      entry only. NEVER kill - signaling that PID would hit an unrelated
//      process.
//   6. The registry file itself fails to parse -> handled one layer below
//      this module, in `terminal_registry_file::scan_registry_dir` (skip
//      that single entry, keep scanning the rest of the directory).
//
// Three-line invariant this table encodes:
//   Adopt = IPC-reachable && identity-ours.
//   Kill  = identity-verified-ours && IPC-dead.
//   Unverified identity -> NEVER kill, drop entry only, regardless of IPC
//   reachability (this module never even asks whether IPC was reachable in
//   that case - see `classify`, `IdentityStatus::NoSuchProcess` and
//   `IdentityStatus::PidReused` short-circuit before `ipc` is consulted).

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum IdentityStatus {
    /// The registered PID exists and its OS-reported start time matches the
    /// registry entry - this is very likely the same process the helper
    /// registered as.
    VerifiedOurs,
    /// No process with the registered PID exists.
    NoSuchProcess,
    /// A process with the registered PID exists, but its start time does
    /// not match - the PID has been recycled by an unrelated process.
    PidReused,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum IpcStatus {
    ReachableShellAlive,
    ReachableShellExited,
    Unreachable,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ReconcileRow {
    AdoptLive,
    AdoptGrace,
    DropNoSuchProcess,
    KillVerified,
    DropPidReused,
}

/// `ipc` is only consulted when `identity` is `VerifiedOurs` - an
/// unverified identity is dropped without ever considering IPC reachability
/// (the daemon-side driver in `terminal.rs` mirrors this by skipping the
/// connect attempt entirely for a failed identity check, not merely
/// ignoring its result).
pub fn classify(identity: IdentityStatus, ipc: IpcStatus) -> ReconcileRow {
    match identity {
        IdentityStatus::NoSuchProcess => ReconcileRow::DropNoSuchProcess,
        IdentityStatus::PidReused => ReconcileRow::DropPidReused,
        IdentityStatus::VerifiedOurs => match ipc {
            IpcStatus::ReachableShellAlive => ReconcileRow::AdoptLive,
            IpcStatus::ReachableShellExited => ReconcileRow::AdoptGrace,
            IpcStatus::Unreachable => ReconcileRow::KillVerified,
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn row_1_adopts_live_only_for_reachable_verified_identity_with_alive_shell() {
        assert_eq!(
            classify(IdentityStatus::VerifiedOurs, IpcStatus::ReachableShellAlive),
            ReconcileRow::AdoptLive
        );
    }

    #[test]
    fn row_2_adopts_grace_for_reachable_verified_identity_with_exited_shell() {
        assert_eq!(
            classify(IdentityStatus::VerifiedOurs, IpcStatus::ReachableShellExited),
            ReconcileRow::AdoptGrace
        );
    }

    #[test]
    fn row_3_drops_without_killing_when_no_such_process_exists() {
        // NEVER kill: `ipc` is deliberately fed every possible value here to
        // prove the outcome does not depend on it once identity fails.
        for ipc in [
            IpcStatus::ReachableShellAlive,
            IpcStatus::ReachableShellExited,
            IpcStatus::Unreachable,
        ] {
            assert_eq!(
                classify(IdentityStatus::NoSuchProcess, ipc),
                ReconcileRow::DropNoSuchProcess,
                "no-such-process must always drop-only, never kill (ipc={ipc:?})"
            );
        }
    }

    #[test]
    fn row_4_kills_verified_only_when_identity_ours_and_ipc_unreachable() {
        assert_eq!(
            classify(IdentityStatus::VerifiedOurs, IpcStatus::Unreachable),
            ReconcileRow::KillVerified
        );
    }

    #[test]
    fn row_5_drops_without_killing_on_pid_reuse() {
        for ipc in [
            IpcStatus::ReachableShellAlive,
            IpcStatus::ReachableShellExited,
            IpcStatus::Unreachable,
        ] {
            assert_eq!(
                classify(IdentityStatus::PidReused, ipc),
                ReconcileRow::DropPidReused,
                "pid-reuse must always drop-only, never kill (ipc={ipc:?})"
            );
        }
    }
}
