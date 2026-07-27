// GC sweep over `agent-profiles/` (260725 Phase 4): reclaims per-terminal
// directories left behind by a closed terminal - `agent-profiles/<id>/` is
// created at spawn time (`terminal.rs::TerminalSession::spawn`) and, before
// this phase, was never cleaned up (see that call site's own forward-note
// CONTRACT pinning cleanup to this phase).
//
// CONTRACT (ORDERING IS LOAD-BEARING - ticket "GC sweep... must run strictly
// AFTER `boot_reconcile` completes"): this module's `sweep_agent_profiles`
// keys candidate selection off the CALLER-SUPPLIED live-id set, which the
// wiring in `server.rs` sources from `TerminalRegistry::live_terminal_ids()`
// AFTER `TerminalRegistry::boot_reconcile` has fully populated the registry.
// Running this sweep before `boot_reconcile` completes (or against an empty/
// freshly-constructed registry) would see zero live ids and delete the
// profile directory of every helper about to be adopted a moment later -
// this module has no way to defend against that itself; the ordering
// guarantee lives entirely in the `server.rs` call site and is proven by
// `crates/daemon/tests/terminal_notify_callback_restart.rs`'s ordering
// regression test, not by anything in this file.
//
// "Keys off TERMINAL liveness" (not `TerminalSession::is_live()`'s strict
// Running-only check) means the live-id set must include every terminal id
// currently in `TerminalRegistry`'s in-memory session map regardless of
// status - a config may legitimately outlive an agent that exited inside a
// surviving terminal (grace window, or simply a shell that outlived its
// agent child).
//
// CONTRACT (260726 Phase 1 - the second pass is deliberately independent):
// this sweep also READS each LIVE terminal's `notify-failures.json` so the
// daemon can warn once about a `terminal-notify` hook whose delivery has
// stayed broken (the hook process itself is permanently silent on stdio by
// design - see `terminal_notify.rs`'s module CONTRACT). That read applies
// ONLY to the directories this sweep already SKIPS, and its results never
// feed back into which directories get reclaimed. The orphan-reclaim loop
// below and its iteration order are unchanged by that addition, because a
// different regression test depends on them
// (`terminal_notify_callback_restart.rs`'s adopt-before-sweep ordering
// test).

use std::collections::HashSet;
use std::path::Path;
use std::time::Duration;

use crate::notify_failure::{NotifyFailureRecord, NotifyFailureWatch};
use crate::terminal::TerminalRegistry;

/// One live terminal directory as observed by a single blocking sweep pass:
/// its id, its failure record (if any), and its `callback.json` mtime.
///
/// CONTRACT: OWNED values, not borrows. The blocking sweep body runs inside
/// `spawn_blocking` and cannot lend anything back across that boundary, so
/// it returns what the async side needs rather than holding a reference to
/// it.
type LiveProfileObservation = (String, Option<NotifyFailureRecord>, Option<u64>);

/// Pure candidate-selection, unit-testable without touching a filesystem:
/// given the directory names currently under `agent-profiles/` and the set
/// of terminal ids the registry considers live, returns the subset that
/// should be reclaimed. Any directory name failing a basic sanity check
/// (empty, `.`/`..`, or containing a path separator) is skipped - logged by
/// the caller, never deleted - as a defensive guard against a malformed name
/// ever reaching a real `remove_dir_all` call.
pub fn orphaned_profile_ids(
    profile_root_entries: impl Iterator<Item = String>,
    live_ids: &HashSet<String>,
) -> Vec<String> {
    profile_root_entries
        .filter(|name| is_sane_directory_name(name))
        .filter(|name| !live_ids.contains(name))
        .collect()
}

fn is_sane_directory_name(name: &str) -> bool {
    !name.is_empty() && name != "." && name != ".." && !name.contains('/') && !name.contains('\\')
}

/// Async driver: reads `<state_dir>/agent-profiles/`, computes orphans via
/// `orphaned_profile_ids` against `registry.live_terminal_ids()`, and removes
/// each orphan's directory plus its matching token-store file. The actual
/// blocking I/O runs inside `spawn_blocking` - the loop body is a single
/// bounded `fs::read_dir` pass (implicitly capped by `MAX_TERMINAL_SESSIONS`
/// live sessions plus whatever accumulated since the last sweep), no
/// recursion, no unbounded per-tick work.
///
/// `watch` carries the warn-once bookkeeping for the delivery-failure
/// escalation across ticks; it is an explicit `&mut` parameter (never a
/// module static) so the sweep task in `server.rs` owns it and dies with it.
/// `grace` is the escalation's policy window, supplied by that same call
/// site: it is the caller's sweep period, and keeping it a parameter is what
/// stops this module from having to reach back up into `server.rs` for a
/// constant.
pub async fn sweep_agent_profiles(
    state_dir: &Path,
    registry: &TerminalRegistry,
    watch: &mut NotifyFailureWatch,
    grace: Duration,
) {
    let live_ids = registry.live_terminal_ids();
    let live_ids_for_blocking = live_ids.clone();
    let state_dir = state_dir.to_path_buf();
    let observations = match tokio::task::spawn_blocking(move || {
        sweep_agent_profiles_blocking(&state_dir, &live_ids_for_blocking)
    })
    .await
    {
        Ok(observations) => observations,
        Err(error) => {
            tracing::warn!(%error, "agent-profiles GC sweep task panicked");
            Vec::new()
        }
    };

    let now = now_ms();
    let grace_ms = grace.as_millis() as u64;
    for (terminal_id, record, callback_mtime_ms) in &observations {
        if !watch.should_warn(terminal_id, record.as_ref(), *callback_mtime_ms, now, grace_ms) {
            continue;
        }
        // `should_warn` only returns true on the `count > 0` path, so the
        // record is present here by construction.
        let Some(record) = record.as_ref() else { continue };
        tracing::warn!(
            terminal_id = %terminal_id,
            count = record.count,
            last_error = %record.last_error,
            "terminal-notify delivery has been failing for this terminal and has not self-healed"
        );
    }
    watch.retain_live(&live_ids);
}

fn now_ms() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|duration| duration.as_millis() as u64)
        .unwrap_or(0)
}

fn sweep_agent_profiles_blocking(
    state_dir: &Path,
    live_ids: &HashSet<String>,
) -> Vec<LiveProfileObservation> {
    let profile_root = state_dir.join("agent-profiles");
    let read_dir = match std::fs::read_dir(&profile_root) {
        Ok(read_dir) => read_dir,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Vec::new(),
        Err(error) => {
            tracing::warn!(
                %error,
                path = %profile_root.display(),
                "agent-profiles GC sweep: directory unreadable"
            );
            return Vec::new();
        }
    };

    // Materialized rather than left lazy: the orphan-reclaim loop below and
    // the live-directory observation pass after it each walk this list once.
    let names: Vec<String> = read_dir
        .filter_map(|entry| {
            let entry = entry.ok()?;
            let is_dir = entry.file_type().ok()?.is_dir();
            is_dir.then(|| entry.file_name().to_string_lossy().into_owned())
        })
        .collect();

    for orphaned in orphaned_profile_ids(names.iter().cloned(), live_ids) {
        let dir = profile_root.join(&orphaned);
        match std::fs::remove_dir_all(&dir) {
            Ok(()) => {
                crate::agent_token_store::delete_token(state_dir, &orphaned);
                tracing::info!(
                    terminal_id = %orphaned,
                    "agent-profiles GC sweep: reclaimed orphaned profile directory"
                );
            }
            Err(error) => {
                tracing::warn!(
                    %error,
                    path = %dir.display(),
                    "agent-profiles GC sweep: failed to remove orphaned profile directory"
                );
            }
        }
    }

    // Second, INDEPENDENT pass (260726 Phase 1): observe only the live
    // directories the reclaim loop above deliberately skipped. Nothing here
    // deletes, and nothing here can influence what the loop above already
    // decided - see this module's CONTRACT.
    names
        .into_iter()
        .filter(|name| is_sane_directory_name(name) && live_ids.contains(name))
        .map(|terminal_id| {
            let dir = profile_root.join(&terminal_id);
            let record = crate::notify_failure::read_record(&dir);
            let callback_mtime_ms = std::fs::metadata(crate::agent_callback::callback_path(&dir))
                .and_then(|metadata| metadata.modified())
                .ok()
                .and_then(|modified| modified.duration_since(std::time::UNIX_EPOCH).ok())
                .map(|since_epoch| since_epoch.as_millis() as u64);
            (terminal_id, record, callback_mtime_ms)
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicU64, Ordering};
    use std::time::Duration;

    // The window the production call sites pass (`server.rs`), referenced
    // rather than re-typed as a literal so these tests cannot keep exercising
    // an old window after the constant moves.
    use crate::server::AGENT_PROFILE_GC_SWEEP_PERIOD;

    static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

    fn temp_dir(label: &str) -> std::path::PathBuf {
        let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
        std::env::temp_dir().join(format!(
            "ws-dashboard-agent-profile-gc-{label}-{}-{unique}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time after epoch")
                .as_nanos()
        ))
    }

    #[test]
    fn orphaned_profile_ids_keeps_live_and_reclaims_the_rest() {
        let live: HashSet<String> = ["term_live".to_owned()].into_iter().collect();
        let entries = vec![
            "term_live".to_owned(),
            "term_orphan_a".to_owned(),
            "term_orphan_b".to_owned(),
        ];

        let mut orphans = orphaned_profile_ids(entries.into_iter(), &live);
        orphans.sort();
        assert_eq!(orphans, vec!["term_orphan_a".to_owned(), "term_orphan_b".to_owned()]);
    }

    #[test]
    fn orphaned_profile_ids_skips_malformed_names_without_reclaiming_them() {
        let live: HashSet<String> = HashSet::new();
        let entries = vec![
            "".to_owned(),
            ".".to_owned(),
            "..".to_owned(),
            "../escape".to_owned(),
            "nested/path".to_owned(),
            "term_real_orphan".to_owned(),
        ];

        let orphans = orphaned_profile_ids(entries.into_iter(), &live);
        assert_eq!(orphans, vec!["term_real_orphan".to_owned()]);
    }

    #[test]
    fn orphaned_profile_ids_on_an_empty_root_reclaims_nothing() {
        let live: HashSet<String> = HashSet::new();
        assert!(orphaned_profile_ids(std::iter::empty(), &live).is_empty());
    }

    #[tokio::test]
    async fn sweep_agent_profiles_removes_an_orphaned_directory_and_its_token() {
        let state_dir = temp_dir("orphan-reclaim");
        let profile_root = state_dir.join("agent-profiles");
        let orphan_dir = profile_root.join("term_orphan");
        std::fs::create_dir_all(&orphan_dir).expect("create orphan profile dir");
        std::fs::write(orphan_dir.join("settings.json"), "{}").expect("write orphan settings.json");
        crate::agent_token_store::write_token(&state_dir, "term_orphan", "secret")
            .expect("write orphan token");

        let registry = TerminalRegistry::new(
            std::path::PathBuf::from("/nonexistent-unused-helper-binary"),
            temp_dir("orphan-reclaim-registry"),
            Duration::from_millis(200),
            Some(state_dir.clone()),
            "http://127.0.0.1:0".to_owned(),
        );

        sweep_agent_profiles(
            &state_dir,
            &registry,
            &mut NotifyFailureWatch::default(),
            AGENT_PROFILE_GC_SWEEP_PERIOD,
        )
        .await;

        assert!(!orphan_dir.exists(), "orphaned profile directory must be reclaimed");
        assert!(
            crate::agent_token_store::read_token(&state_dir, "term_orphan").is_none(),
            "the orphan's token file must be deleted alongside its profile directory"
        );

        let _ = std::fs::remove_dir_all(&state_dir);
    }

    #[tokio::test]
    async fn sweep_agent_profiles_never_touches_a_directory_belonging_to_a_live_session() {
        let state_dir = temp_dir("preserve-live");
        let profile_root = state_dir.join("agent-profiles");
        let live_dir = profile_root.join("term_live");
        std::fs::create_dir_all(&live_dir).expect("create live profile dir");
        std::fs::write(live_dir.join("settings.json"), "{}").expect("write live settings.json");
        crate::agent_token_store::write_token(&state_dir, "term_live", "secret")
            .expect("write live token");

        let registry = TerminalRegistry::new(
            std::path::PathBuf::from("/nonexistent-unused-helper-binary"),
            temp_dir("preserve-live-registry"),
            Duration::from_millis(200),
            Some(state_dir.clone()),
            "http://127.0.0.1:0".to_owned(),
        );
        crate::terminal::insert_fake_live_session_for_test(&registry, "term_live").await;

        sweep_agent_profiles(
            &state_dir,
            &registry,
            &mut NotifyFailureWatch::default(),
            AGENT_PROFILE_GC_SWEEP_PERIOD,
        )
        .await;

        assert!(
            live_dir.exists(),
            "a directory whose terminal id is still live must never be reclaimed"
        );
        assert!(
            crate::agent_token_store::read_token(&state_dir, "term_live").is_some(),
            "a live session's token file must survive the sweep"
        );

        let _ = std::fs::remove_dir_all(&state_dir);
    }

    // CONTRACT (260725 Phase 4 review cycle 1, finding A - the mandatory
    // regression test for the CONCURRENT-SPAWN case): reproduces the exact
    // gap the Critical finding named - `agent-profiles/<id>/` created on
    // disk (and its token written) BEFORE the session is ever inserted into
    // `sessions` (which is what `TerminalSession::spawn` does in
    // production: directory + token first, real process spawn and IPC
    // handshake after, `insert`/`insert_unchecked` only at the very end).
    // Without `TerminalRegistry::mark_profile_pending` (called at exactly
    // that "about to create the directory" point), `live_terminal_ids()`
    // would return empty here - `insert_unchecked` is deliberately never
    // called in this test, unlike
    // `sweep_agent_profiles_never_touches_a_directory_belonging_to_a_live_session`
    // above, which only exercises the ALREADY-live case. See the mutation
    // evidence in the Phase 4 review-cycle-1 fix report for proof this test
    // actually fails without the fix.
    #[tokio::test]
    async fn sweep_agent_profiles_never_touches_a_directory_whose_terminal_is_pending_but_not_yet_live() {
        let state_dir = temp_dir("preserve-pending");
        let profile_root = state_dir.join("agent-profiles");
        let pending_dir = profile_root.join("term_pending");
        std::fs::create_dir_all(&pending_dir).expect("create pending profile dir");
        std::fs::write(pending_dir.join("settings.json"), "{}").expect("write pending settings.json");
        crate::agent_token_store::write_token(&state_dir, "term_pending", "secret")
            .expect("write pending token");

        let registry = TerminalRegistry::new(
            std::path::PathBuf::from("/nonexistent-unused-helper-binary"),
            temp_dir("preserve-pending-registry"),
            Duration::from_millis(200),
            Some(state_dir.clone()),
            "http://127.0.0.1:0".to_owned(),
        );
        // Mark pending WITHOUT ever inserting a session - this is the
        // window `TerminalSession::spawn` sits in between creating the
        // directory and the caller's eventual `insert`/`insert_unchecked`
        // call, which a real process spawn plus IPC handshake can stretch
        // to tens or hundreds of milliseconds.
        crate::terminal::mark_profile_pending_for_test(&registry, "term_pending");

        sweep_agent_profiles(
            &state_dir,
            &registry,
            &mut NotifyFailureWatch::default(),
            AGENT_PROFILE_GC_SWEEP_PERIOD,
        )
        .await;

        assert!(
            pending_dir.exists(),
            "a directory whose terminal id is pending (created but not yet session-inserted) \
             must never be reclaimed by a sweep landing in that window"
        );
        assert!(
            crate::agent_token_store::read_token(&state_dir, "term_pending").is_some(),
            "a pending terminal's token file must survive a sweep landing in that window"
        );

        let _ = std::fs::remove_dir_all(&state_dir);
    }

    #[tokio::test]
    async fn sweep_agent_profiles_on_a_missing_agent_profiles_dir_is_a_harmless_no_op() {
        let state_dir = temp_dir("missing-root");
        let registry = TerminalRegistry::new(
            std::path::PathBuf::from("/nonexistent-unused-helper-binary"),
            temp_dir("missing-root-registry"),
            Duration::from_millis(200),
            Some(state_dir.clone()),
            "http://127.0.0.1:0".to_owned(),
        );

        sweep_agent_profiles(
            &state_dir,
            &registry,
            &mut NotifyFailureWatch::default(),
            AGENT_PROFILE_GC_SWEEP_PERIOD,
        )
        .await;

        let _ = std::fs::remove_dir_all(&state_dir);
    }
}
