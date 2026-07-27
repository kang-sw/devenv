// Per-terminal `terminal-notify` delivery-failure record (260726 Phase 1):
// the one bridge across the deliberate stdio silence that
// `terminal_notify.rs`'s module CONTRACT imposes on the hook process.
//
// CONTRACT (why this module exists at all): the hook process must never
// print to stdout/stderr and must never exit non-zero - a real-PTY
// measurement proved either one turns into per-turn visible noise inside the
// user's live agent session (see `terminal_notify.rs`'s module CONTRACT for
// the full evidence). Before this module, that silence was total: a
// callback target that had gone permanently bad (stale port, wrong token,
// unparseable file) failed on EVERY turn boundary forever and nothing but a
// hand-read of `logs/terminal-notify.log` could ever reveal it. This module
// leaves the hook process just as silent on stdio while dropping one small
// piece of durable state next to the callback file, so the DAEMON - a
// process that is allowed to be loud - can notice and say so once.
//
// CONTRACT (writer/reader split - the two halves live in different
// processes): the WRITER half (`record_failure`/`clear_record`) runs inside
// the short-lived hook process and carries NO policy at all: it counts,
// stamps, and clears, and it swallows every error it meets. The READER half
// (`read_record` plus the pure `NotifyFailureWatch`) runs inside the daemon's
// GC sweep and owns ALL policy - the grace window, the self-heal check, and
// the warn-once bookkeeping. Do not move any threshold or timing decision
// into the writer: the hook process has no way to know how often the daemon
// looks, and a policy split across two processes cannot be tested as one.
//
// CONTRACT (keyed by PATH SIBLING, never by a parsed terminal id): the
// record lives beside the `callback.json` the hook was pointed at, derived
// from `args.callback.parent()`. It is deliberately NOT keyed by
// `CallbackTarget::terminal_id`, because the single most likely permanent
// failure - an unreadable or unparseable callback file - is exactly the case
// where no terminal id can be parsed at all. A terminal-id-keyed record
// would go silent in precisely the case it exists to report.
//
// CONTRACT (the writer must NEVER create the profile directory): the
// `agent-profiles/<id>/` directory is owned by the terminal's lifecycle and
// reclaimed by `agent_profile_gc`'s sweep the moment its terminal stops
// being live. A `create_dir_all` here would let a hook fire arriving after
// that reclaim resurrect the directory the GC just deleted, and the next
// sweep would reclaim it again, forever. When the directory is absent,
// `record_failure` does nothing at all.
//
// CONTRACT (warn-once is held in an explicit `&mut NotifyFailureWatch`,
// never a module static): the watch is owned by the GC sweep task in
// `server.rs`, so it dies with that task on the daemon's `.abort()` shutdown
// path and every test can construct its own. A module static would make the
// warn-once state process-global and untestable in parallel.

use std::collections::{HashMap, HashSet};
use std::fs;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

/// The durable record the hook process leaves beside `callback.json`.
///
/// `last_error` is `log_failure`'s own error string verbatim (same binding,
/// not a re-rendered copy), truncated only for size - see
/// `truncate_last_error`.
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct NotifyFailureRecord {
    pub count: u32,
    pub last_failure_at_ms: u64,
    pub last_error: String,
}

/// Sibling of `callback.json` inside a terminal's `agent-profiles/<id>/`
/// directory.
pub fn notify_failure_path(profile_dir: &Path) -> PathBuf {
    profile_dir.join("notify-failures.json")
}

// Bounds one pathological error string (a server echoing a large body into
// the message, say) from growing this file without limit. The record is
// rewritten on every failure, so this cap is per-file, not cumulative.
const MAX_LAST_ERROR_BYTES: usize = 512;

/// Truncates on a `char` boundary at or below `MAX_LAST_ERROR_BYTES`, never
/// mid-code-point (which would either panic on a naive slice or produce a
/// U+FFFD when re-read).
fn truncate_last_error(error: &str) -> String {
    if error.len() <= MAX_LAST_ERROR_BYTES {
        return error.to_owned();
    }
    let mut end = MAX_LAST_ERROR_BYTES;
    while end > 0 && !error.is_char_boundary(end) {
        end -= 1;
    }
    error[..end].to_owned()
}

/// Best-effort read. Every failure class - file absent, unreadable, or
/// malformed - collapses to `None`, because the reader's only question is
/// "is there a live failure record here", and a corrupt file answers "no"
/// just as safely as a missing one.
pub fn read_record(profile_dir: &Path) -> Option<NotifyFailureRecord> {
    let raw = fs::read_to_string(notify_failure_path(profile_dir)).ok()?;
    serde_json::from_str(&raw).ok()
}

/// Increments (or creates) the failure record beside `callback.json`.
///
/// CONTRACT: this function is INFALLIBLE by signature and silent by
/// construction. It returns nothing, prints nothing, and cannot change its
/// caller's exit status - see the module CONTRACT. Every I/O error on the
/// path below is swallowed deliberately; a failure to record a failure must
/// never become the visible per-turn noise the whole design exists to avoid.
pub fn record_failure(profile_dir: &Path, error: &str, now_ms: u64) {
    // Never create the profile directory - see the module CONTRACT. This is
    // checked up front rather than leaning on `create_new`'s `NotFound`,
    // because the check is the documented behavior, not an incidental
    // consequence of the writer's error path.
    if !profile_dir.is_dir() {
        return;
    }
    let previous_count = read_record(profile_dir).map(|record| record.count).unwrap_or(0);
    let record = NotifyFailureRecord {
        count: previous_count.saturating_add(1),
        last_failure_at_ms: now_ms,
        last_error: truncate_last_error(error),
    };
    let Ok(raw) = serde_json::to_string_pretty(&record) else {
        return;
    };
    let path = notify_failure_path(profile_dir);
    // CONTRACT: the temp name must be UNIQUE, not the fixed
    // `notify-failures.json.tmp` shape `write_callback_target` gets away
    // with. That writer's fixed name is safe only because its file is keyed
    // by a per-spawn-random terminal id; this writer is keyed by a directory
    // path shared by every hook process firing for that terminal, and a
    // vendor CLI fires `UserPromptSubmit` and `Stop` hooks that can overlap.
    // Two overlapping writers on one fixed temp name would interleave
    // truncate-then-write and publish a TORN file, which `read_record` then
    // reads back as "no record" - silently restoring the exact silence this
    // phase removes.
    let temp_path = crate::agent_callback::unique_temp_path(&path);
    // Create at `0600` directly rather than write-then-chmod (the sequence
    // `write_bound_base_url` uses and its own forward-note warns against
    // copying): this file sits beside a token-bearing `callback.json` in a
    // directory whose contents are already treated as sensitive.
    //
    // KNOWN, accepted: a process killed between the create below and the
    // rename leaves one `notify-failures.json.tmp.*` file behind in a LIVE
    // profile directory, which nothing reclaims (the GC sweep only removes
    // whole orphan directories, and this directory is not one). It is inert -
    // `read_record` reads only the published name - bounded to one file per
    // crashed write, and is the same exposure `write_bound_base_url` already
    // carries. Deliberately no reclaim path.
    if crate::agent_token_store::create_new_file_at_mode_0600(&temp_path, raw.as_bytes()).is_err() {
        let _ = fs::remove_file(&temp_path);
        return;
    }
    if fs::rename(&temp_path, &path).is_err() {
        let _ = fs::remove_file(&temp_path);
    }
}

/// Drops the record after a successful delivery. Silent and best-effort for
/// the same reason `record_failure` is: the common case is that no record
/// exists, which `fs::remove_file` reports as an error we do not care about.
pub fn clear_record(profile_dir: &Path) {
    // The same guard `record_failure` carries, for a reason that is specific
    // to the deleting half: `args.callback.parent()` yields the EMPTY path for
    // a relative `--callback` argument, and `Path::new("").join(...)` is the
    // bare relative name `notify-failures.json` - which would resolve against
    // the hook process's CWD and delete a same-named file that is not ours.
    // The materialized hook config always passes an absolute path, so this is
    // not reachable in production; the two functions' guards must still match.
    if !profile_dir.is_dir() {
        return;
    }
    let _ = fs::remove_file(notify_failure_path(profile_dir));
}

/// What was observed at the moment a terminal was warned about, kept so a
/// LATER observation can be told apart from the one already reported.
///
/// CONTRACT (why both fields, not just one): the pair is the only available
/// discriminator between "the same unrepaired failure, seen again" and "a
/// second, distinct breakage after a self-heal". See
/// `NotifyFailureWatch::should_warn`'s doc comment for the derivation.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
struct WarnedObservation {
    count: u32,
    last_failure_at_ms: u64,
}

/// The daemon-side warn-once bookkeeping for the escalation rule.
///
/// Owned by the GC sweep task (`server.rs`) and threaded in as `&mut` - see
/// the module CONTRACT on why this is never a module static.
#[derive(Debug, Default)]
pub struct NotifyFailureWatch {
    warned: HashMap<String, WarnedObservation>,
}

impl NotifyFailureWatch {
    /// The escalation rule, as a PURE function over its arguments plus this
    /// struct's warn-once set: no filesystem access, no clock read, no
    /// registry access. `now_ms` and `grace_ms` are parameters precisely so
    /// the rule can be tested across a 300-second grace window at zero
    /// wall-clock cost.
    ///
    /// Warns only when all three conditions hold at once:
    ///   1. a record exists with `count >= 1`;
    ///   2. the last failure is at least `grace_ms` old (one full sweep
    ///      period has passed with no repair);
    ///   3. `callback.json`'s mtime is NOT newer than the last failure - a
    ///      rewritten callback target means the situation may have already
    ///      self-healed and the next hook fire will tell us. A missing or
    ///      unreadable mtime counts as "not superseded": absence of evidence
    ///      of repair must not be read as evidence of repair.
    ///
    /// There are THREE drop triggers for the warned flag, and the third one
    /// is load-bearing rather than belt-and-braces:
    ///
    ///   a. the id is next observed with no record or `count == 0` (a
    ///      successful delivery deleted the record) - handled below;
    ///   b. the id leaves the live set - handled by `retain_live`;
    ///   c. the observation shows the failure COUNTER WAS RESET since the
    ///      warning was emitted - `counter_was_reset` below.
    ///
    /// CONTRACT (why (a) alone is not enough, and why (c) uses the pair it
    /// uses): trigger (a) only fires if a sweep happens to sample the window
    /// in which the record is absent. In the ticket's headline sequence -
    /// broke, warned, owner returned and a delivery succeeded, then broke
    /// again - that window is one turn long against a 300 s sweep period, so
    /// (a) almost never lands and the second, distinct breakage would be
    /// swallowed. The ticket's `## Decisions` states it must not be.
    ///
    /// The discriminator is that `clear_record` DELETES the record, so a
    /// post-clear failure restarts the count at 1 while stamping a fresh
    /// timestamp. Without a clear, the two fields always advance TOGETHER
    /// (each new failure both increments the count and re-stamps the time).
    /// A timestamp that advanced while the count did NOT can therefore only
    /// mean the counter was reset by a successful delivery in between. Note
    /// that comparing the count alone does not work: the reset restarts at 1,
    /// so a first breakage warned about at `count == 1` - the idle-owner case
    /// this whole mechanism exists for - is followed by a second breakage
    /// also at `count == 1`, and no count-only comparison can separate them.
    ///
    /// KNOWN, accepted weakness (do not "fix" it): the writer's
    /// read-modify-write of the count is not atomic, so two overlapping hook
    /// processes can lose an increment - which looks exactly like a reset
    /// (timestamp advanced, count did not) and costs one extra warning. That
    /// is the same benign direction as the residual false positive the ticket
    /// already accepts by name: one extra line in an operator-facing log. The
    /// alternative is the silent swallow above.
    pub fn should_warn(
        &mut self,
        terminal_id: &str,
        record: Option<&NotifyFailureRecord>,
        callback_mtime_ms: Option<u64>,
        now_ms: u64,
        grace_ms: u64,
    ) -> bool {
        let Some(record) = record.filter(|record| record.count > 0) else {
            // Drop trigger (a): the failure is gone (record cleared by a
            // successful delivery, or never existed), so a future
            // recurrence deserves a fresh warning.
            self.warned.remove(terminal_id);
            return false;
        };
        let aged_enough = now_ms.saturating_sub(record.last_failure_at_ms) >= grace_ms;
        let not_superseded =
            callback_mtime_ms.is_none_or(|mtime_ms| mtime_ms <= record.last_failure_at_ms);
        if !aged_enough || !not_superseded {
            return false;
        }
        match self.warned.get(terminal_id) {
            // Never warned about this id, or a drop trigger already fired.
            None => {}
            // Drop trigger (c): a distinct breakage after a self-heal.
            Some(previous) if counter_was_reset(previous, record) => {}
            // The same unrepaired failure, seen on a later sweep.
            Some(_) => return false,
        }
        self.warned.insert(
            terminal_id.to_owned(),
            WarnedObservation {
                count: record.count,
                last_failure_at_ms: record.last_failure_at_ms,
            },
        );
        true
    }

    /// Drop trigger: an id that has left the live terminal set forgets its
    /// warned flag, so a terminal id observed live again later can warn
    /// again rather than being suppressed for the daemon's whole lifetime.
    pub fn retain_live(&mut self, live_ids: &HashSet<String>) {
        self.warned.retain(|id, _| live_ids.contains(id));
    }
}

/// True when the record observed now must have come from a counter reset
/// since `previous` was warned about - see `should_warn`'s CONTRACT for the
/// derivation and for the one accepted false positive (a lost increment from
/// two overlapping hook writers).
fn counter_was_reset(previous: &WarnedObservation, observed: &NotifyFailureRecord) -> bool {
    observed.last_failure_at_ms > previous.last_failure_at_ms && observed.count <= previous.count
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicU64, Ordering};

    const GRACE_MS: u64 = 300_000;

    static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

    fn temp_dir(label: &str) -> PathBuf {
        let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
        std::env::temp_dir().join(format!(
            "ws-dashboard-notify-failure-{label}-{}-{unique}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time after epoch")
                .as_nanos()
        ))
    }

    fn record(count: u32, last_failure_at_ms: u64) -> NotifyFailureRecord {
        NotifyFailureRecord {
            count,
            last_failure_at_ms,
            last_error: "POST to http://127.0.0.1:1/... failed".to_owned(),
        }
    }

    // ---- escalation rule (pure, no filesystem, no clock) ----

    #[test]
    fn should_warn_is_false_for_a_record_whose_count_is_zero() {
        let mut watch = NotifyFailureWatch::default();
        let aged = record(0, 0);
        assert!(!watch.should_warn("t1", Some(&aged), None, GRACE_MS * 2, GRACE_MS));
    }

    #[test]
    fn should_warn_is_false_when_no_record_exists_at_all() {
        let mut watch = NotifyFailureWatch::default();
        assert!(!watch.should_warn("t1", None, None, GRACE_MS * 2, GRACE_MS));
    }

    #[test]
    fn should_warn_is_false_while_the_failure_is_younger_than_the_grace_window() {
        let mut watch = NotifyFailureWatch::default();
        let young = record(1, 1_000_000);
        assert!(!watch.should_warn(
            "t1",
            Some(&young),
            None,
            1_000_000 + GRACE_MS - 1,
            GRACE_MS
        ));
    }

    #[test]
    fn should_warn_is_false_when_the_callback_file_was_rewritten_after_the_failure() {
        let mut watch = NotifyFailureWatch::default();
        let aged = record(1, 1_000_000);
        // A callback.json newer than the failure means the target may have
        // just been re-pointed (e.g. a daemon restart rewrote it) and the
        // next hook fire will settle it - do not warn yet.
        assert!(!watch.should_warn(
            "t1",
            Some(&aged),
            Some(1_000_001),
            1_000_000 + GRACE_MS,
            GRACE_MS
        ));
    }

    #[test]
    fn should_warn_is_true_when_the_callback_mtime_is_unavailable() {
        let mut watch = NotifyFailureWatch::default();
        let aged = record(1, 1_000_000);
        // Missing/unreadable mtime must NOT suppress: an absent callback
        // file is one of the failure modes this warning exists to surface.
        assert!(watch.should_warn("t1", Some(&aged), None, 1_000_000 + GRACE_MS, GRACE_MS));
    }

    // THE mechanism's whole reason to exist: an idle agent whose owner has
    // stopped typing produces exactly ONE failed hook fire and then nothing
    // more, forever. A count threshold of 2 or 3 would never fire here.
    #[test]
    fn should_warn_is_true_for_an_idle_owners_single_aged_unrepaired_failure() {
        let mut watch = NotifyFailureWatch::default();
        let single = record(1, 1_000_000);
        assert!(
            watch.should_warn(
                "t1",
                Some(&single),
                Some(999_000),
                1_000_000 + GRACE_MS,
                GRACE_MS
            ),
            "count == 1 must be enough - an idle terminal never reaches a higher count"
        );
    }

    #[test]
    fn should_warn_warns_once_and_then_suppresses_the_identical_observation() {
        let mut watch = NotifyFailureWatch::default();
        let aged = record(3, 1_000_000);
        let now = 1_000_000 + GRACE_MS;
        assert!(watch.should_warn("t1", Some(&aged), Some(999_000), now, GRACE_MS));
        assert!(
            !watch.should_warn("t1", Some(&aged), Some(999_000), now + GRACE_MS, GRACE_MS),
            "the same unrepaired failure must not warn on every subsequent sweep"
        );
    }

    #[test]
    fn should_warn_rewarns_after_the_record_is_cleared_and_the_failure_recurs() {
        let mut watch = NotifyFailureWatch::default();
        let first = record(1, 1_000_000);
        assert!(watch.should_warn("t1", Some(&first), None, 1_000_000 + GRACE_MS, GRACE_MS));

        // A successful delivery cleared the record - this observation both
        // returns false and drops the warned flag.
        assert!(!watch.should_warn("t1", None, None, 2_000_000, GRACE_MS));

        // The failure recurs later and must warn again rather than staying
        // suppressed for the daemon's whole lifetime.
        let second = record(1, 3_000_000);
        assert!(watch.should_warn("t1", Some(&second), None, 3_000_000 + GRACE_MS, GRACE_MS));
    }

    // THE case the drop rule exists for, and the one a count-only rule cannot
    // solve: the cleared window is one turn long against a 300 s sweep, so no
    // observation ever sees the record absent - the warn-once flag has to be
    // dropped from the RE-BROKEN record alone.
    #[test]
    fn should_warn_rewarns_when_a_self_heal_no_sweep_ever_observed_restarted_the_count_at_one() {
        let mut watch = NotifyFailureWatch::default();
        let first = record(1, 1_000_000);
        assert!(watch.should_warn("t1", Some(&first), None, 1_000_000 + GRACE_MS, GRACE_MS));

        // Owner returned, a delivery succeeded (clearing the record), the
        // next turn broke again - all between two sweeps. The record is back
        // at count 1 with a LATER timestamp: the count did not advance, so
        // the counter must have been reset. A count-only comparison
        // ("observed count < warned count") reads `1 < 1` as false here and
        // swallows this second, distinct breakage forever.
        let second = record(1, 5_000_000);
        assert!(
            watch.should_warn("t1", Some(&second), None, 5_000_000 + GRACE_MS, GRACE_MS),
            "a fresh failure after a self-heal must warn again even though the count \
             restarted at the same value that was warned about"
        );

        // ...and the re-warn re-arms warn-once against the NEW observation.
        assert!(
            !watch.should_warn("t1", Some(&second), None, 6_000_000, GRACE_MS),
            "the re-warned observation must itself be suppressed on the next sweep"
        );
    }

    #[test]
    fn should_warn_rewarns_when_a_self_heal_no_sweep_ever_observed_restarted_the_count_from_five() {
        let mut watch = NotifyFailureWatch::default();
        let streak = record(5, 1_000_000);
        assert!(watch.should_warn("t1", Some(&streak), None, 1_000_000 + GRACE_MS, GRACE_MS));

        let after_self_heal = record(1, 5_000_000);
        assert!(
            watch.should_warn("t1", Some(&after_self_heal), None, 5_000_000 + GRACE_MS, GRACE_MS),
            "a count that fell back to 1 can only mean clear_record deleted the record in between"
        );
    }

    // The idle owner this mechanism exists for submits nothing more, so the
    // record FREEZES: same count, same timestamp, sweep after sweep. Neither
    // field advanced, so nothing was reset and warn-once must hold.
    #[test]
    fn should_warn_does_not_rewarn_for_the_frozen_record_of_an_idle_owner() {
        let mut watch = NotifyFailureWatch::default();
        let frozen = record(1, 1_000_000);
        let first_sweep = 1_000_000 + GRACE_MS;
        assert!(watch.should_warn("t1", Some(&frozen), None, first_sweep, GRACE_MS));

        for sweep in 1..=3 {
            let now = first_sweep + sweep * GRACE_MS;
            assert!(
                !watch.should_warn("t1", Some(&frozen), None, now, GRACE_MS),
                "an unrepaired failure that nothing has touched must warn exactly once"
            );
        }
    }

    // An owner who keeps working against a broken target advances BOTH fields
    // on every failure. That is an unbroken streak, not a reset, so warn-once
    // must hold no matter how far the count climbs.
    #[test]
    fn should_warn_does_not_rewarn_while_a_growing_streak_advances_both_fields() {
        let mut watch = NotifyFailureWatch::default();
        let first = record(1, 1_000_000);
        assert!(watch.should_warn("t1", Some(&first), None, 1_000_000 + GRACE_MS, GRACE_MS));

        for (count, last_failure_at_ms) in [(2, 2_000_000), (3, 3_000_000), (9, 4_000_000)] {
            let growing = record(count, last_failure_at_ms);
            assert!(
                !watch.should_warn(
                    "t1",
                    Some(&growing),
                    None,
                    last_failure_at_ms + GRACE_MS,
                    GRACE_MS
                ),
                "count {count} advanced along with the timestamp - the counter was never reset"
            );
        }
    }

    #[test]
    fn retain_live_drops_a_warned_id_that_left_the_live_set_so_it_can_warn_again() {
        let mut watch = NotifyFailureWatch::default();
        let aged = record(1, 1_000_000);
        let now = 1_000_000 + GRACE_MS;
        assert!(watch.should_warn("t1", Some(&aged), None, now, GRACE_MS));
        assert!(!watch.should_warn("t1", Some(&aged), None, now, GRACE_MS));

        watch.retain_live(&HashSet::new());

        assert!(
            watch.should_warn("t1", Some(&aged), None, now, GRACE_MS),
            "an id that left the live set must warn again once it is observed live and failing"
        );
    }

    #[test]
    fn retain_live_keeps_the_warned_flag_of_an_id_that_is_still_live() {
        let mut watch = NotifyFailureWatch::default();
        let aged = record(1, 1_000_000);
        let now = 1_000_000 + GRACE_MS;
        assert!(watch.should_warn("t1", Some(&aged), None, now, GRACE_MS));

        let live: HashSet<String> = ["t1".to_owned()].into_iter().collect();
        watch.retain_live(&live);

        assert!(
            !watch.should_warn("t1", Some(&aged), None, now, GRACE_MS),
            "a still-live id must keep its warned flag across sweeps"
        );
    }

    // ---- writer (real filesystem) ----

    #[test]
    fn record_failure_increments_the_count_across_consecutive_failures() {
        let dir = temp_dir("increment");
        fs::create_dir_all(&dir).expect("create profile dir");

        record_failure(&dir, "first failure", 111);
        let after_first = read_record(&dir).expect("record after first failure");
        assert_eq!(after_first.count, 1);
        assert_eq!(after_first.last_failure_at_ms, 111);
        assert_eq!(after_first.last_error, "first failure");

        record_failure(&dir, "second failure", 222);
        let after_second = read_record(&dir).expect("record after second failure");
        assert_eq!(after_second.count, 2);
        assert_eq!(after_second.last_failure_at_ms, 222);
        assert_eq!(after_second.last_error, "second failure");

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn record_failure_stores_the_error_string_verbatim_when_it_is_under_the_cap() {
        let dir = temp_dir("verbatim");
        fs::create_dir_all(&dir).expect("create profile dir");

        // The exact shape `deliver` produces, passed through unchanged - the
        // record and the log line must name the same failure text.
        let error = "POST to http://127.0.0.1:1/api/dashboard/terminals/t1/turn-state failed: \
                     error sending request";
        record_failure(&dir, error, 5);

        let stored = read_record(&dir).expect("record").last_error;
        assert_eq!(stored, error, "the error string must not be reformatted");

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn record_failure_truncates_an_oversized_error_on_a_char_boundary() {
        let dir = temp_dir("truncate");
        fs::create_dir_all(&dir).expect("create profile dir");

        // Three-byte code points, so byte 512 lands mid-character
        // (512 = 170 * 3 + 2) - a naive slice here would panic and a naive
        // byte truncation would produce a U+FFFD on re-read.
        let error: String = "\u{3042}".repeat(300);
        assert!(error.len() > MAX_LAST_ERROR_BYTES);
        record_failure(&dir, &error, 7);

        let stored = read_record(&dir).expect("record").last_error;
        assert!(
            stored.len() <= MAX_LAST_ERROR_BYTES,
            "stored error must fit the cap, got {} bytes",
            stored.len()
        );
        assert_eq!(stored.len(), 510, "must land on the nearest char boundary at or below the cap");
        assert!(
            !stored.contains('\u{FFFD}'),
            "truncation must never split a code point into a replacement char"
        );
        assert!(error.starts_with(&stored), "truncation must be a prefix of the original");

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn clear_record_removes_the_record_left_by_earlier_failures() {
        let dir = temp_dir("clear");
        fs::create_dir_all(&dir).expect("create profile dir");

        record_failure(&dir, "failure", 1);
        record_failure(&dir, "failure", 2);
        assert!(read_record(&dir).is_some());

        clear_record(&dir);

        assert!(read_record(&dir).is_none(), "a successful delivery must clear the record");
        assert!(!notify_failure_path(&dir).exists(), "the record file itself must be removed");

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn clear_record_on_a_profile_dir_with_no_record_is_a_silent_no_op() {
        let dir = temp_dir("clear-absent");
        fs::create_dir_all(&dir).expect("create profile dir");

        clear_record(&dir);

        assert!(read_record(&dir).is_none());

        let _ = fs::remove_dir_all(&dir);
    }

    // CONTRACT: the writer must never resurrect a profile directory the GC
    // sweep just reclaimed - see the module CONTRACT. This is also the live
    // shape `terminal_notify.rs`'s own missing-callback-file test exercises,
    // where `args.callback.parent()` never existed on disk in the first
    // place.
    #[test]
    fn record_failure_never_creates_the_profile_directory() {
        let dir = temp_dir("absent-dir");
        assert!(!dir.exists());

        record_failure(&dir, "failure against a reclaimed profile dir", 9);

        assert!(
            !dir.exists(),
            "recording a failure must never resurrect a profile directory the GC reclaimed"
        );
    }

    #[test]
    #[cfg(unix)]
    fn record_failure_writes_the_record_at_mode_0600() {
        use std::os::unix::fs::PermissionsExt;

        let dir = temp_dir("mode");
        fs::create_dir_all(&dir).expect("create profile dir");
        record_failure(&dir, "failure", 3);

        let mode = fs::metadata(notify_failure_path(&dir))
            .expect("notify-failures.json metadata")
            .permissions()
            .mode()
            & 0o777;
        assert_eq!(mode, 0o600);

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn record_failure_leaves_no_temp_file_behind() {
        let dir = temp_dir("no-temp-leftover");
        fs::create_dir_all(&dir).expect("create profile dir");
        record_failure(&dir, "failure", 3);

        let leftovers: Vec<_> = fs::read_dir(&dir)
            .expect("read profile dir")
            .filter_map(|entry| entry.ok())
            .map(|entry| entry.file_name().to_string_lossy().into_owned())
            .filter(|name| name != "notify-failures.json")
            .collect();
        assert!(leftovers.is_empty(), "unexpected leftovers: {leftovers:?}");

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn read_record_on_a_malformed_file_reports_no_record_rather_than_panicking() {
        let dir = temp_dir("malformed");
        fs::create_dir_all(&dir).expect("create profile dir");
        fs::write(notify_failure_path(&dir), "not json").expect("write malformed record");

        assert!(read_record(&dir).is_none());

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn the_record_serializes_with_camel_case_field_names() {
        let dir = temp_dir("camel-case");
        fs::create_dir_all(&dir).expect("create profile dir");
        record_failure(&dir, "boom", 42);

        let raw = fs::read_to_string(notify_failure_path(&dir)).expect("read record");
        assert!(raw.contains("\"count\""), "raw: {raw}");
        assert!(raw.contains("\"lastFailureAtMs\""), "raw: {raw}");
        assert!(raw.contains("\"lastError\""), "raw: {raw}");

        let _ = fs::remove_dir_all(&dir);
    }
}
