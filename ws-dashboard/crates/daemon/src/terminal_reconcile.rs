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
// CONTRACT (260723 Phase-1 review finding M-c, numbering note): the list
// above enumerates outcomes in THIS module's own order, which does not
// match the ticket's literal 6-row table numbering 1:1 - the ticket's row 3
// is "IPC reachable + identity mismatch (PID reused)" and its row 6 is "PID
// gone (`NoSuchProcess`)"; item 3 above is the ticket's row 6, and item 5
// above (`PidReused`) actually covers BOTH the ticket's row 3 (reachable +
// mismatch) and row 5 (unreachable + mismatch), because `identity` is
// checked before `ipc` is ever consulted (see `classify` below and
// `reconcile_entry`'s matching pre-check in `terminal.rs`) - a mismatched
// identity drops the entry regardless of what IPC reachability would have
// been, so those two ticket rows are provably the same code branch here,
// not two independently-reachable outcomes. `tests::row_3_...` /
// `tests::row_5_...` below name themselves after THIS list's order, not the
// ticket's row numbers - keep that in mind when cross-checking ticket-row
// coverage against test names in this module or in `terminal.rs`'s
// `boot_reconcile_drops_entry_*` tests.
//
// A seventh outcome was added later (boot-identity gate, see
// `boot_identity_verified` below): a registry entry whose recorded boot
// identity does not match this boot's - or which carries none at all on a
// platform where `start_time` is boot-relative - is UNVERIFIABLE, not merely
// mismatched. It joins rows 3/5 as drop-only, never kill.
//
// Three-line invariant this table encodes:
//   Adopt = IPC-reachable && identity-ours.
//   Kill  = identity-verified-ours && IPC-dead.
//   Unverified identity -> NEVER kill, drop entry only, regardless of IPC
//   reachability (this module never even asks whether IPC was reachable in
//   that case - see `classify`, `IdentityStatus::NoSuchProcess`,
//   `IdentityStatus::PidReused` and `IdentityStatus::UnverifiableBoot`
//   short-circuit before `ipc` is consulted).

/// CONTRACT (boot-identity gate): decides whether a registry entry's
/// `start_time` may be compared against a live process's at all.
///
/// `start_time_is_boot_relative` / `current` come from
/// `terminal_platform::START_TIME_IS_BOOT_RELATIVE` /
/// `terminal_platform::boot_identity()`; `recorded` is
/// `TerminalRegistryEntry::boot_id`. Kept pure (all three injected) so every
/// platform's behavior is testable from any platform - the real hazard is
/// Linux-only, but the guard must be provably inert on macOS/Windows too.
///
/// Two deliberate asymmetries:
/// - On an ABSOLUTE-start-time platform (macOS, Windows) this is always
///   `true`: the recorded field is not consulted at all, so entries written
///   before the field existed keep verifying exactly as they did before.
///   Making those platforms consult it would strand every legacy entry as
///   unverifiable - a regression, since their start times were never
///   ambiguous across boots in the first place.
/// - On a BOOT-RELATIVE platform (Linux) verification requires BOTH sides to
///   be present AND equal. `recorded == None` (a legacy entry, or a helper
///   that could not read the boot id) is unverifiable, NOT trusted: a
///   boot-relative `start_time` with no boot qualifier is exactly the value
///   that a hard reboot can silently re-mint under an unrelated process's
///   pid. `current == None` (this daemon cannot read the boot id) is
///   likewise unverifiable rather than "assume it still matches".
pub fn boot_identity_verified(
    start_time_is_boot_relative: bool,
    recorded: Option<&str>,
    current: Option<&str>,
) -> bool {
    if !start_time_is_boot_relative {
        return true;
    }
    matches!((recorded, current), (Some(recorded), Some(current)) if recorded == current)
}

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
    /// The recorded start time cannot be compared at all, because the entry's
    /// boot identity does not match this boot (or is absent on a platform
    /// where start times are boot-relative). Distinct from `NoSuchProcess`
    /// on purpose: a process may well hold that PID right now, we simply have
    /// no evidence it is ours. Behaviorally identical to `NoSuchProcess`
    /// (drop-only, never kill) - see `boot_identity_verified`.
    UnverifiableBoot,
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
    DropUnverifiableBoot,
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
        IdentityStatus::UnverifiableBoot => ReconcileRow::DropUnverifiableBoot,
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

    // CONTRACT (boot-identity gate): an unverifiable boot identity is a
    // drop-only row for every IPC value, exactly like rows 3 and 5. Fails
    // loudly if anyone maps `UnverifiableBoot` onto the kill row.
    #[test]
    fn unverifiable_boot_identity_drops_without_killing_for_every_ipc_status() {
        for ipc in [
            IpcStatus::ReachableShellAlive,
            IpcStatus::ReachableShellExited,
            IpcStatus::Unreachable,
        ] {
            let row = classify(IdentityStatus::UnverifiableBoot, ipc);
            assert_eq!(
                row,
                ReconcileRow::DropUnverifiableBoot,
                "an unverifiable boot identity must always drop-only (ipc={ipc:?})"
            );
            assert_ne!(
                row,
                ReconcileRow::KillVerified,
                "an unverifiable boot identity must NEVER reach the kill row"
            );
        }
    }

    // CONTRACT (boot-identity gate, Linux hazard): on a boot-relative
    // platform, only a present-and-equal pair verifies. The `None`/`None`
    // case is the one a naive `recorded == current` implementation gets
    // wrong - it compares equal while proving nothing, which is precisely
    // the legacy-entry-after-a-hard-reboot shape that lets an arbitrary
    // process be SIGKILLed.
    #[test]
    fn boot_relative_platform_verifies_only_a_present_and_equal_boot_identity() {
        assert!(
            boot_identity_verified(true, Some("boot-a"), Some("boot-a")),
            "a matching present boot identity must verify"
        );
        assert!(
            !boot_identity_verified(true, Some("boot-a"), Some("boot-b")),
            "a different boot's entry must never verify"
        );
        assert!(
            !boot_identity_verified(true, None, Some("boot-a")),
            "a legacy entry with no recorded boot identity is unverifiable, not trusted"
        );
        assert!(
            !boot_identity_verified(true, Some("boot-a"), None),
            "an unreadable current boot identity is unverifiable, not assumed-matching"
        );
        assert!(
            !boot_identity_verified(true, None, None),
            "two absent boot identities must NOT compare equal into a verified result"
        );
    }

    // CONTRACT (boot-identity gate, no-regression leg): platforms whose
    // start times are absolute (macOS, Windows) must not be made worse -
    // the recorded field is not consulted, so pre-existing entries keep
    // verifying. Fails if the gate is ever applied unconditionally.
    #[test]
    fn absolute_start_time_platform_never_consults_the_recorded_boot_identity() {
        for recorded in [None, Some("boot-a"), Some("boot-b")] {
            for current in [None, Some("boot-a"), Some("boot-b")] {
                assert!(
                    boot_identity_verified(false, recorded, current),
                    "an absolute-start-time platform must always pass the gate \
                     (recorded={recorded:?}, current={current:?})"
                );
            }
        }
    }
}
