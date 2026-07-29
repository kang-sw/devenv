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
//
// AMENDED (260729 helper liveness probe): the second line above used to read
// "IPC-dead", where "dead" meant `connect_and_handshake` returned `None` from
// any of six failure paths. That was the defect: a HEALTHY, BUSY helper -
// one serving another daemon's session, since `serve_connections` serves one
// connection at a time while its listener stays bound - produces exactly
// that `None`. The kill line is now:
//
//   Kill = identity-verified-ours && POSITIVE evidence of unreachability,
//   where positive evidence is either no listener at all, or (only for a
//   helper whose registry entry declares the probe capability) a probe that
//   went unanswered or came back reporting nobody attached for longer than
//   `UNATTACHED_GRACE`.
//
// An eighth outcome follows from that: `ReconcileRow::Leave` - entry
// untouched, process untouched. It is a real row, not the absence of a kill;
// see its own doc comment for why reusing a drop-only row would be worse
// than the bug.

use std::time::Duration;

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
    /// CONTRACT (260729): POSITIVE ABSENCE - `connect` itself failed, so
    /// there is no listener behind the socket path (it is gone, or nothing
    /// is bound to it). This is the only reachability signal that authorizes
    /// a kill on its own.
    ///
    /// Split out of the former single `Unreachable` because that variant
    /// conflated it with the case below, which is the defect this ticket
    /// exists to fix: `connect_and_handshake` returned a bare `None` from
    /// six distinct failure paths and `reconcile_entry` folded all of them
    /// into one value.
    NoListener,
    /// CONTRACT (260729): the connection was ACCEPTED but no handshake
    /// arrived within the budget. This is the signature of a HEALTHY, BUSY
    /// helper - `serve_connections` serves one session at a time and the
    /// listener stays bound while it does, so another daemon's connect is
    /// queued, not refused. It is ALSO the signature of a wedged helper.
    /// The two are indistinguishable at this layer by construction, which is
    /// why the probe (below) exists: only the helper can break the tie.
    ConnectedButSilent,
}

/// CONTRACT (260729): the answer to "is this helper reachable?", which the
/// pre-ticket code could not ask at all. Only consulted for
/// `IpcStatus::ConnectedButSilent`, where reachability alone is undecidable.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ProbeVerdict {
    /// The registry entry declares no probe capability, so no probe was sent
    /// (sending one would SIGKILL that helper's shell - see
    /// `TerminalRegistryEntry::supports_liveness_probe`). "Connected but
    /// silent" is therefore undecidable for this helper, and the outcome is
    /// leave-alone. Kill only on positive absence.
    Unsupported,
    /// The entry declares the capability, the probe was sent, and nothing
    /// came back within the budget: wedged, deadlocked, or gone. Nothing
    /// else reclaims this.
    Unanswered,
    /// The helper answered and reports a session attached: somebody owns it
    /// and is using it. NEVER kill.
    Attached,
    /// The helper answered, reports nobody attached, but for less than the
    /// grace: its daemon is restarting, or is mid-`boot_reconcile`. Leave.
    UnattachedWithinGrace,
    /// The helper answered, reports nobody attached, past the grace: a real
    /// orphan holding a live shell and a PTY, with no daemon coming back.
    /// This leg is not decoration - a naive "kill only when the probe does
    /// not answer" predicate leaks exactly this case forever, because an
    /// orphan with a live shell keeps its listener bound and DOES answer.
    UnattachedPastGrace,
    /// CONTRACT (260729 review round 3, finding C): THIS DAEMON gave up on the
    /// exchange - it hit `PROBE_EXCHANGE_TOTAL_TIMEOUT` or
    /// `MAX_PROBE_EXCHANGE_MESSAGES` while the peer was still talking.
    ///
    /// Distinct from `Unanswered` on purpose, and the distinction is the whole
    /// point of the variant. Those two bounds exist to protect the DAEMON (a
    /// peer dripping one line every <5s otherwise holds `boot_reconcile` -
    /// which runs before the router binds - and the reaper loop forever); they
    /// are not measurements of the helper. Abandoning an exchange produces no
    /// evidence of unreachability, and this ticket's governing rule is that a
    /// kill needs POSITIVE evidence. Mapping this onto `Unanswered` would
    /// manufacture a SIGKILL out of the daemon's own impatience - the same
    /// mistake finding F10 corrected for the connect budget, one level up.
    Abandoned,
}

/// CONTRACT (260729): the unattached window a helper must clear before it
/// counts as an orphan. Must comfortably exceed a full daemon restart -
/// including the interval during which the restarting daemon's own
/// `boot_reconcile` has not yet re-adopted its helpers - or another daemon's
/// 10s sweep reaps live terminals during that restart, which is this
/// ticket's bug in a narrower form.
///
/// This deliberately SUPERSEDES `terminal.rs::EVICTION_BACKSTOP_GRACE` (30s):
/// an evicted helper that fails to self-exit now lingers for this window
/// instead. That is the accepted cost of not reaping during a restart; do
/// not "fix" it by shortening this below a restart window.
pub const UNATTACHED_GRACE: Duration = Duration::from_secs(120);

/// Maps a helper's self-report onto the three-way predicate.
///
/// `unattached_for_ms == None` alongside `attached == false` is read as
/// WITHIN grace, never as an orphan: a helper that cannot say how long it
/// has been unattached has not given evidence of orphanhood, and the
/// conservative direction here is "do not kill".
pub fn probe_verdict_from_report(
    attached: bool,
    unattached_for_ms: Option<u64>,
    grace_ms: u64,
) -> ProbeVerdict {
    if attached {
        return ProbeVerdict::Attached;
    }
    match unattached_for_ms {
        Some(elapsed_ms) if elapsed_ms >= grace_ms => ProbeVerdict::UnattachedPastGrace,
        _ => ProbeVerdict::UnattachedWithinGrace,
    }
}

/// CONTRACT (260729): the single predicate shared by every site that decides
/// whether a helper it has NOT handshaked with may be reclaimed - the
/// boot-reconcile `ConnectedButSilent` row (`classify`), the periodic sweep
/// (Site A), and `agent_profile_gc`, which is not a kill site at all but
/// destroys state off the same signal. Having one function is what stops the
/// three drifting apart; the GC in particular bypasses `classify` entirely.
pub fn probe_authorizes_reclaim(probe: ProbeVerdict) -> bool {
    match probe {
        ProbeVerdict::Unanswered | ProbeVerdict::UnattachedPastGrace => true,
        ProbeVerdict::Unsupported
        | ProbeVerdict::Attached
        | ProbeVerdict::UnattachedWithinGrace
        | ProbeVerdict::Abandoned => false,
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ReconcileRow {
    AdoptLive,
    AdoptGrace,
    DropNoSuchProcess,
    KillVerified,
    DropPidReused,
    DropUnverifiableBoot,
    /// CONTRACT (260729): entry untouched, process untouched. A REAL
    /// outcome, not the absence of a kill - and it had to be added, because
    /// no existing row expresses it. Mapping "do not kill" onto a drop-only
    /// row would spare the helper AND delete its registry entry, which is
    /// worse than the bug it fixes: the helper keeps running but its owning
    /// daemon can no longer adopt it at the next `boot_reconcile`, so it
    /// becomes a permanent orphan holding a PTY and a shell.
    Leave,
}

/// `ipc` is only consulted when `identity` is `VerifiedOurs` - an
/// unverified identity is dropped without ever considering IPC reachability
/// (the daemon-side driver in `terminal.rs` mirrors this by skipping the
/// connect attempt entirely for a failed identity check, not merely
/// ignoring its result).
///
/// `probe` is only consulted for `IpcStatus::ConnectedButSilent`, the one
/// reachability signal that cannot decide on its own. `None` means "no probe
/// was performed" and resolves to `Leave`: a caller that did not ask has no
/// evidence, and no evidence must never authorize a SIGKILL.
pub fn classify(
    identity: IdentityStatus,
    ipc: IpcStatus,
    probe: Option<ProbeVerdict>,
) -> ReconcileRow {
    match identity {
        IdentityStatus::NoSuchProcess => ReconcileRow::DropNoSuchProcess,
        IdentityStatus::PidReused => ReconcileRow::DropPidReused,
        IdentityStatus::UnverifiableBoot => ReconcileRow::DropUnverifiableBoot,
        IdentityStatus::VerifiedOurs => match ipc {
            IpcStatus::ReachableShellAlive => ReconcileRow::AdoptLive,
            IpcStatus::ReachableShellExited => ReconcileRow::AdoptGrace,
            IpcStatus::NoListener => ReconcileRow::KillVerified,
            IpcStatus::ConnectedButSilent => match probe {
                Some(probe) if probe_authorizes_reclaim(probe) => ReconcileRow::KillVerified,
                _ => ReconcileRow::Leave,
            },
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const EVERY_IPC_STATUS: [IpcStatus; 4] = [
        IpcStatus::ReachableShellAlive,
        IpcStatus::ReachableShellExited,
        IpcStatus::NoListener,
        IpcStatus::ConnectedButSilent,
    ];

    const EVERY_PROBE_VERDICT: [ProbeVerdict; 6] = [
        ProbeVerdict::Unsupported,
        ProbeVerdict::Unanswered,
        ProbeVerdict::Attached,
        ProbeVerdict::UnattachedWithinGrace,
        ProbeVerdict::UnattachedPastGrace,
        ProbeVerdict::Abandoned,
    ];

    #[test]
    fn row_1_adopts_live_only_for_reachable_verified_identity_with_alive_shell() {
        assert_eq!(
            classify(
                IdentityStatus::VerifiedOurs,
                IpcStatus::ReachableShellAlive,
                None
            ),
            ReconcileRow::AdoptLive
        );
    }

    #[test]
    fn row_2_adopts_grace_for_reachable_verified_identity_with_exited_shell() {
        assert_eq!(
            classify(
                IdentityStatus::VerifiedOurs,
                IpcStatus::ReachableShellExited,
                None
            ),
            ReconcileRow::AdoptGrace
        );
    }

    #[test]
    fn row_3_drops_without_killing_when_no_such_process_exists() {
        // NEVER kill: `ipc` is deliberately fed every possible value here to
        // prove the outcome does not depend on it once identity fails.
        for ipc in EVERY_IPC_STATUS {
            assert_eq!(
                classify(IdentityStatus::NoSuchProcess, ipc, None),
                ReconcileRow::DropNoSuchProcess,
                "no-such-process must always drop-only, never kill (ipc={ipc:?})"
            );
        }
    }

    #[test]
    fn row_4_kills_verified_only_when_identity_ours_and_there_is_no_listener() {
        assert_eq!(
            classify(IdentityStatus::VerifiedOurs, IpcStatus::NoListener, None),
            ReconcileRow::KillVerified
        );
    }

    #[test]
    fn row_5_drops_without_killing_on_pid_reuse() {
        for ipc in EVERY_IPC_STATUS {
            assert_eq!(
                classify(IdentityStatus::PidReused, ipc, None),
                ReconcileRow::DropPidReused,
                "pid-reuse must always drop-only, never kill (ipc={ipc:?})"
            );
        }
    }

    // CONTRACT (260729): the headline correction. "Connected, unanswered" is
    // the signature of a healthy, busy helper serving ANOTHER daemon, and
    // before this ticket it collapsed into the same `Unreachable` value as
    // "socket gone" and went straight to `KillVerified`. It must now depend
    // entirely on what the helper itself says.
    #[test]
    fn connected_but_silent_kills_only_on_a_probe_verdict_that_authorizes_reclaim() {
        let expected = [
            // A helper that predates the probe cannot answer, so silence
            // proves nothing about it - leave it alone.
            (ProbeVerdict::Unsupported, ReconcileRow::Leave),
            (ProbeVerdict::Unanswered, ReconcileRow::KillVerified),
            (ProbeVerdict::Attached, ReconcileRow::Leave),
            (ProbeVerdict::UnattachedWithinGrace, ReconcileRow::Leave),
            (ProbeVerdict::UnattachedPastGrace, ReconcileRow::KillVerified),
            // Round 3 finding C: the daemon's own bounds firing is not
            // evidence about the helper. See `ProbeVerdict::Abandoned`.
            (ProbeVerdict::Abandoned, ReconcileRow::Leave),
        ];
        for (probe, row) in expected {
            assert_eq!(
                classify(
                    IdentityStatus::VerifiedOurs,
                    IpcStatus::ConnectedButSilent,
                    Some(probe)
                ),
                row,
                "connected-but-silent + {probe:?} must resolve to {row:?}"
            );
        }
    }

    // CONTRACT (260729, the leave-alone default): a caller that did not
    // probe has no evidence, and no evidence must never authorize a SIGKILL.
    // The pre-ticket behaviour for this exact input was `KillVerified`.
    #[test]
    fn connected_but_silent_without_a_probe_result_leaves_the_helper_alone() {
        assert_eq!(
            classify(
                IdentityStatus::VerifiedOurs,
                IpcStatus::ConnectedButSilent,
                None
            ),
            ReconcileRow::Leave,
            "an unprobed connected-but-silent helper must never be killed"
        );
    }

    // Non-vacuity for the two rows above: `Leave` must be genuinely
    // conditional, not a blanket "never kill anything that answered a
    // socket". Positive absence still kills regardless of any probe verdict,
    // including the ones that spare a connected helper.
    #[test]
    fn no_listener_still_kills_for_every_probe_verdict() {
        for probe in EVERY_PROBE_VERDICT {
            assert_eq!(
                classify(
                    IdentityStatus::VerifiedOurs,
                    IpcStatus::NoListener,
                    Some(probe)
                ),
                ReconcileRow::KillVerified,
                "positive absence of a listener must kill regardless of {probe:?}"
            );
        }
    }

    // CONTRACT (boot-identity gate): an unverifiable boot identity is a
    // drop-only row for every IPC value, exactly like rows 3 and 5. Fails
    // loudly if anyone maps `UnverifiableBoot` onto the kill row.
    #[test]
    fn unverifiable_boot_identity_drops_without_killing_for_every_ipc_status() {
        for ipc in EVERY_IPC_STATUS {
            let row = classify(IdentityStatus::UnverifiableBoot, ipc, None);
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

    // CONTRACT (260729): the predicate is THREE-way. The naive two-way rule
    // ("kill only when the probe does not answer") is wrong in the exact
    // place the Background says nothing else covers: an orphaned helper with
    // a live shell keeps its listener bound and sits in its idle accept loop
    // forever, so it ANSWERS - and under the naive rule its shell and PTY
    // leak permanently and silently. The `attached` bit plus the grace is
    // what separates that orphan from a healthy helper.
    #[test]
    fn the_unattached_grace_separates_a_restarting_daemons_helper_from_a_real_orphan() {
        let grace_ms = 60_000;
        assert_eq!(
            probe_verdict_from_report(true, None, grace_ms),
            ProbeVerdict::Attached,
            "an attached helper is never an orphan, whatever any clock says"
        );
        assert_eq!(
            probe_verdict_from_report(true, Some(u64::MAX), grace_ms),
            ProbeVerdict::Attached,
            "`attached` is authoritative and must dominate the duration"
        );
        assert_eq!(
            probe_verdict_from_report(false, Some(grace_ms - 1), grace_ms),
            ProbeVerdict::UnattachedWithinGrace,
            "a helper whose daemon is mid-restart must be left alone"
        );
        assert_eq!(
            probe_verdict_from_report(false, Some(grace_ms), grace_ms),
            ProbeVerdict::UnattachedPastGrace,
            "the grace boundary itself counts as elapsed"
        );
        assert_eq!(
            probe_verdict_from_report(false, Some(grace_ms * 10), grace_ms),
            ProbeVerdict::UnattachedPastGrace,
            "a long-unattached helper is a real orphan and must be reclaimable"
        );
        assert_eq!(
            probe_verdict_from_report(false, None, grace_ms),
            ProbeVerdict::UnattachedWithinGrace,
            "a helper that cannot say how long it has been unattached has given no evidence \
             of orphanhood - the conservative direction is leave-alone"
        );
    }

    // CONTRACT (260729): `agent_profile_gc` is not a kill site and never
    // routes through `classify`, but it destroys state off the same signal -
    // so it shares this predicate rather than re-deriving one. Pinning the
    // shared function directly is what stops the two drifting apart.
    #[test]
    fn probe_authorizes_reclaim_matches_the_three_way_predicate_exactly() {
        assert!(probe_authorizes_reclaim(ProbeVerdict::Unanswered));
        assert!(probe_authorizes_reclaim(ProbeVerdict::UnattachedPastGrace));
        assert!(!probe_authorizes_reclaim(ProbeVerdict::Attached));
        assert!(!probe_authorizes_reclaim(ProbeVerdict::UnattachedWithinGrace));
        assert!(
            !probe_authorizes_reclaim(ProbeVerdict::Unsupported),
            "a helper that predates the probe must never be reclaimed for staying silent"
        );
        assert!(
            !probe_authorizes_reclaim(ProbeVerdict::Abandoned),
            "the daemon hitting its OWN total/message bound says nothing about the helper - \
             turning impatience into a SIGKILL is the F10 mistake one level up"
        );
    }

    // CONTRACT (260729): the grace must comfortably clear a daemon restart,
    // INCLUDING the window in which the restarting daemon's own
    // `boot_reconcile` has not yet re-adopted its helpers. It therefore also
    // supersedes the 30s `EVICTION_BACKSTOP_GRACE`. A future tweak that
    // shortens it below a restart window reintroduces this ticket's bug in a
    // narrower form, so the floor is asserted rather than left to a comment.
    #[test]
    fn the_unattached_grace_comfortably_exceeds_a_daemon_restart_window() {
        assert!(
            UNATTACHED_GRACE >= Duration::from_secs(60),
            "a grace shorter than a minute cannot cover a daemon restart plus its \
             boot_reconcile; another daemon's 10s sweep would reap live terminals during it"
        );
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
