//! FS-watch-driven git epoch invalidation (Phase 4 of
//! `260726-refactor-ws-dashboard-git-fs-watch-invalidation`).
//!
//! Implemented in the ticket's own step order because the first steps are the
//! correctness core and are fully testable without any I/O:
//!
//! 1. [`classify`] - pure path -> epoch-kind classification.
//! 2. [`IgnoreSet::derive`] - one `git status --ignored=matching` spawn per arm.
//! 3. `plan_watch_set` - the Linux gitignore-aware walk (added in a later
//!    checkpoint).
//! 4. Arming (platform-split registration).
//! 5. The event pipeline (debounced epoch bumps).
//! 6. Linux incremental registration.
//! 7. `reconcile` - the one hook everything routes through.
//! 8. Wiring the real `EpochSource` into `GitStateCache`.
//!
//! `cfg`-gated surface stays minimal by design (Lead Disposition D5): only the
//! registration backend, the Linux inotify budget read, and mount-type
//! resolution may carry a `#[cfg(...)]`. `classify`, `IgnoreSet`,
//! `RepoEpochs`, debounce/coalescing, and `reconcile`'s decision table stay
//! `cfg`-free and unit-testable on Linux/WSL.

use std::collections::{HashMap, HashSet};
use std::env;
use std::ffi::OsStr;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex, OnceLock};
use std::time::Instant;

use notify::{Event, RecommendedWatcher, RecursiveMode, Watcher};

use crate::discovery::WatchKey;
use crate::git_exec::{capture, git_timeout_from_env, GitFailureExpectation, GitSpawnStats};
use crate::git_state_cache::EpochSource;

/// The three filesystem paths `reconcile` (a later checkpoint) hands to the
/// watcher for one repo, populated straight from the widened
/// `DiscoveredWorkRoot`. `git_dir == common_dir` for a primary root; a linked
/// worktree's `git_dir` sits under `common_dir/worktrees/<name>`.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct WatchTargets {
    pub worktree: PathBuf,
    pub git_dir: PathBuf,
    pub common_dir: PathBuf,
}

/// Per-repo watcher health, reported by `GET /api/dashboard/diag/git` (a
/// later checkpoint) and consulted by `git_toolbar.rs`'s TTL selection.
/// `Degraded` carries a reason so the diag route can distinguish over-cap
/// from foreign-filesystem from arm-error, rather than reporting an
/// undifferentiated "not working" for a watcher that structurally cannot
/// fire (ticket step 9).
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum WatchHealth {
    Armed,
    Degraded(String),
    Unarmed,
}

impl WatchHealth {
    pub fn label(&self) -> &'static str {
        match self {
            WatchHealth::Armed => "armed",
            WatchHealth::Degraded(_) => "degraded",
            WatchHealth::Unarmed => "unarmed",
        }
    }

    pub fn reason(&self) -> Option<&str> {
        match self {
            WatchHealth::Degraded(reason) => Some(reason.as_str()),
            _ => None,
        }
    }
}

/// One `GET /api/dashboard/diag/git` `repos[]` row (ticket step 9). `key` is
/// not one of the ticket's literal five field names, but is added anyway -
/// a diagnostics endpoint reporting `{ health, worktreeEpoch, ... }` for an
/// unnamed set of "repos" plural, with no way to tell entries apart, would
/// not actually be usable for the "distinguish over-cap from
/// foreign-filesystem from arm-error" job the route exists to do.
#[derive(Clone, Debug)]
pub struct WatchDiagEntry {
    pub key: String,
    pub health: WatchHealth,
    pub worktree_epoch: u64,
    pub refs_epoch: u64,
    pub last_event_at_ms: Option<u64>,
    pub registered_watches: usize,
}

/// Which cache axis an observed filesystem event invalidates. Mirrors
/// `git_state_cache`'s two independently revalidated slots.
#[derive(Clone, Copy, Debug, Eq, Hash, PartialEq)]
pub(crate) enum EpochKind {
    Worktree,
    Refs,
}

/// Step 5's leading+trailing coalescing, at most two bumps per kind per
/// window (ticket Phase 4 step 5). Takes every timestamp as an explicit
/// caller-supplied `now_ms` parameter (an injected clock, per the ticket's
/// Verification boundary) rather than reading a wall clock itself, so the
/// unit tests below run in zero wall-clock time and the production event
/// loop can drive it from `tokio::time::Instant` millis without this type
/// depending on tokio.
///
/// - **Leading:** the first event of a window bumps immediately
///   ([`DebounceEvent::BumpNow`]) - a single save is visible to a poll
///   landing moments later.
/// - **Trailing:** if any further event arrived after the leading bump,
///   [`Debouncer::poll_close`] reports one more bump when the window closes.
///   Both halves are required: trailing-only delays the bump past a poll
///   that lands mid-window; leading-only is a correctness hole (later writes
///   in the window are silently suppressed and the slot reads valid for a
///   full TTL despite being stale).
/// - The window is capped at `window_ms * 5` measured from the FIRST event,
///   so a continuous stream of writes still closes and reopens periodically
///   instead of suppressing every bump forever.
pub(crate) struct Debouncer {
    window_ms: u64,
    cap_ms: u64,
    open: Option<DebounceWindow>,
}

struct DebounceWindow {
    opened_at_ms: u64,
    last_event_at_ms: u64,
    event_count: u32,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum DebounceEvent {
    /// Leading edge of a new window: bump the epoch now.
    BumpNow,
    /// Inside an already-open window: no bump yet: `poll_close` will decide
    /// whether a trailing bump is due once the window closes.
    Deferred,
}

impl Debouncer {
    pub(crate) fn new(window_ms: u64) -> Self {
        Self {
            window_ms,
            cap_ms: window_ms.saturating_mul(5),
            open: None,
        }
    }

    /// Record an event arriving at `now_ms`.
    pub(crate) fn record_event(&mut self, now_ms: u64) -> DebounceEvent {
        match &mut self.open {
            None => {
                self.open = Some(DebounceWindow {
                    opened_at_ms: now_ms,
                    last_event_at_ms: now_ms,
                    event_count: 1,
                });
                DebounceEvent::BumpNow
            }
            Some(window) => {
                window.last_event_at_ms = now_ms;
                window.event_count += 1;
                DebounceEvent::Deferred
            }
        }
    }

    /// Caller-driven poll (the production event loop calls this on a short
    /// periodic tick for every repo/kind with an open window): at `now_ms`,
    /// should the open window close? Returns `true` exactly once per window,
    /// and only when a second-or-later event arrived after the leading bump
    /// - a window whose only event was the leading one closes silently.
    pub(crate) fn poll_close(&mut self, now_ms: u64) -> bool {
        let should_close = match &self.open {
            None => false,
            Some(window) => {
                now_ms.saturating_sub(window.last_event_at_ms) >= self.window_ms
                    || now_ms.saturating_sub(window.opened_at_ms) >= self.cap_ms
            }
        };
        if !should_close {
            return false;
        }
        self.open
            .take()
            .map(|window| window.event_count > 1)
            .unwrap_or(false)
    }

    #[cfg(test)]
    pub(crate) fn is_open(&self) -> bool {
        self.open.is_some()
    }
}

/// The gitignore-derived ignore set for one repo's worktree (step 2,
/// `IgnoreSet::derive`).
///
/// Entries are path prefixes, and they are not all directories: `git status
/// -unormal --ignored=matching` reports collapsed directory prefixes
/// (trailing `/`) alongside individually-ignored files (no trailing `/`).
/// Treating the set as directories-only leaves an ignored file inside a
/// tracked directory neither pruned nor filtered.
#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub(crate) struct IgnoreSet {
    dir_prefixes: Vec<PathBuf>,
    files: Vec<PathBuf>,
}

impl IgnoreSet {
    /// The empty set: matches nothing, i.e. "watch everything" - the correct,
    /// just-noisier fallback when `derive` fails to run at all.
    pub(crate) fn empty() -> Self {
        Self::default()
    }

    #[cfg(test)]
    pub(crate) fn from_parts(
        dir_prefixes: impl IntoIterator<Item = PathBuf>,
        files: impl IntoIterator<Item = PathBuf>,
    ) -> Self {
        Self {
            dir_prefixes: dir_prefixes.into_iter().collect(),
            files: files.into_iter().collect(),
        }
    }

    pub(crate) fn matches(&self, path: &Path) -> bool {
        self.files.iter().any(|file| file == path)
            || self
                .dir_prefixes
                .iter()
                .any(|prefix| path.starts_with(prefix))
    }

    /// Step 2: one `git status` spawn per arm (or per 30s re-derive
    /// interval - see the event pipeline in a later checkpoint), collecting
    /// the `!!` (ignored) entries. On any failure (spawn error, non-zero
    /// exit, truncated/non-UTF-8 output) returns the empty set: "watch
    /// everything" is the correct, just-noisier fallback (ticket Decisions).
    ///
    /// **Must use `-unormal`, never `-uno`** - verified in the ticket's
    /// Decisions section: `-uno` silently suppresses the `!!` output
    /// entirely, which is the one failure mode that looks like success.
    /// [`IGNORE_DERIVE_ARGS`] is pinned by a dedicated unit test for exactly
    /// this reason.
    pub(crate) fn derive(worktree: &Path, stats: &GitSpawnStats) -> Self {
        let Ok(outcome) = capture(
            stats,
            worktree,
            IGNORE_DERIVE_ARGS,
            GitFailureExpectation::Unexpected,
            git_timeout_from_env(),
        ) else {
            return Self::empty();
        };
        let Some(stdout) = outcome.stdout_strict() else {
            return Self::empty();
        };
        Self::parse_status_z_output(stdout, worktree)
    }

    /// Parsing half of `derive`, split out so it is testable against a fixed
    /// byte string with no `git` spawn (ticket Verification boundary: "a
    /// fixed `-z` byte string").
    ///
    /// Keeps the trailing-`/` distinction from the raw `-z` output: a
    /// directory entry (`!! dir/`) becomes a path-prefix match, a file entry
    /// (`!! file`) becomes an exact match - the set is not directories-only
    /// (see [`IgnoreSet`]'s doc comment). `Path`'s `starts_with` compares by
    /// component, not by string, so the stored prefix's own trailing
    /// separator (or lack of one) does not affect matching.
    fn parse_status_z_output(stdout: &str, worktree: &Path) -> Self {
        let mut dir_prefixes = Vec::new();
        let mut files = Vec::new();
        for record in stdout.split('\0') {
            let Some(rel) = record.strip_prefix("!! ") else {
                continue;
            };
            if rel.is_empty() {
                continue;
            }
            let abs = worktree.join(rel);
            if rel.ends_with('/') {
                dir_prefixes.push(abs);
            } else {
                files.push(abs);
            }
        }
        Self { dir_prefixes, files }
    }
}

/// `git status --porcelain=v1 -unormal --ignored=matching -z` - see
/// [`IgnoreSet::derive`]'s doc comment for why `-unormal` (not `-uno`) is
/// load-bearing. `pub(crate)` so a unit test can assert on it directly
/// without spawning `git`.
const IGNORE_DERIVE_ARGS: &[&str] = &[
    "status",
    "--porcelain=v1",
    "-unormal",
    "--ignored=matching",
    "-z",
];

/// Step 3 bail-out: the walk crossed the per-repo directory cap
/// (`WS_DASHBOARD_GIT_WATCH_MAX_DIRS`) before finishing. `found` is the
/// directory count at the moment the cap was crossed, not a full-walk total -
/// the walk stops immediately rather than continuing to enumerate a
/// pathological tree.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) struct TooLarge {
    pub(crate) found: usize,
}

/// Step 3, Linux-only production code but a pure function over a path and an
/// ignore set, so it is exercised on every platform's `cargo test` (see the
/// ticket's Decisions: "plan_watch_set is Linux-only but needs no `cfg` on
/// its logic").
///
/// Descends from `worktree`, pruning any directory matching `ignore` and
/// pruning `git_dir`/`common_dir` from *registration* (not merely from
/// `classify` - see the ticket's Constraints: nothing else prunes the 256-way
/// `objects/` fanout). Then appends the git-internal targets explicitly:
/// `common_dir` top level, `common_dir/info/` (required for `classify`'s
/// `info/exclude` rule to be reachable at all - see [`classify`]), and a full
/// recursive directory walk of `common_dir/refs` and `common_dir/worktrees`
/// (branch names can nest arbitrarily deep as real subdirectories, and each
/// linked worktree's own `HEAD`/`ORIG_HEAD` live directly inside
/// `worktrees/<name>/`, one level under `common_dir/worktrees`).
///
/// Counts directories as it walks and bails with [`TooLarge`] the instant
/// `cap` is crossed, so a pathological monorepo costs a partial walk.
pub(crate) fn plan_watch_set(
    worktree: &Path,
    git_dir: &Path,
    common_dir: &Path,
    ignore: &IgnoreSet,
    cap: usize,
) -> Result<Vec<PathBuf>, TooLarge> {
    let mut targets = Vec::new();
    let mut count = 0usize;

    walk_worktree_dirs(worktree, git_dir, common_dir, ignore, cap, &mut targets, &mut count)?;

    push_target(common_dir.to_path_buf(), cap, &mut targets, &mut count)?;
    let info_dir = common_dir.join("info");
    if info_dir.is_dir() {
        push_target(info_dir, cap, &mut targets, &mut count)?;
    }
    walk_git_internal_subtree(&common_dir.join("refs"), cap, &mut targets, &mut count)?;
    walk_git_internal_subtree(&common_dir.join("worktrees"), cap, &mut targets, &mut count)?;

    Ok(targets)
}

fn push_target(
    path: PathBuf,
    cap: usize,
    targets: &mut Vec<PathBuf>,
    count: &mut usize,
) -> Result<(), TooLarge> {
    *count += 1;
    if *count > cap {
        return Err(TooLarge { found: *count });
    }
    targets.push(path);
    Ok(())
}

fn sorted_subdirs(dir: &Path) -> Vec<PathBuf> {
    let Ok(entries) = fs::read_dir(dir) else {
        return Vec::new();
    };
    let mut subdirs: Vec<PathBuf> = entries
        .filter_map(|entry| entry.ok())
        .filter(|entry| entry.file_type().map(|kind| kind.is_dir()).unwrap_or(false))
        .map(|entry| entry.path())
        .collect();
    // Deterministic order (ticket Constraints: "arm in a deterministic
    // order"), so two runs over the same tree register the same list.
    subdirs.sort();
    subdirs
}

fn walk_worktree_dirs(
    dir: &Path,
    git_dir: &Path,
    common_dir: &Path,
    ignore: &IgnoreSet,
    cap: usize,
    targets: &mut Vec<PathBuf>,
    count: &mut usize,
) -> Result<(), TooLarge> {
    if dir == git_dir || dir == common_dir || ignore.matches(dir) {
        return Ok(());
    }
    push_target(dir.to_path_buf(), cap, targets, count)?;
    for subdir in sorted_subdirs(dir) {
        walk_worktree_dirs(&subdir, git_dir, common_dir, ignore, cap, targets, count)?;
    }
    Ok(())
}

/// Unlike `walk_worktree_dirs`, git-internal targets are never subject to the
/// ignore set - `git status --ignored` never reports paths under `.git`, so
/// there is nothing to prune here.
fn walk_git_internal_subtree(
    dir: &Path,
    cap: usize,
    targets: &mut Vec<PathBuf>,
    count: &mut usize,
) -> Result<(), TooLarge> {
    if !dir.is_dir() {
        return Ok(());
    }
    push_target(dir.to_path_buf(), cap, targets, count)?;
    for subdir in sorted_subdirs(dir) {
        walk_git_internal_subtree(&subdir, cap, targets, count)?;
    }
    Ok(())
}

/// The three filesystem targets `classify` (and, in a later checkpoint,
/// `plan_watch_set`/arming) need to decide what an observed path means for
/// one repo. Populated straight from the widened `DiscoveredWorkRoot`
/// (`git_dir`/`common_dir`) plus the derived `IgnoreSet`.
///
/// `git_dir == common_dir` for a primary root; a linked worktree's `git_dir`
/// sits under `common_dir/worktrees/<name>`.
#[derive(Clone, Debug)]
pub(crate) struct ArmedRepo {
    pub(crate) git_dir: PathBuf,
    pub(crate) common_dir: PathBuf,
    pub(crate) ignore: IgnoreSet,
}

/// Step 1: pure path -> epoch-kind classification. `None` means "ignore, no
/// epoch bump"; `Some(kind)` means "bump this epoch". Never spawns `git` and
/// never mutates `repo.ignore` - the ignore-rule-file staleness signal is a
/// separate, equally pure predicate ([`is_ignore_rule_file`]) so the event
/// pipeline (step 5) can react to it without `classify` itself carrying any
/// side-effecting state.
///
/// Order (load-bearing, see the ticket's Phase 4 step 1):
/// 1. Ignore-set match -> ignore. Checked first because it is the hot path:
///    a recursive Windows/macOS registration delivers every gitignored-tree
///    event, and `git status --ignored` never reports paths under `.git`, so
///    no git-internal path can spuriously match here.
/// 2. Explicit ignore-rule files (`.gitignore`, `common_dir/info/exclude`)
///    -> `Worktree`. Checked before the git-dir branch below, or
///    `info/exclude` (which lives under `common_dir`) is unreachable.
/// 3. Under `git_dir`/`common_dir`: `objects|lfs|modules` -> ignore; `*.lock`
///    -> ignore; `{HEAD,packed-refs,FETCH_HEAD,ORIG_HEAD}`/`refs/**`/
///    `worktrees/**` -> `Refs`; `index` -> `Worktree`; anything else -> ignore
///    (explicit fallthrough).
/// 4. Otherwise -> `Worktree`.
pub(crate) fn classify(path: &Path, repo: &ArmedRepo) -> Option<EpochKind> {
    if repo.ignore.matches(path) {
        return None;
    }
    if is_ignore_rule_file(path, repo) {
        return Some(EpochKind::Worktree);
    }
    if let Ok(rel) = path.strip_prefix(&repo.git_dir) {
        return classify_under_git_dir(rel);
    }
    if let Ok(rel) = path.strip_prefix(&repo.common_dir) {
        return classify_under_git_dir(rel);
    }
    Some(EpochKind::Worktree)
}

/// Whether `path` is one of the explicit ignore-rule files whose change must
/// mark the repo's [`IgnoreSet`] stale (step 1.2 / step 5's re-derivation
/// scheduling). Kept separate from `classify`'s return value so `classify`
/// can stay a pure `Option<EpochKind>` function per the ticket's signature
/// while the event pipeline still gets the staleness signal it needs.
pub(crate) fn is_ignore_rule_file(path: &Path, repo: &ArmedRepo) -> bool {
    if path.file_name() == Some(OsStr::new(".gitignore")) {
        return true;
    }
    path == repo.common_dir.join("info").join("exclude")
}

/// Step 1.3: classify a path already known to be under `git_dir` or
/// `common_dir`, given relative to whichever one matched.
fn classify_under_git_dir(rel: &Path) -> Option<EpochKind> {
    let mut components = rel.components();
    if let Some(first) = components.next() {
        let first = first.as_os_str();
        if first == OsStr::new("objects")
            || first == OsStr::new("lfs")
            || first == OsStr::new("modules")
        {
            return None;
        }
        if first == OsStr::new("refs") || first == OsStr::new("worktrees") {
            // `*.lock` still wins over `refs/**`/`worktrees/**` (e.g.
            // `refs/heads/main.lock` is noise, not a ref change), so the
            // lock check below must still run before returning `Refs` here.
            if is_lock_file(rel) {
                return None;
            }
            return Some(EpochKind::Refs);
        }
    }
    if is_lock_file(rel) {
        return None;
    }
    match rel.to_str() {
        Some("HEAD") | Some("packed-refs") | Some("FETCH_HEAD") | Some("ORIG_HEAD") => {
            Some(EpochKind::Refs)
        }
        Some("index") => Some(EpochKind::Worktree),
        _ => None,
    }
}

fn is_lock_file(rel: &Path) -> bool {
    rel.extension() == Some(OsStr::new("lock"))
}

// ---------------------------------------------------------------------------
// Pre-arm gate: filesystem allowlist (ticket Constraints, shared across
// platforms). A watcher armed on a foreign mount (WSL2 `/mnt/*`, NFS, CIFS,
// SSHFS/FUSE) succeeds and then silently never fires, which is worse than
// never arming - the diag route would claim `Armed` for a watcher that
// structurally cannot work. Resolution is the one piece of this rule that is
// genuinely platform-specific (D5); the allow/degrade decision itself is not.
// ---------------------------------------------------------------------------

/// Resolve `path`'s mount filesystem type and decide whether it is known to
/// deliver filesystem events. Allowlist, not blocklist, per the ticket
/// Constraints: an over-conservative miss just degrades one repo to polling,
/// while a missed blocklist entry silently mis-arms it.
#[cfg(target_os = "linux")]
pub(crate) fn mount_allows_watching(path: &Path) -> bool {
    const ALLOWED: &[&str] = &[
        "ext2", "ext3", "ext4", "btrfs", "xfs", "f2fs", "zfs", "overlay", "tmpfs",
    ];
    match linux_mount_fstype(path) {
        Some(fstype) => ALLOWED.contains(&fstype.as_str()),
        None => false,
    }
}

/// Parses `/proc/self/mountinfo`, returning the filesystem type of the
/// longest matching mount point for `path` (i.e. the mount that actually
/// owns it, not an ancestor mount it happens to sit under).
#[cfg(target_os = "linux")]
fn linux_mount_fstype(path: &Path) -> Option<String> {
    let contents = fs::read_to_string("/proc/self/mountinfo").ok()?;
    let resolved = path.canonicalize().unwrap_or_else(|_| path.to_path_buf());
    let mut best: Option<(usize, String)> = None;
    for line in contents.lines() {
        // Format: `<id> <parent> <major:minor> <root> <mount point> <options> \
        // <optional fields...> - <fstype> <source> <super options>`. The `-`
        // separator is the only reliable anchor before the fixed 3-field
        // tail.
        let Some((left, right)) = line.split_once(" - ") else {
            continue;
        };
        // `continue`, never `?` (review finding 10): one unparsable line
        // must skip only that line, not degrade every repo's mount-type
        // resolution by returning `None` for the whole function.
        let Some(mount_point) = left.split_whitespace().nth(4) else {
            continue;
        };
        let Some(fstype) = right.split_whitespace().next() else {
            continue;
        };
        if resolved.starts_with(mount_point) {
            let len = mount_point.len();
            if best.as_ref().map(|(best_len, _)| len > *best_len).unwrap_or(true) {
                best = Some((len, fstype.to_owned()));
            }
        }
    }
    best.map(|(_, fstype)| fstype)
}

#[cfg(target_os = "macos")]
pub(crate) fn mount_allows_watching(path: &Path) -> bool {
    const ALLOWED: &[&str] = &["apfs", "hfs"];
    match macos_mount_fstype(path) {
        Some(fstype) => ALLOWED.contains(&fstype.as_str()),
        None => false,
    }
}

#[cfg(target_os = "macos")]
fn macos_mount_fstype(path: &Path) -> Option<String> {
    use std::ffi::CString;
    use std::mem::MaybeUninit;
    use std::os::unix::ffi::OsStrExt;

    let resolved = path.canonicalize().unwrap_or_else(|_| path.to_path_buf());
    let c_path = CString::new(resolved.as_os_str().as_bytes()).ok()?;
    // SAFETY: `buf` is a valid, correctly-sized, zero-initialized `statfs`
    // for this platform; `statfs` fully populates it on success (return 0)
    // and we only read `f_fstypename` afterward.
    unsafe {
        let mut buf: MaybeUninit<libc::statfs> = MaybeUninit::zeroed();
        if libc::statfs(c_path.as_ptr(), buf.as_mut_ptr()) != 0 {
            return None;
        }
        let buf = buf.assume_init();
        let name = std::ffi::CStr::from_ptr(buf.f_fstypename.as_ptr());
        Some(name.to_string_lossy().into_owned())
    }
}

#[cfg(windows)]
pub(crate) fn mount_allows_watching(path: &Path) -> bool {
    use std::path::Component;
    // `GetDriveTypeW` lives under `Storage::FileSystem`, but its `DRIVE_*`
    // return-value constants live under `System::WindowsProgramming` in
    // windows-sys - two different modules, both needed here.
    use windows_sys::Win32::Storage::FileSystem::GetDriveTypeW;
    use windows_sys::Win32::System::WindowsProgramming::{DRIVE_FIXED, DRIVE_RAMDISK};

    let resolved = path.canonicalize().unwrap_or_else(|_| path.to_path_buf());
    // Reject any UNC path outright: `\\server\share\...` has no drive-letter
    // root for GetDriveType to answer about, and a mapped network drive vs. a
    // raw UNC path are the identical silent-and-armed hazard.
    let Some(Component::Prefix(prefix)) = resolved.components().next() else {
        return false;
    };
    let drive_root = match prefix.kind() {
        std::path::Prefix::Disk(letter) | std::path::Prefix::VerbatimDisk(letter) => {
            format!("{}:\\", letter as char)
        }
        _ => return false,
    };
    let mut wide: Vec<u16> = drive_root.encode_utf16().collect();
    wide.push(0);
    // SAFETY: `wide` is a valid null-terminated UTF-16 string.
    let drive_type = unsafe { GetDriveTypeW(wide.as_ptr()) };
    matches!(drive_type, DRIVE_FIXED | DRIVE_RAMDISK)
}

#[cfg(not(any(target_os = "linux", target_os = "macos", windows)))]
pub(crate) fn mount_allows_watching(_path: &Path) -> bool {
    false
}

// ---------------------------------------------------------------------------
// Linux-only: process-wide inotify budget (ticket Constraints - "read both
// inotify limits, not just the watch count").
// ---------------------------------------------------------------------------

#[cfg(target_os = "linux")]
pub(crate) fn linux_inotify_process_budget() -> usize {
    let max_user_watches = linux_read_proc_sys_u64("/proc/sys/fs/inotify/max_user_watches")
        .unwrap_or(8_192);
    let max_user_instances = linux_read_proc_sys_u64("/proc/sys/fs/inotify/max_user_instances")
        .unwrap_or(128);
    if max_user_instances < 2 {
        tracing::warn!(
            max_user_instances,
            "host's inotify max_user_instances is unusually low; the shared \
             watcher still needs only one instance, but this is worth knowing \
             if that changes"
        );
    }
    ((max_user_watches as usize).saturating_mul(60) / 100).min(8_192)
}

#[cfg(target_os = "linux")]
fn linux_read_proc_sys_u64(path: &str) -> Option<u64> {
    fs::read_to_string(path).ok()?.trim().parse().ok()
}

#[cfg(not(target_os = "linux"))]
fn linux_process_budget() -> usize {
    0
}

#[cfg(target_os = "linux")]
fn linux_process_budget() -> usize {
    linux_inotify_process_budget()
}

// ---------------------------------------------------------------------------
// Step 4 & 6: arming (platform-split registration) and the event pipeline.
// Everything below this point is orchestration around the pure functions
// above; the decision logic itself (`classify`, `IgnoreSet`, `Debouncer`) has
// already run by the time any of this touches a lock. No `#[cfg(...)]`
// appears past the two backend fns (`do_arm_linux`/`do_arm_recursive`) and
// `owned_registered_dirs`' platform split (D5) - registry bookkeeping,
// health computation, and the event loop are shared.
// ---------------------------------------------------------------------------

/// How aggressively the watcher arms. `Off` never touches `notify`; every
/// repo reports [`WatchHealth::Unarmed`] and callers fall back to the short
/// TTL. `Force` skips the [`mount_allows_watching`] pre-arm gate (diagnostic
/// escape hatch, ticket Constraints); `Auto` is the default.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum WatchMode {
    Off,
    Auto,
    Force,
}

/// Tunables threaded through from the `WS_DASHBOARD_GIT_WATCH*` env vars,
/// via [`WatchConfig::from_env`] (ticket step 9).
#[derive(Clone, Debug)]
pub struct WatchConfig {
    pub mode: WatchMode,
    pub debounce_ms: u64,
    pub max_dirs: usize,
    pub armed_ttl_ms: u64,
}

impl Default for WatchConfig {
    fn default() -> Self {
        Self {
            mode: WatchMode::Auto,
            debounce_ms: 100,
            max_dirs: 1024,
            armed_ttl_ms: 120_000,
        }
    }
}

static WATCH_CONFIG_FROM_ENV: OnceLock<WatchConfig> = OnceLock::new();

impl WatchConfig {
    /// Reads `WS_DASHBOARD_GIT_WATCH` (`off`|`auto`|`force`),
    /// `WS_DASHBOARD_GIT_WATCH_DEBOUNCE_MS`, `WS_DASHBOARD_GIT_WATCH_MAX_DIRS`,
    /// and `WS_DASHBOARD_GIT_ARMED_TTL_MS` once per process and caches the
    /// result in a `OnceLock` (ticket step 9), matching
    /// `git_state_cache::git_cache_ttl_from_env`'s cached-read-once idiom
    /// exactly. An absent or malformed value for any one field falls back to
    /// [`WatchConfig::default`]'s value for that field only - the same
    /// per-field fallback discipline every other env-tunable in this daemon
    /// uses, so one typo'd knob cannot silently reset the other three.
    pub fn from_env() -> Self {
        WATCH_CONFIG_FROM_ENV
            .get_or_init(|| {
                let default = WatchConfig::default();
                let mode = env::var("WS_DASHBOARD_GIT_WATCH")
                    .ok()
                    .and_then(|raw| parse_watch_mode(&raw))
                    .unwrap_or(default.mode);
                let debounce_ms = env::var("WS_DASHBOARD_GIT_WATCH_DEBOUNCE_MS")
                    .ok()
                    .and_then(|raw| raw.trim().parse::<u64>().ok())
                    .unwrap_or(default.debounce_ms);
                let max_dirs = env::var("WS_DASHBOARD_GIT_WATCH_MAX_DIRS")
                    .ok()
                    .and_then(|raw| raw.trim().parse::<usize>().ok())
                    .unwrap_or(default.max_dirs);
                let armed_ttl_ms = env::var("WS_DASHBOARD_GIT_ARMED_TTL_MS")
                    .ok()
                    .and_then(|raw| raw.trim().parse::<u64>().ok())
                    .unwrap_or(default.armed_ttl_ms);
                WatchConfig { mode, debounce_ms, max_dirs, armed_ttl_ms }
            })
            .clone()
    }
}

/// `WS_DASHBOARD_GIT_WATCH`'s three recognized values (case-insensitive,
/// trimmed), split out of [`WatchConfig::from_env`] as a pure function so it
/// is directly unit-testable - the surrounding `OnceLock`/`env::var` glue is
/// deliberately left untested, matching
/// `git_state_cache::git_cache_ttl_from_env`'s and
/// `git_exec::git_timeout_from_env`'s existing precedent (neither has a
/// dedicated test - process-wide env state and a process-wide cache make
/// that fragile under parallel `cargo test` execution).
fn parse_watch_mode(raw: &str) -> Option<WatchMode> {
    match raw.trim().to_ascii_lowercase().as_str() {
        "off" => Some(WatchMode::Off),
        "auto" => Some(WatchMode::Auto),
        "force" => Some(WatchMode::Force),
        _ => None,
    }
}

/// Monotonic millisecond clock shared by the debounce windows and the
/// rescan-degraded self-heal timer. Process-relative (not wall-clock) so a
/// system clock adjustment cannot stall or fast-forward a debounce window;
/// unlike [`Debouncer`] itself (which takes `now_ms` as an explicit
/// parameter for zero-wall-clock-time unit tests), this is the one place
/// production code actually samples time, and it stays out of the pure
/// functions above by design.
fn clock_ms() -> u64 {
    static START: OnceLock<Instant> = OnceLock::new();
    let start = *START.get_or_init(Instant::now);
    start.elapsed().as_millis() as u64
}

/// One repo's live bookkeeping: the registration state ([`WatchHealth`] plus,
/// when armed, the [`ArmedRepo`] classify needs and the exact directories
/// registered with the OS watcher), the debounce windows per axis, and the
/// rate-limit/staleness timers checkpoint 6 (`reconcile`) will read.
struct RepoRuntime {
    targets: WatchTargets,
    registration_health: WatchHealth,
    armed: Option<ArmedRepo>,
    registered_dirs: Vec<PathBuf>,
    /// Set by a `notify` "need_rescan" event (step 5): the OS watcher itself
    /// reports it may have dropped events (e.g. an inotify queue overflow).
    /// Registration stays intact - only the *reported* health degrades, for
    /// one TTL window, so `git_toolbar.rs`'s TTL selection falls back to the
    /// short poll interval until this clears.
    rescan_degraded_until_ms: Option<u64>,
    /// When the last arm attempt was *made* (not necessarily finished - see
    /// `WatchRegistry::reconcile`, which stamps this before offloading the
    /// actual work). The single timestamp both of `reconcile`'s rate-limit
    /// guards read: `Unarmed` checks it against the flat
    /// [`MIN_ARM_INTERVAL_MS`]; `Degraded` checks it against
    /// `degraded_backoff_ms`. Deliberately never touched by `do_disarm` - a
    /// disarm-driven transition back to `Unarmed` must not reset this, or
    /// the flat-interval guard could not do the one job it exists for
    /// (ticket step 7: bound a flapping-availability root's re-arm rate).
    last_arm_attempt_ms: Option<u64>,
    /// Current backoff for a `Degraded` repo's next eligible retry
    /// (`reconcile`'s exponential-backoff guard, ticket step 7: start 60 s,
    /// double on each consecutive `Degraded` outcome, cap 15 min). Reset to
    /// [`DEGRADED_BACKOFF_START_MS`] on every transition OUT of `Degraded`
    /// (a successful arm, or the first degrade after `Unarmed`/`Armed`) so
    /// the next time this repo degrades it starts a fresh cycle rather than
    /// inheriting a stale, possibly-maxed-out backoff from an unrelated
    /// earlier incident.
    degraded_backoff_ms: u64,
    last_event_at_ms: Option<u64>,
    ignore_stale: bool,
    /// When the last ignore-set re-derivation was *scheduled* (not
    /// necessarily finished yet) for this repo. Gates the ticket's "at most
    /// once per 30 s per repo" limit - checked before scheduling, not before
    /// running, so N `.gitignore` writes landing faster than one
    /// `IgnoreSet::derive` spawn still coalesce into one re-derivation
    /// instead of queuing several.
    last_ignore_rederive_attempt_ms: Option<u64>,
    debounce: HashMap<EpochKind, Debouncer>,
}

impl RepoRuntime {
    fn new(targets: WatchTargets) -> Self {
        Self {
            targets,
            registration_health: WatchHealth::Unarmed,
            armed: None,
            registered_dirs: Vec::new(),
            rescan_degraded_until_ms: None,
            last_arm_attempt_ms: None,
            degraded_backoff_ms: DEGRADED_BACKOFF_START_MS,
            last_event_at_ms: None,
            ignore_stale: false,
            last_ignore_rederive_attempt_ms: None,
            debounce: HashMap::new(),
        }
    }

    /// The health a caller should observe right now: the registration state,
    /// unless a rescan-degraded window is still open, in which case that
    /// takes precedence regardless of what the underlying registration says.
    fn effective_health(&self, now_ms: u64) -> WatchHealth {
        if let Some(until) = self.rescan_degraded_until_ms {
            if now_ms < until {
                return WatchHealth::Degraded("rescan required".to_owned());
            }
        }
        self.registration_health.clone()
    }
}

#[derive(Default)]
struct RegistryState {
    repos: HashMap<WatchKey, RepoRuntime>,
    /// Exact registered directory -> owning repos. A shared `common_dir`
    /// (primary root + its linked worktrees) is registered once; one write
    /// event under it fans out to every owner (ticket Constraints / Lead
    /// Disposition D2). Kept as its own map (not derived from `repos` on
    /// every event) because the event pipeline's hot path is "which repos
    /// own this path", not "what does this repo own".
    dir_index: HashMap<PathBuf, HashSet<WatchKey>>,
}

struct RegistryInner {
    epoch_source: Arc<dyn EpochSource>,
    git_stats: Arc<GitSpawnStats>,
    watcher: Mutex<Option<RecommendedWatcher>>,
    state: Mutex<RegistryState>,
    config: WatchConfig,
    linux_process_budget: usize,
}

/// The one shared handle everything routes through: one `notify` watcher
/// instance, one background event-loop task, and the per-repo bookkeeping
/// `reconcile` (a later checkpoint) drives via [`WatchRegistry::arm_now`]/
/// [`WatchRegistry::disarm_now`].
#[derive(Clone)]
pub struct WatchRegistry {
    inner: Arc<RegistryInner>,
}

impl WatchRegistry {
    /// Constructs the shared `notify` watcher and, if called from inside a
    /// Tokio runtime, spawns the background event-loop task. Outside a
    /// runtime (a plain `cargo test -p ws-dashboard-daemon --lib` unit test
    /// constructing a `WatchRegistry` directly, with no `#[tokio::test]`)
    /// the registry still builds and `arm_now`/`disarm_now` still run their
    /// synchronous halves inline - only the async event-consumption loop is
    /// skipped, which is exactly the piece those tests do not need.
    pub fn new(
        epoch_source: Arc<dyn EpochSource>,
        git_stats: Arc<GitSpawnStats>,
        config: WatchConfig,
    ) -> Self {
        let (event_tx, event_rx) = tokio::sync::mpsc::unbounded_channel::<notify::Result<Event>>();
        let watcher = notify::recommended_watcher(move |res| {
            // The `notify` callback fires on its own internal thread; this
            // send is the only work it does, so a slow consumer never stalls
            // the OS-level watch delivery.
            let _ = event_tx.send(res);
        })
        .map_err(|error| {
            tracing::warn!(%error, "failed to construct filesystem watcher; every repo will report Degraded and fall back to poll-driven TTL");
        })
        .ok();

        let inner = Arc::new(RegistryInner {
            epoch_source,
            git_stats,
            watcher: Mutex::new(watcher),
            state: Mutex::new(RegistryState::default()),
            config,
            linux_process_budget: linux_process_budget(),
        });

        if let Ok(handle) = tokio::runtime::Handle::try_current() {
            handle.spawn(run_event_loop(inner.clone(), event_rx));
        } else {
            tracing::debug!(
                "WatchRegistry constructed outside a Tokio runtime; the \
                 background event-consumption loop was not started (expected \
                 in a plain #[test], not in production)"
            );
        }

        Self { inner }
    }

    /// The health `reconcile` (later checkpoint) and `git_toolbar.rs`'s TTL
    /// selection (a later checkpoint) should observe for `key` right now.
    /// Unknown keys are `Unarmed` - the same fallback as a repo that has
    /// never been armed.
    /// The TTL `git_toolbar.rs`'s TTL selection uses for an `Armed` repo
    /// (ticket step 8's "120s armed / 2s degraded-or-unarmed" split). Reads
    /// through to `WatchConfig::armed_ttl_ms` so a later checkpoint's
    /// `WS_DASHBOARD_GIT_ARMED_TTL_MS` env override needs no change at this
    /// call site - only at `WatchConfig`'s construction in `server.rs`.
    pub fn armed_ttl_ms(&self) -> u64 {
        self.inner.config.armed_ttl_ms
    }

    pub fn health_for(&self, key: &WatchKey) -> WatchHealth {
        let now = clock_ms();
        let state = self.inner.state.lock().expect("watch registry state lock poisoned");
        state
            .repos
            .get(key)
            .map(|repo| repo.effective_health(now))
            .unwrap_or(WatchHealth::Unarmed)
    }

    /// One entry per repo this registry currently tracks (ticket step 9):
    /// `GET /api/dashboard/diag/git`'s `repos` array reads straight from
    /// this. `registered_watches` is the exact count the Linux cap
    /// (`WatchConfig::max_dirs`) is enforced against, and the number of
    /// recursive registrations (normally 1-3) on Windows/macOS - a large
    /// value there signals the wrong backend ran.
    pub fn diag_snapshot(&self) -> Vec<WatchDiagEntry> {
        let now = clock_ms();
        let state = self.inner.state.lock().expect("watch registry state lock poisoned");
        state
            .repos
            .iter()
            .map(|(key, repo)| {
                let (worktree_epoch, refs_epoch) = self.inner.epoch_source.epochs(key);
                WatchDiagEntry {
                    key: key.as_str().to_owned(),
                    health: repo.effective_health(now),
                    worktree_epoch,
                    refs_epoch,
                    last_event_at_ms: repo.last_event_at_ms,
                    registered_watches: repo.registered_dirs.len(),
                }
            })
            .collect()
    }

    /// Arm (or re-arm) `key` against `targets`. Runs the `git status` spawn
    /// and (Linux) the directory walk on whatever thread calls this, so
    /// callers driving this from an async context should do so via
    /// `spawn_blocking` (checkpoint 6's `reconcile` does).
    pub fn arm_now(&self, key: &WatchKey, targets: &WatchTargets) {
        do_arm(&self.inner, key, targets);
    }

    /// Disarm `key`: drop its OS-level registration (unregistering any
    /// directory no other repo still owns) and report [`WatchHealth::Unarmed`]
    /// from then on. Cheap - no `git` spawn, no filesystem walk - so, unlike
    /// `arm_now`, callers do not need to offload this to a blocking pool.
    pub fn disarm_now(&self, key: &WatchKey) {
        do_disarm(&self.inner, key);
    }

    /// Ticket step 7: the one hook everything routes through - register/
    /// unregister, every availability transition, `remove_workspace`, and
    /// `git_worktree_remove_submit` (a later checkpoint wires that last one
    /// in) instead of six separate call-site hooks.
    ///
    /// `entries` is this reconcile cycle's *complete* discovered-root set
    /// (owner candidates and linked worktrees alike), straight from
    /// `discovery::DashboardResourcesSync::watch_reconcile_entries`.
    ///
    /// Semantics (ticket step 7):
    /// - present + `Available` + `Unarmed`, rate-limit-eligible => arm.
    /// - present + not `Available` (or `targets` is `None` - not a git root)
    ///   => disarm + bump both, so the next poll recomputes and reports the
    ///   degraded state.
    /// - tracked by this registry but missing from `entries` entirely
    ///   (`absent`) => disarm + drop the epoch counters
    ///   ([`EpochSource::forget`]), rather than merely bumping - nothing will
    ///   ever probe this key again.
    ///
    /// **The two rate-limit guards this must not get wrong (D8):** `Unarmed`
    /// is gated by the flat [`MIN_ARM_INTERVAL_MS`] interval; `Degraded` is
    /// gated by its own sticky exponential backoff
    /// ([`RepoRuntime::degraded_backoff_ms`]) and is never treated as
    /// arm-eligible merely for being "not Armed" - see
    /// `RepoRuntime::last_arm_attempt_ms`'s doc comment for why both guards
    /// are independently required. **Arming never runs inline on this call:**
    /// an eligible arm is offloaded via `spawn_blocking` (falling back to
    /// inline only outside a Tokio runtime, e.g. a plain `#[test]`), so this
    /// method returns as soon as it has decided *what* to do, never after
    /// the `git status` spawn (and, on Linux, walk) an arm attempt costs.
    pub fn reconcile(
        &self,
        entries: &[(WatchKey, Option<WatchTargets>, ws_dashboard_core::WorkRootAvailability)],
    ) {
        let now = clock_ms();
        let present: HashSet<&WatchKey> = entries.iter().map(|(key, _, _)| key).collect();

        let absent: Vec<WatchKey> = {
            let state = self.inner.state.lock().expect("watch registry state lock poisoned");
            state.repos.keys().filter(|key| !present.contains(key)).cloned().collect()
        };
        for key in &absent {
            do_disarm(&self.inner, key);
            self.inner.epoch_source.forget(key);
            let mut state = self.inner.state.lock().expect("watch registry state lock poisoned");
            state.repos.remove(key);
        }

        for (key, targets, availability) in entries {
            let Some(targets) = targets else {
                do_disarm(&self.inner, key);
                continue;
            };
            if *availability != ws_dashboard_core::WorkRootAvailability::Available {
                do_disarm(&self.inner, key);
                continue;
            }
            if self.inner.config.mode == WatchMode::Off {
                continue;
            }

            let eligible = {
                let mut state = self.inner.state.lock().expect("watch registry state lock poisoned");
                let entry = state
                    .repos
                    .entry(key.clone())
                    .or_insert_with(|| RepoRuntime::new(targets.clone()));
                if !matches!(entry.registration_health, WatchHealth::Armed) {
                    // Keeps an Unarmed/Degraded repo's remembered targets
                    // current, so a re-arm after an outage picks up a
                    // changed git_dir/common_dir without waiting for a
                    // second reconcile tick (D1: kind change across an
                    // outage). An already-Armed repo's targets are refreshed
                    // only through a full disarm/re-arm cycle (`finish_arm`)
                    // - touching this field for an Armed repo here would
                    // desync it from `entry.armed`'s own frozen
                    // git_dir/common_dir/ignore copy.
                    entry.targets = targets.clone();
                }
                arm_eligible(
                    &entry.registration_health,
                    entry.last_arm_attempt_ms,
                    entry.degraded_backoff_ms,
                    now,
                )
            };
            if !eligible {
                continue;
            }

            // Stamp the attempt timestamp synchronously - this IS the value
            // the rate-limit guards above read, so it must be visible before
            // this method returns, not only after the offloaded arm work
            // below finishes (which may be seconds later on a loaded host).
            {
                let mut state = self.inner.state.lock().expect("watch registry state lock poisoned");
                if let Some(entry) = state.repos.get_mut(key) {
                    entry.last_arm_attempt_ms = Some(now);
                }
            }

            let inner = Arc::clone(&self.inner);
            let key = key.clone();
            let targets = targets.clone();
            if let Ok(handle) = tokio::runtime::Handle::try_current() {
                handle.spawn_blocking(move || do_arm(&inner, &key, &targets));
            } else {
                do_arm(&inner, &key, &targets);
            }
        }
    }
}

async fn run_event_loop(
    inner: Arc<RegistryInner>,
    mut events: tokio::sync::mpsc::UnboundedReceiver<notify::Result<Event>>,
) {
    // Ticks the open debounce windows closed independently of new events
    // arriving - a repo whose last write landed just inside a window still
    // needs its trailing bump even if nothing else ever touches it again.
    let mut ticker = tokio::time::interval(std::time::Duration::from_millis(20));
    loop {
        tokio::select! {
            received = events.recv() => {
                match received {
                    Some(Ok(event)) => handle_fs_event(&inner, event),
                    Some(Err(error)) => {
                        tracing::warn!(%error, "filesystem watcher reported an error");
                    }
                    None => break,
                }
            }
            _ = ticker.tick() => {
                flush_debounce_windows(&inner);
            }
        }
    }
}

/// Step 5/6: an event owns a set of repos (via [`owners_for_path`]), each
/// path is classified against that repo's [`ArmedRepo`], and a classified
/// event feeds that repo/kind's [`Debouncer`]. A `need_rescan` event (the OS
/// watcher itself reporting possibly-dropped events) skips classification
/// entirely and degrades every currently-armed repo for one TTL window
/// instead - there is no path to classify against.
///
/// Two side channels ride alongside the classify+debounce hot path:
/// - An ignore-rule-file write ([`is_ignore_rule_file`]) schedules an
///   `IgnoreSet` re-derivation via `spawn_blocking` at most once per 30 s per
///   repo (step 5's `.gitignore` rule), rather than re-deriving inline on
///   this thread.
/// - Linux only: a `Create` event whose path is an in-scope directory
///   registers it incrementally (step 6), so a subsequent write inside it is
///   observed without waiting for the next full re-arm.
///
/// `inner` is `&Arc<RegistryInner>` (not `&RegistryInner`) specifically so
/// the ignore-rederive side channel can clone it into a `spawn_blocking`
/// closure without threading a second parameter through every call in this
/// module.
fn handle_fs_event(inner: &Arc<RegistryInner>, event: Event) {
    let now = clock_ms();
    if event.need_rescan() {
        let mut state = inner.state.lock().expect("watch registry state lock poisoned");
        let until = now.saturating_add(inner.config.armed_ttl_ms);
        let keys: Vec<WatchKey> = state
            .repos
            .iter()
            .filter(|(_, repo)| matches!(repo.registration_health, WatchHealth::Armed))
            .map(|(key, _)| key.clone())
            .collect();
        for key in &keys {
            if let Some(repo) = state.repos.get_mut(key) {
                repo.rescan_degraded_until_ms = Some(until);
                repo.last_event_at_ms = Some(now);
            }
        }
        drop(state);
        for key in &keys {
            inner.epoch_source.bump_worktree(key);
            inner.epoch_source.bump_refs(key);
        }
        return;
    }

    for path in &event.paths {
        let owners = {
            let state = inner.state.lock().expect("watch registry state lock poisoned");
            owners_for_path(&state.dir_index, path)
        };
        if owners.is_empty() {
            continue;
        }

        // Step 6, Linux only: a new in-scope directory registers itself
        // before classification runs, so the mkdir's own event (handled by
        // the classify/debounce block below regardless) and any write that
        // races it both land in an already-registered directory. Runs
        // outside the mutable `state` lock taken below - it takes that same
        // lock internally (and, on cap overflow, calls `do_disarm`, which
        // also locks it), so holding it here would deadlock.
        #[cfg(target_os = "linux")]
        if event.kind.is_create() && path.is_dir() {
            for owner in &owners {
                register_incremental_directory(inner, owner, path);
            }
        }

        let mut rederive_due: Vec<WatchKey> = Vec::new();
        {
            let mut state = inner.state.lock().expect("watch registry state lock poisoned");
            for owner in &owners {
                let Some(repo) = state.repos.get_mut(owner) else {
                    continue;
                };
                repo.last_event_at_ms = Some(now);
                let Some(armed) = repo.armed.as_ref() else {
                    continue;
                };
                if is_ignore_rule_file(path, armed) {
                    repo.ignore_stale = true;
                    let due = repo
                        .last_ignore_rederive_attempt_ms
                        .map(|last| now.saturating_sub(last) >= IGNORE_REDERIVE_INTERVAL_MS)
                        .unwrap_or(true);
                    if due {
                        repo.last_ignore_rederive_attempt_ms = Some(now);
                        rederive_due.push(owner.clone());
                    }
                    let debouncer = repo
                        .debounce
                        .entry(EpochKind::Worktree)
                        .or_insert_with(|| Debouncer::new(inner.config.debounce_ms));
                    if debouncer.record_event(now) == DebounceEvent::BumpNow {
                        inner.epoch_source.bump_worktree(owner);
                    }
                    continue;
                }
                let Some(kind) = classify(path, armed) else {
                    continue;
                };
                let debouncer = repo
                    .debounce
                    .entry(kind)
                    .or_insert_with(|| Debouncer::new(inner.config.debounce_ms));
                if debouncer.record_event(now) == DebounceEvent::BumpNow {
                    match kind {
                        EpochKind::Worktree => inner.epoch_source.bump_worktree(owner),
                        EpochKind::Refs => inner.epoch_source.bump_refs(owner),
                    }
                }
            }
        }

        // The re-derive itself spawns `git status` (and, on Linux, re-walks
        // the tree) - both real I/O, so it runs on the blocking pool rather
        // than this event-consumption thread (ticket step 5: "re-derivation
        // never runs on the event thread").
        for key in rederive_due {
            let inner = Arc::clone(inner);
            tokio::task::spawn_blocking(move || rederive_ignore_set(&inner, &key));
        }
    }
}

/// Ticket step 5: at most one `IgnoreSet` re-derivation per repo per this
/// interval, coalescing every staleness mark inside it into the one
/// re-derivation the first mark already scheduled.
const IGNORE_REDERIVE_INTERVAL_MS: u64 = 30_000;

/// `reconcile`'s flat rate-limit guard (ticket step 7): the minimum interval
/// between arm attempts for an `Unarmed` repo, regardless of why it is
/// `Unarmed`. This is the guard a flapping-availability root actually hits -
/// see [`RepoRuntime::last_arm_attempt_ms`]'s doc comment for why the
/// exponential backoff guard alone does not cover that path.
const MIN_ARM_INTERVAL_MS: u64 = 30_000;

/// `reconcile`'s exponential-backoff guard for a `Degraded` repo (ticket
/// step 7): starting interval before the first retry.
const DEGRADED_BACKOFF_START_MS: u64 = 60_000;

/// Cap on [`DEGRADED_BACKOFF_START_MS`]'s doubling (ticket step 7: "cap
/// 15 min").
const DEGRADED_BACKOFF_CAP_MS: u64 = 900_000;

/// Pure decision function backing `reconcile`'s two independent rate-limit
/// guards (D8), factored out of `reconcile` itself specifically so it is
/// unit-testable against an injected `now_ms` in zero wall-clock time -
/// `reconcile`'s own `now` still comes from the real [`clock_ms`] in
/// production, exactly like `Debouncer`'s injected-clock split.
///
/// `Armed` is never eligible (nothing to do). `Unarmed` is gated by the flat
/// [`MIN_ARM_INTERVAL_MS`] - the guard the availability-flap path actually
/// needs, since a disarm always yields `Unarmed`. `Degraded` is gated by its
/// own `degraded_backoff_ms`, never by the flat interval - `Degraded` must
/// not become arm-eligible just because [`MIN_ARM_INTERVAL_MS`] has passed;
/// only its own (usually much longer) backoff governs it.
fn arm_eligible(
    health: &WatchHealth,
    last_arm_attempt_ms: Option<u64>,
    degraded_backoff_ms: u64,
    now_ms: u64,
) -> bool {
    let interval_ms = match health {
        WatchHealth::Armed => return false,
        WatchHealth::Unarmed => MIN_ARM_INTERVAL_MS,
        WatchHealth::Degraded(_) => degraded_backoff_ms,
    };
    last_arm_attempt_ms
        .map(|last| now_ms.saturating_sub(last) >= interval_ms)
        .unwrap_or(true)
}

/// Pure step of the exponential-backoff bookkeeping `finish_arm` and
/// `set_degraded_after_disarm` both apply (D8): the next `degraded_backoff_ms`
/// given whether the repo was already `Degraded` before this outcome, and
/// whether this outcome is itself `Degraded`. Doubles (capped) only on a
/// *consecutive* `Degraded` outcome; any other transition - a successful arm,
/// or the first degrade coming from `Unarmed`/`Armed` - resets to the
/// starting interval.
fn next_degraded_backoff_ms(was_degraded: bool, new_health_is_degraded: bool, previous_backoff_ms: u64) -> u64 {
    if new_health_is_degraded && was_degraded {
        previous_backoff_ms.saturating_mul(2).min(DEGRADED_BACKOFF_CAP_MS)
    } else {
        DEGRADED_BACKOFF_START_MS
    }
}

fn flush_debounce_windows(inner: &RegistryInner) {
    let now = clock_ms();
    let mut due: Vec<(WatchKey, EpochKind)> = Vec::new();
    {
        let mut state = inner.state.lock().expect("watch registry state lock poisoned");
        for (key, repo) in state.repos.iter_mut() {
            for (kind, debouncer) in repo.debounce.iter_mut() {
                if debouncer.poll_close(now) {
                    due.push((key.clone(), *kind));
                }
            }
        }
    }
    for (key, kind) in due {
        match kind {
            EpochKind::Worktree => inner.epoch_source.bump_worktree(&key),
            EpochKind::Refs => inner.epoch_source.bump_refs(&key),
        }
    }
}

/// Which repos own `path`: every `dir_index` entry whose registered
/// directory is an ancestor of (or equal to) `path`. On Linux this is
/// normally a single directory match per event (the walk registers
/// individual directories `NonRecursive`, so an event's parent is exactly
/// one registered entry) but a shared `common_dir` target still yields
/// several owners for one event, which is the fanout D2 requires. On
/// Windows/macOS (recursive roots), this is the only way to attribute a deep
/// event at all - the registered entry is the worktree/git-dir root, several
/// path components above the event itself.
fn owners_for_path(dir_index: &HashMap<PathBuf, HashSet<WatchKey>>, path: &Path) -> HashSet<WatchKey> {
    let mut owners = HashSet::new();
    for (target, keys) in dir_index {
        if path.starts_with(target) {
            owners.extend(keys.iter().cloned());
        }
    }
    owners
}

/// Replace `key`'s previously-registered directories with `new_dirs` in
/// `dir_index`, unregistering (via the shared `notify` watcher) any
/// directory no repo owns afterward. Shared by `finish_arm` (re-arm) and
/// `do_disarm` (`new_dirs` is empty) so the dedup/fanout bookkeeping has one
/// implementation regardless of which direction triggered it.
fn update_dir_index(inner: &RegistryInner, key: &WatchKey, old_dirs: &[PathBuf], new_dirs: &[PathBuf]) {
    let new_set: HashSet<&PathBuf> = new_dirs.iter().collect();
    let mut to_unwatch = Vec::new();
    {
        let mut state = inner.state.lock().expect("watch registry state lock poisoned");
        for dir in old_dirs {
            if new_set.contains(dir) {
                continue;
            }
            if let Some(owners) = state.dir_index.get_mut(dir) {
                owners.remove(key);
                if owners.is_empty() {
                    state.dir_index.remove(dir);
                    to_unwatch.push(dir.clone());
                }
            }
        }
        for dir in new_dirs {
            state.dir_index.entry(dir.clone()).or_default().insert(key.clone());
        }
    }
    if !to_unwatch.is_empty() {
        let mut watcher_guard = inner.watcher.lock().expect("watcher lock poisoned");
        if let Some(watcher) = watcher_guard.as_mut() {
            for dir in &to_unwatch {
                if let Err(error) = watcher.unwatch(dir) {
                    tracing::debug!(?dir, %error, "unwatch failed (already gone is fine)");
                }
            }
        }
    }
}

/// Finish an arm attempt: record the outcome in `state.repos`, reconcile
/// `dir_index`, and - on a successful transition to [`WatchHealth::Armed`] -
/// bump both epochs. That last part is the shared post-arm rule (ticket
/// Constraints): a slot filled while `Unarmed` carries epoch 0, and without
/// this bump it stays valid for the whole armed TTL even though it was
/// computed during a window with no watcher running, including whatever
/// changed *during* arming itself (the `git status` spawn and, on Linux, the
/// directory walk both take real wall-clock time against a live worktree).
fn finish_arm(
    inner: &RegistryInner,
    key: &WatchKey,
    targets: &WatchTargets,
    health: WatchHealth,
    registered_dirs: Vec<PathBuf>,
    ignore: IgnoreSet,
) {
    let now = clock_ms();
    let armed_repo = matches!(health, WatchHealth::Armed).then(|| ArmedRepo {
        git_dir: targets.git_dir.clone(),
        common_dir: targets.common_dir.clone(),
        ignore,
    });

    let old_dirs = {
        let mut state = inner.state.lock().expect("watch registry state lock poisoned");
        let entry = state
            .repos
            .entry(key.clone())
            .or_insert_with(|| RepoRuntime::new(targets.clone()));
        let old_dirs = std::mem::take(&mut entry.registered_dirs);
        // Exponential-backoff bookkeeping (ticket step 7, D8's two rate-limit
        // guards): double only on a *consecutive* Degraded outcome; any other
        // transition (a successful arm, or the first degrade coming from
        // Unarmed/Armed) resets to the starting interval, so a repo that
        // degrades again later starts a fresh backoff cycle rather than
        // inheriting a stale, possibly-maxed-out value.
        let was_degraded = matches!(entry.registration_health, WatchHealth::Degraded(_));
        entry.degraded_backoff_ms = next_degraded_backoff_ms(
            was_degraded,
            matches!(health, WatchHealth::Degraded(_)),
            entry.degraded_backoff_ms,
        );
        entry.targets = targets.clone();
        entry.registration_health = health.clone();
        entry.armed = armed_repo;
        entry.registered_dirs = registered_dirs.clone();
        entry.last_arm_attempt_ms = Some(now);
        entry.rescan_degraded_until_ms = None;
        entry.ignore_stale = false;
        old_dirs
    };
    update_dir_index(inner, key, &old_dirs, &registered_dirs);

    if matches!(health, WatchHealth::Armed) {
        inner.epoch_source.bump_worktree(key);
        inner.epoch_source.bump_refs(key);
    }
}

fn do_disarm(inner: &RegistryInner, key: &WatchKey) {
    let old_dirs = {
        let mut state = inner.state.lock().expect("watch registry state lock poisoned");
        let Some(repo) = state.repos.get_mut(key) else {
            return;
        };
        repo.registration_health = WatchHealth::Unarmed;
        repo.armed = None;
        repo.rescan_degraded_until_ms = None;
        repo.debounce.clear();
        std::mem::take(&mut repo.registered_dirs)
    };
    update_dir_index(inner, key, &old_dirs, &[]);
    // Shared post-arm/disarm rule (see `finish_arm`'s doc comment): the next
    // poll must recompute rather than serve a slot stamped valid while this
    // repo was still armed.
    inner.epoch_source.bump_worktree(key);
    inner.epoch_source.bump_refs(key);
}

/// Sets `key`'s health to `Degraded(reason)` and applies `finish_arm`'s same
/// backoff bookkeeping (double only on a consecutive `Degraded`, reset
/// otherwise), for the two call sites that disarm-and-degrade a repo without
/// going through `finish_arm` itself: `register_incremental_directory`'s cap
/// overflow and `rederive_ignore_set`'s Linux `TooLarge`/arm-error path.
/// Skipping this bookkeeping at either site would let those repos bypass
/// `reconcile`'s exponential-backoff guard entirely (D8) - they would always
/// read as freshly-degraded (`degraded_backoff_ms` still at its default) and
/// retry every 60 s forever instead of backing off.
fn set_degraded_after_disarm(inner: &RegistryInner, key: &WatchKey, reason: String) {
    let now = clock_ms();
    let mut state = inner.state.lock().expect("watch registry state lock poisoned");
    let Some(repo) = state.repos.get_mut(key) else {
        return;
    };
    let was_degraded = matches!(repo.registration_health, WatchHealth::Degraded(_));
    repo.degraded_backoff_ms = next_degraded_backoff_ms(was_degraded, true, repo.degraded_backoff_ms);
    repo.registration_health = WatchHealth::Degraded(reason);
    repo.last_arm_attempt_ms = Some(now);
}

fn do_arm(inner: &RegistryInner, key: &WatchKey, targets: &WatchTargets) {
    if inner.config.mode == WatchMode::Off {
        finish_arm(inner, key, targets, WatchHealth::Unarmed, Vec::new(), IgnoreSet::empty());
        return;
    }
    let forced = inner.config.mode == WatchMode::Force;
    let mount_allows = mount_allows_watching(&targets.worktree);
    if !mount_allows {
        if !forced {
            finish_arm(
                inner,
                key,
                targets,
                WatchHealth::Degraded("filesystem does not deliver events".to_owned()),
                Vec::new(),
                IgnoreSet::empty(),
            );
            return;
        }
        // Ticket step 9: `force` is a diagnose-a-suspected-wrong-allowlist
        // escape hatch, not a supported production mode - log every time it
        // actually overrides the gate (bounded by the same 30s/backoff
        // rate limit as any other arm attempt, so this cannot spam) so an
        // operator who forgot to unset it notices in the logs, not only by
        // reading `diag/git`.
        tracing::warn!(
            worktree = %targets.worktree.display(),
            "WS_DASHBOARD_GIT_WATCH=force is arming a repo on a filesystem \
             the mount allowlist would otherwise reject"
        );
    }

    let ignore = IgnoreSet::derive(&targets.worktree, &inner.git_stats);

    // A fresh arm is the "diff against nothing" case of the same
    // registration-diff logic the ignore-set re-derive uses (see
    // `linux_plan_and_apply`'s doc comment).
    #[cfg(target_os = "linux")]
    let outcome = linux_plan_and_apply(inner, targets, &ignore, &[]);
    #[cfg(not(target_os = "linux"))]
    let outcome = do_arm_recursive(inner, targets);

    match outcome {
        Ok(dirs) => finish_arm(inner, key, targets, WatchHealth::Armed, dirs, ignore),
        Err(reason) => finish_arm(inner, key, targets, WatchHealth::Degraded(reason), Vec::new(), ignore),
    }
}

/// Linux backend: the gitignore-aware walk ([`plan_watch_set`]) plus a
/// per-directory `NonRecursive` registration, all-or-nothing - a partial
/// registration would leave a repo reporting `Armed` while silently missing
/// events under whichever directory failed, which is the exact
/// worse-than-`Unarmed` failure mode the pre-arm mount gate exists to avoid
/// for the whole-filesystem case.
///
/// Applies the *difference* against `old_dirs` rather than registering the
/// full planned set unconditionally: `finish_arm`'s `update_dir_index` call
/// (run by every caller right after this returns) already unregisters
/// whatever is no longer planned, so this function only needs to watch what
/// is newly planned. A fresh arm (`old_dirs` empty) and an ignore-set
/// re-derive's registration change (`old_dirs` the repo's current
/// `registered_dirs`) are therefore the same operation - ticket step 5: "a
/// diff, not a disarm-and-re-arm".
#[cfg(target_os = "linux")]
fn linux_plan_and_apply(
    inner: &RegistryInner,
    targets: &WatchTargets,
    ignore: &IgnoreSet,
    old_dirs: &[PathBuf],
) -> Result<Vec<PathBuf>, String> {
    let dirs = plan_watch_set(
        &targets.worktree,
        &targets.git_dir,
        &targets.common_dir,
        ignore,
        inner.config.max_dirs,
    )
    .map_err(|TooLarge { found }| format!("watch set too large: {found} dirs"))?;

    let old_set: HashSet<&PathBuf> = old_dirs.iter().collect();
    let to_add: Vec<PathBuf> = dirs.iter().filter(|dir| !old_set.contains(dir)).cloned().collect();

    let existing: HashSet<PathBuf> = {
        let state = inner.state.lock().expect("watch registry state lock poisoned");
        state.dir_index.keys().cloned().collect()
    };
    let new_dir_count = to_add.iter().filter(|dir| !existing.contains(*dir)).count();
    {
        let state = inner.state.lock().expect("watch registry state lock poisoned");
        let used = state.dir_index.len();
        if used + new_dir_count > inner.linux_process_budget {
            return Err("process-wide inotify budget exhausted".to_owned());
        }
    }

    let mut watcher_guard = inner.watcher.lock().expect("watcher lock poisoned");
    let Some(watcher) = watcher_guard.as_mut() else {
        return Err("watcher unavailable".to_owned());
    };
    let mut newly_registered: Vec<PathBuf> = Vec::new();
    for dir in &to_add {
        if existing.contains(dir) {
            // Already registered - either a sibling repo sharing this
            // target (`common_dir` dedup, D2), or this exact directory was
            // already registered incrementally (step 6) before this diff
            // ran.
            continue;
        }
        if let Err(error) = watcher.watch(dir.as_path(), RecursiveMode::NonRecursive) {
            tracing::warn!(?dir, %error, "failed to register inotify watch; unwinding this registration change");
            for registered in &newly_registered {
                let _ = watcher.unwatch(registered);
            }
            return Err(format!("arm error: {error}"));
        }
        newly_registered.push(dir.clone());
    }
    Ok(dirs)
}

/// Step 6, Linux only: a `Create` event for an in-scope directory
/// (`dir` - not ignored, not under `git_dir`/`common_dir`, not already
/// registered) registers it immediately and increments the repo's directory
/// count; crossing the per-repo cap disarms the whole repo rather than
/// leaving it half-covered (ticket step 6, mirroring `plan_watch_set`'s own
/// bail-out). A no-op for any repo that is not currently `Armed`, or where
/// `dir` fails the same admission checks `plan_watch_set` would have applied
/// at arm time.
#[cfg(target_os = "linux")]
fn register_incremental_directory(inner: &RegistryInner, key: &WatchKey, dir: &Path) {
    let over_cap = {
        let state = inner.state.lock().expect("watch registry state lock poisoned");
        let Some(repo) = state.repos.get(key) else {
            return;
        };
        if !matches!(repo.registration_health, WatchHealth::Armed) {
            return;
        }
        let Some(armed) = repo.armed.as_ref() else {
            return;
        };
        if armed.ignore.matches(dir) {
            return;
        }
        if dir.starts_with(&armed.git_dir) || dir.starts_with(&armed.common_dir) {
            return;
        }
        if state.dir_index.contains_key(dir) {
            return;
        }
        repo.registered_dirs.len() + 1 > inner.config.max_dirs
    };

    if over_cap {
        do_disarm(inner, key);
        set_degraded_after_disarm(inner, key, "watch set outgrew cap".to_owned());
        return;
    }

    let watched = {
        let mut watcher_guard = inner.watcher.lock().expect("watcher lock poisoned");
        match watcher_guard.as_mut() {
            Some(watcher) => watcher.watch(dir, RecursiveMode::NonRecursive).is_ok(),
            None => false,
        }
    };
    if !watched {
        return;
    }

    let mut state = inner.state.lock().expect("watch registry state lock poisoned");
    let Some(repo) = state.repos.get_mut(key) else {
        return;
    };
    // Re-check health under the write lock: a concurrent disarm (e.g. this
    // same repo's cap was crossed by a sibling's incremental registration
    // between the read above and this write) must not resurrect bookkeeping
    // for a repo that is no longer armed.
    if !matches!(repo.registration_health, WatchHealth::Armed) {
        return;
    }
    repo.registered_dirs.push(dir.to_path_buf());
    state.dir_index.entry(dir.to_path_buf()).or_default().insert(key.clone());
}

/// Ticket step 5's `.gitignore`/`info/exclude` re-derivation: recompute the
/// `IgnoreSet`, then - Linux only - apply the registration diff it implies
/// (`linux_plan_and_apply`, disarming wholly on `TooLarge`); non-Linux swaps
/// only the in-memory filter, since recursive registration needs no
/// registration change when the filter changes. Bumps both epochs on success
/// either way: "the filter changed and previously-suppressed paths may now
/// be significant" (ticket step 5). Runs on the blocking pool - see
/// `handle_fs_event`'s `spawn_blocking` call site.
fn rederive_ignore_set(inner: &RegistryInner, key: &WatchKey) {
    let Some(targets) = ({
        let state = inner.state.lock().expect("watch registry state lock poisoned");
        state
            .repos
            .get(key)
            .filter(|repo| matches!(repo.registration_health, WatchHealth::Armed))
            .map(|repo| repo.targets.clone())
    }) else {
        return;
    };

    let ignore = IgnoreSet::derive(&targets.worktree, &inner.git_stats);

    #[cfg(target_os = "linux")]
    {
        let old_dirs = {
            let state = inner.state.lock().expect("watch registry state lock poisoned");
            state.repos.get(key).map(|repo| repo.registered_dirs.clone()).unwrap_or_default()
        };
        match linux_plan_and_apply(inner, &targets, &ignore, &old_dirs) {
            Ok(new_dirs) => apply_rederive_success(inner, key, ignore, new_dirs),
            Err(_) => {
                do_disarm(inner, key);
                set_degraded_after_disarm(inner, key, "watch set outgrew cap".to_owned());
            }
        }
    }

    #[cfg(not(target_os = "linux"))]
    {
        let existing_dirs = {
            let state = inner.state.lock().expect("watch registry state lock poisoned");
            state.repos.get(key).map(|repo| repo.registered_dirs.clone()).unwrap_or_default()
        };
        apply_rederive_success(inner, key, ignore, existing_dirs);
    }
}

/// Shared success path for [`rederive_ignore_set`]: swap the `ArmedRepo`'s
/// `IgnoreSet` (the copy `classify` actually consults), reconcile
/// `registered_dirs`/`dir_index` against `new_dirs` (a no-op on non-Linux,
/// where `new_dirs` is always the unchanged existing set), clear the
/// staleness flag, and bump both epochs.
fn apply_rederive_success(inner: &RegistryInner, key: &WatchKey, ignore: IgnoreSet, new_dirs: Vec<PathBuf>) {
    let old_dirs = {
        let mut state = inner.state.lock().expect("watch registry state lock poisoned");
        let Some(repo) = state.repos.get_mut(key) else {
            return;
        };
        let old_dirs = std::mem::replace(&mut repo.registered_dirs, new_dirs.clone());
        if let Some(armed) = repo.armed.as_mut() {
            armed.ignore = ignore;
        }
        repo.ignore_stale = false;
        old_dirs
    };
    update_dir_index(inner, key, &old_dirs, &new_dirs);
    inner.epoch_source.bump_worktree(key);
    inner.epoch_source.bump_refs(key);
}

/// Windows/macOS backend: one kernel-level recursive watch per target (at
/// most three: worktree, `git_dir`, `common_dir` - deduplicated when they
/// coincide, as for a primary root where `git_dir == common_dir`), no walk,
/// no cap (Lead Disposition, ticket Decisions: recursive registration has no
/// analogue to inotify's per-process watch-count ceiling).
#[cfg(not(target_os = "linux"))]
fn do_arm_recursive(inner: &RegistryInner, targets: &WatchTargets) -> Result<Vec<PathBuf>, String> {
    let mut roots = vec![targets.worktree.clone()];
    if !roots.contains(&targets.git_dir) {
        roots.push(targets.git_dir.clone());
    }
    if !roots.contains(&targets.common_dir) {
        roots.push(targets.common_dir.clone());
    }

    let mut watcher_guard = inner.watcher.lock().expect("watcher lock poisoned");
    let Some(watcher) = watcher_guard.as_mut() else {
        return Err("watcher unavailable".to_owned());
    };
    // Explicit `Vec<PathBuf>` annotation: `watcher.unwatch(done)` below takes
    // `&Path`, and without a fixed element type up front rustc's inference
    // can settle on the unsized `Vec<Path>` from that call site alone (the
    // same pattern hit and fixed this way in the Linux incremental-planner
    // predecessor of this function).
    let mut registered: Vec<PathBuf> = Vec::new();
    for root in &roots {
        if let Err(error) = watcher.watch(root, RecursiveMode::Recursive) {
            tracing::warn!(?root, %error, "failed to register recursive watch; unwinding this arm attempt");
            for done in &registered {
                let _ = watcher.unwatch(done);
            }
            return Err(format!("arm error: {error}"));
        }
        registered.push(root.clone());
    }
    Ok(registered)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn temp_dir(name: &str) -> PathBuf {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system time after epoch")
            .as_nanos();
        std::env::temp_dir().join(format!("ws-dashboard-watch-{name}-{unique}"))
    }

    fn remove_temp(path: &Path) {
        let _ = fs::remove_dir_all(path);
    }

    fn primary_repo(ignore: IgnoreSet) -> ArmedRepo {
        ArmedRepo {
            git_dir: PathBuf::from("/repo/.git"),
            common_dir: PathBuf::from("/repo/.git"),
            ignore,
        }
    }

    fn linked_repo(ignore: IgnoreSet) -> ArmedRepo {
        ArmedRepo {
            git_dir: PathBuf::from("/repo/.git/worktrees/feature"),
            common_dir: PathBuf::from("/repo/.git"),
            ignore,
        }
    }

    fn p(s: &str) -> PathBuf {
        PathBuf::from(s)
    }

    // The ~25-case classify table (ticket Phase 4 Verification boundary /
    // Lead Disposition D5). Do NOT attempt to assert branch order - a pure
    // function cannot expose which branches ran, so order is pinned by
    // outcomes, not by introspection.
    #[test]
    fn classify_table() {
        let ignore = IgnoreSet::from_parts(
            [p("/repo/target/"), p("/repo/untracked_dir/build/")],
            [p("/repo/ai-docs/_index.local.md")],
        );
        let repo = primary_repo(ignore);

        let cases: &[(&str, PathBuf, Option<EpochKind>)] = &[
            (
                "ignore-set directory prefix match",
                p("/repo/target/debug/foo.o"),
                None,
            ),
            (
                "ignore-set nested ignored dir inside an untracked dir",
                p("/repo/untracked_dir/build/foo.o"),
                None,
            ),
            (
                "ignore-set exact FILE match (set is not directories-only)",
                p("/repo/ai-docs/_index.local.md"),
                None,
            ),
            (".gitignore anywhere -> Worktree", p("/repo/.gitignore"), Some(EpochKind::Worktree)),
            (
                "nested .gitignore -> Worktree",
                p("/repo/src/.gitignore"),
                Some(EpochKind::Worktree),
            ),
            (
                "common_dir/info/exclude -> Worktree, not dropped into the git-dir arm",
                p("/repo/.git/info/exclude"),
                Some(EpochKind::Worktree),
            ),
            ("git_dir objects/ -> ignore", p("/repo/.git/objects/ab/cdef"), None),
            (
                "common_dir objects/pack -> ignore",
                p("/repo/.git/objects/pack/pack-x.pack"),
                None,
            ),
            ("git_dir lfs/ -> ignore", p("/repo/.git/lfs/objects/00/aa"), None),
            ("git_dir modules/ -> ignore", p("/repo/.git/modules/sub/HEAD"), None),
            ("index.lock -> ignore", p("/repo/.git/index.lock"), None),
            ("HEAD.lock -> ignore", p("/repo/.git/HEAD.lock"), None),
            (
                "refs/heads/*.lock -> ignore (lock wins over refs/**)",
                p("/repo/.git/refs/heads/main.lock"),
                None,
            ),
            ("HEAD -> Refs", p("/repo/.git/HEAD"), Some(EpochKind::Refs)),
            ("packed-refs -> Refs", p("/repo/.git/packed-refs"), Some(EpochKind::Refs)),
            ("FETCH_HEAD -> Refs", p("/repo/.git/FETCH_HEAD"), Some(EpochKind::Refs)),
            ("ORIG_HEAD -> Refs", p("/repo/.git/ORIG_HEAD"), Some(EpochKind::Refs)),
            (
                "refs/heads/main -> Refs",
                p("/repo/.git/refs/heads/main"),
                Some(EpochKind::Refs),
            ),
            (
                "worktrees/<name>/HEAD -> Refs",
                p("/repo/.git/worktrees/feature/HEAD"),
                Some(EpochKind::Refs),
            ),
            ("index -> Worktree", p("/repo/.git/index"), Some(EpochKind::Worktree)),
            (
                "common_dir/config -> ignore via explicit git-dir fallthrough",
                p("/repo/.git/config"),
                None,
            ),
            (
                "COMMIT_EDITMSG -> ignore via explicit git-dir fallthrough",
                p("/repo/.git/COMMIT_EDITMSG"),
                None,
            ),
            ("hooks/ -> ignore", p("/repo/.git/hooks/pre-commit"), None),
            ("logs/ -> ignore", p("/repo/.git/logs/HEAD"), None),
            (
                "info/attributes (not info/exclude) -> ignore via fallthrough",
                p("/repo/.git/info/attributes"),
                None,
            ),
            (
                "ordinary worktree file -> Worktree (default case)",
                p("/repo/src/main.rs"),
                Some(EpochKind::Worktree),
            ),
        ];

        for (description, path, expected) in cases {
            assert_eq!(
                classify(path, &repo),
                *expected,
                "case failed: {description} ({path:?})"
            );
        }
    }

    #[test]
    fn classify_linked_worktree_git_dir_index_is_worktree() {
        let repo = linked_repo(IgnoreSet::empty());
        assert_eq!(
            classify(&p("/repo/.git/worktrees/feature/index"), &repo),
            Some(EpochKind::Worktree)
        );
        assert_eq!(
            classify(&p("/repo/.git/worktrees/feature/HEAD"), &repo),
            Some(EpochKind::Refs)
        );
        // Shared common_dir refs are still visible/classified from the
        // linked worktree's ArmedRepo too (both `git_dir` and `common_dir`
        // are checked).
        assert_eq!(
            classify(&p("/repo/.git/refs/heads/main"), &repo),
            Some(EpochKind::Refs)
        );
    }

    #[test]
    fn is_ignore_rule_file_pins_gitignore_and_info_exclude_only() {
        let repo = primary_repo(IgnoreSet::empty());
        assert!(is_ignore_rule_file(&p("/repo/.gitignore"), &repo));
        assert!(is_ignore_rule_file(&p("/repo/src/.gitignore"), &repo));
        assert!(is_ignore_rule_file(&p("/repo/.git/info/exclude"), &repo));
        assert!(!is_ignore_rule_file(&p("/repo/.git/info/attributes"), &repo));
        assert!(!is_ignore_rule_file(&p("/repo/src/main.rs"), &repo));
    }

    #[test]
    fn empty_ignore_set_matches_nothing() {
        assert!(!IgnoreSet::empty().matches(&p("/repo/target/anything")));
    }

    // --- IgnoreSet::derive / parse_status_z_output -------------------------

    #[test]
    fn ignore_derive_argv_pins_unormal_not_uno() {
        // The measured failure in the ticket's Decisions is silent (`-uno`
        // still "succeeds", it just reports zero ignored entries), so this
        // needs a pin rather than a comment.
        assert!(IGNORE_DERIVE_ARGS.contains(&"-unormal"));
        assert!(!IGNORE_DERIVE_ARGS.contains(&"-uno"));
    }

    #[test]
    fn ignore_set_parses_fixed_status_z_bytes_directories_and_files_separately() {
        let stdout =
            "!! target/\0?? untracked.txt\0!! ai-docs/_index.local.md\0!! frontend/node_modules/\0";
        let set = IgnoreSet::parse_status_z_output(stdout, Path::new("/repo"));

        assert!(
            set.matches(&p("/repo/target/debug/foo.o")),
            "directory entry must become a prefix match"
        );
        assert!(set.matches(&p("/repo/frontend/node_modules/pkg/index.js")));
        assert!(
            set.matches(&p("/repo/ai-docs/_index.local.md")),
            "file entry must become an exact match"
        );
        assert!(
            !set.matches(&p("/repo/ai-docs/other.md")),
            "a file entry must not become a directory-style prefix match"
        );
        assert!(
            !set.matches(&p("/repo/untracked.txt")),
            "`??` (untracked, not ignored) entries must not be collected"
        );
        assert!(!set.matches(&p("/repo/src/main.rs")));
    }

    #[test]
    fn ignore_set_parse_ignores_empty_and_malformed_records() {
        let stdout = "\0!! \0garbage\0!! real/";
        let set = IgnoreSet::parse_status_z_output(stdout, Path::new("/repo"));
        assert!(set.matches(&p("/repo/real/anything")));
        assert!(!set.matches(&p("/repo/garbage")));
    }

    // --- plan_watch_set ------------------------------------------------

    #[test]
    fn plan_watch_set_prunes_ignored_and_git_internal_dirs_and_registers_explicit_targets() {
        let base = temp_dir("plan-basic");
        let worktree = base.join("repo");
        let git_dir = worktree.join(".git");
        fs::create_dir_all(worktree.join("src")).expect("create src");
        fs::create_dir_all(worktree.join("target/debug")).expect("create target");
        fs::create_dir_all(git_dir.join("refs/heads")).expect("create refs/heads");
        fs::create_dir_all(git_dir.join("objects/pack")).expect("create objects/pack");
        fs::create_dir_all(git_dir.join("info")).expect("create info");

        let ignore = IgnoreSet::from_parts([worktree.join("target")], []);
        let result = plan_watch_set(&worktree, &git_dir, &git_dir, &ignore, 1024)
            .expect("plan must succeed under a generous cap");

        assert!(result.contains(&worktree), "worktree root must be registered");
        assert!(result.contains(&worktree.join("src")));
        assert!(
            !result.iter().any(|path| path.starts_with(worktree.join("target"))),
            "an ignored directory must be pruned from registration, not just classify"
        );
        assert_eq!(
            result.iter().filter(|path| **path == git_dir).count(),
            1,
            "git_dir/common_dir (same path here) must be registered exactly once, \
             via the explicit target append, not duplicated by the worktree walk"
        );
        assert!(
            result.contains(&git_dir.join("info")),
            "common_dir/info/ must be registered so classify's info/exclude rule is reachable"
        );
        assert!(result.contains(&git_dir.join("refs")));
        assert!(
            result.contains(&git_dir.join("refs/heads")),
            "refs/** must be a real recursive walk, not just the refs/ top level"
        );
        assert!(
            !result.iter().any(|path| path.starts_with(git_dir.join("objects"))),
            "objects/ must never be registered - it is excluded from registration \
             entirely (Constraints), not merely filtered by classify"
        );

        remove_temp(&base);
    }

    #[test]
    fn plan_watch_set_registers_linked_worktree_targets_under_worktrees_subtree() {
        let base = temp_dir("plan-linked");
        let worktree = base.join("linked");
        let common_dir = base.join("primary/.git");
        let git_dir = common_dir.join("worktrees/linked");
        fs::create_dir_all(&worktree).expect("create linked worktree");
        fs::create_dir_all(git_dir.join("nested")).expect("create per-worktree git dir");
        fs::create_dir_all(common_dir.join("refs/heads")).expect("create refs");

        let result = plan_watch_set(&worktree, &git_dir, &common_dir, &IgnoreSet::empty(), 1024)
            .expect("plan must succeed");

        assert!(result.contains(&worktree));
        assert!(result.contains(&common_dir));
        assert!(
            result.contains(&git_dir),
            "the linked worktree's own git_dir must be reachable via the \
             common_dir/worktrees/** recursive walk"
        );
        assert!(result.contains(&git_dir.join("nested")));

        remove_temp(&base);
    }

    #[test]
    fn plan_watch_set_returns_too_large_as_soon_as_cap_is_crossed_not_after_a_full_walk() {
        let base = temp_dir("plan-cap");
        let worktree = base.join("repo");
        let git_dir = worktree.join(".git");
        fs::create_dir_all(&git_dir).expect("create git dir");
        for index in 0..10 {
            fs::create_dir_all(worktree.join(format!("dir{index}"))).expect("create subdir");
        }

        let error = plan_watch_set(&worktree, &git_dir, &git_dir, &IgnoreSet::empty(), 3)
            .expect_err("a tree past the cap must return TooLarge");
        assert!(
            error.found <= 5,
            "the walk must bail immediately on crossing the cap, not enumerate the \
             remaining ~10 siblings first (found={})",
            error.found
        );
        assert!(error.found > 3);

        remove_temp(&base);
    }

    // --- Debouncer -------------------------------------------------------

    #[test]
    fn debounce_single_event_bumps_leading_only_no_trailing_bump() {
        let mut debouncer = Debouncer::new(100);
        assert_eq!(debouncer.record_event(0), DebounceEvent::BumpNow);
        // Window closes (100ms since the only event); no further event
        // arrived, so no trailing bump.
        assert!(!debouncer.poll_close(100));
        assert!(!debouncer.is_open(), "the window must actually close");
    }

    #[test]
    fn debounce_two_events_in_one_window_bump_exactly_twice() {
        let mut debouncer = Debouncer::new(100);
        assert_eq!(debouncer.record_event(0), DebounceEvent::BumpNow);
        assert_eq!(debouncer.record_event(50), DebounceEvent::Deferred);
        // Not yet due: last event at 50, only 40ms have passed.
        assert!(!debouncer.poll_close(90));
        assert!(debouncer.poll_close(150), "trailing bump must fire once the window closes");
    }

    #[test]
    fn debounce_a_thousand_events_in_one_window_still_bump_exactly_twice() {
        let mut debouncer = Debouncer::new(100);
        assert_eq!(debouncer.record_event(0), DebounceEvent::BumpNow);
        for tick in 1..1000u64 {
            assert_eq!(debouncer.record_event(tick % 90), DebounceEvent::Deferred);
        }
        assert!(debouncer.poll_close(1200));
    }

    #[test]
    fn debounce_window_reopens_after_closing() {
        let mut debouncer = Debouncer::new(100);
        assert_eq!(debouncer.record_event(0), DebounceEvent::BumpNow);
        assert!(!debouncer.poll_close(100));
        // A new event after the window closed opens a fresh window and
        // bumps leading again.
        assert_eq!(debouncer.record_event(500), DebounceEvent::BumpNow);
    }

    #[test]
    fn debounce_continuous_events_still_close_at_the_five_x_cap() {
        // A never-ending stream of events (each one within window_ms of the
        // last) must not suppress every bump forever - the cap bounds it.
        let mut debouncer = Debouncer::new(100);
        assert_eq!(debouncer.record_event(0), DebounceEvent::BumpNow);
        let mut now = 0u64;
        while now < 480 {
            now += 30;
            assert_eq!(debouncer.record_event(now), DebounceEvent::Deferred);
            assert!(
                !debouncer.poll_close(now),
                "must not close yet at t={now} (window keeps re-extending, cap not hit)"
            );
        }
        // At t=500 the cap (5 * 100ms = 500ms from the first event at t=0)
        // is crossed even though the last event was recent.
        assert!(debouncer.poll_close(500), "the 5x cap must force a close");
    }

    // --- mount allowlist (Linux) -----------------------------------------

    #[cfg(target_os = "linux")]
    #[test]
    fn mount_allows_watching_allows_the_process_temp_dir() {
        // TMPDIR-backed test fixtures are local VFS (tmpfs or the disk
        // filesystem backing /tmp) and must not be excluded by the
        // allowlist, per the ticket Constraints - otherwise Phase 4's own
        // integration tier would fail on any host whose temp dir is not on a
        // disk filesystem.
        let dir = std::env::temp_dir();
        assert!(
            mount_allows_watching(&dir),
            "the process temp dir ({dir:?}) must be on an allowlisted local filesystem"
        );
    }

    #[cfg(target_os = "linux")]
    #[test]
    fn mount_allows_watching_rejects_wsl2_drvfs_mnt_when_present() {
        // The concrete case the ticket's Constraints call out: `/mnt/*`
        // (drvfs/9p) is this project's own dogfood WSL2 topology and must
        // never report as watch-allowed. Skip on a host with no `/mnt/c`
        // rather than asserting a specific mount exists.
        let probe = Path::new("/mnt/c");
        if !probe.exists() {
            return;
        }
        assert!(
            !mount_allows_watching(probe),
            "/mnt/c (drvfs/9p) must be rejected by the allowlist"
        );
    }

    #[cfg(target_os = "linux")]
    #[test]
    fn mount_allows_watching_rejects_an_unresolvable_path_under_a_foreign_mount() {
        // A path that does not exist yet, under a foreign mount, must still
        // resolve to that mount's (rejected) fstype via the raw-path
        // fallback rather than defaulting to the (locally allowlisted) root
        // filesystem.
        let probe = Path::new("/mnt/c/this-does-not-exist-ws-dashboard-fixture");
        if !Path::new("/mnt/c").exists() {
            return;
        }
        assert!(!mount_allows_watching(probe));
    }

    #[cfg(target_os = "linux")]
    #[test]
    fn linux_inotify_process_budget_is_bounded_by_the_8192_ceiling() {
        assert!(linux_inotify_process_budget() <= 8_192);
    }

    // --- reconcile: the two rate-limit guards, as SEPARATE tests (D8) -----
    //
    // Each test exercises `arm_eligible` (the pure decision function
    // `reconcile` delegates to) with an injected `now_ms`, exactly like
    // `Debouncer`'s tests above - the 30s/60s+ intervals run in zero
    // wall-clock time. Written so each fails if *only its own* guard were
    // removed: test 1 would start passing at any elapsed time if the flat
    // interval were deleted from the `Unarmed` arm; test 2 would start
    // passing at 30s (rather than needing the full 60s backoff) if
    // `Degraded` fell back to the flat interval instead of its own backoff.

    #[test]
    fn reconcile_unarmed_arm_attempts_are_rate_limited_by_the_flat_30s_interval() {
        // Never attempted before -> eligible immediately.
        assert!(arm_eligible(&WatchHealth::Unarmed, None, DEGRADED_BACKOFF_START_MS, 0));
        // 1ms short of the flat interval -> not yet eligible.
        assert!(!arm_eligible(
            &WatchHealth::Unarmed,
            Some(0),
            DEGRADED_BACKOFF_START_MS,
            MIN_ARM_INTERVAL_MS - 1
        ));
        // Exactly at the flat interval -> eligible.
        assert!(arm_eligible(
            &WatchHealth::Unarmed,
            Some(0),
            DEGRADED_BACKOFF_START_MS,
            MIN_ARM_INTERVAL_MS
        ));
    }

    #[test]
    fn reconcile_degraded_arm_attempts_are_rate_limited_by_their_own_backoff_not_the_flat_interval() {
        let backoff = DEGRADED_BACKOFF_START_MS; // 60_000ms, double the flat 30_000ms interval.
        // Past the flat 30s interval but short of the 60s backoff -> a
        // `Degraded` repo must still NOT be eligible. This is the exact
        // guard the ticket's Constraints warn a naive "not Armed => eligible"
        // implementation would skip.
        assert!(!arm_eligible(&WatchHealth::Degraded("x".to_owned()), Some(0), backoff, MIN_ARM_INTERVAL_MS));
        // 1ms short of its own backoff -> still not eligible.
        assert!(!arm_eligible(&WatchHealth::Degraded("x".to_owned()), Some(0), backoff, backoff - 1));
        // At its own backoff -> eligible.
        assert!(arm_eligible(&WatchHealth::Degraded("x".to_owned()), Some(0), backoff, backoff));
        // A wider, already-doubled backoff is honored too, not clamped back
        // to the starting interval.
        let doubled = backoff * 2;
        assert!(!arm_eligible(&WatchHealth::Degraded("x".to_owned()), Some(0), doubled, backoff));
        assert!(arm_eligible(&WatchHealth::Degraded("x".to_owned()), Some(0), doubled, doubled));
    }

    #[test]
    fn reconcile_armed_is_never_arm_eligible() {
        assert!(!arm_eligible(&WatchHealth::Armed, None, DEGRADED_BACKOFF_START_MS, u64::MAX));
    }

    #[test]
    fn degraded_backoff_doubles_only_on_consecutive_degraded_and_resets_otherwise() {
        // First degrade (coming from Unarmed) starts at the base interval,
        // not a doubled one.
        let mut backoff = next_degraded_backoff_ms(false, true, DEGRADED_BACKOFF_START_MS);
        assert_eq!(backoff, DEGRADED_BACKOFF_START_MS);
        // Each consecutive Degraded outcome doubles it.
        backoff = next_degraded_backoff_ms(true, true, backoff);
        assert_eq!(backoff, DEGRADED_BACKOFF_START_MS * 2);
        backoff = next_degraded_backoff_ms(true, true, backoff);
        assert_eq!(backoff, DEGRADED_BACKOFF_START_MS * 4);
        backoff = next_degraded_backoff_ms(true, true, backoff);
        assert_eq!(backoff, DEGRADED_BACKOFF_START_MS * 8);
        // ...capped at 15 minutes, not left to grow unbounded.
        backoff = next_degraded_backoff_ms(true, true, backoff);
        assert_eq!(backoff, DEGRADED_BACKOFF_CAP_MS);
        backoff = next_degraded_backoff_ms(true, true, backoff);
        assert_eq!(backoff, DEGRADED_BACKOFF_CAP_MS, "must stay capped, not overflow past it");
        // A successful arm (health no longer Degraded) resets it, so the
        // next time this repo degrades it starts a fresh cycle.
        assert_eq!(next_degraded_backoff_ms(true, false, backoff), DEGRADED_BACKOFF_START_MS);
    }

    // --- reconcile: end-to-end decision table -----------------------------

    fn init_git_repo_fixture() -> PathBuf {
        let dir = temp_dir("reconcile-repo");
        fs::create_dir_all(&dir).expect("create fixture worktree");
        let status = std::process::Command::new("git")
            .args(["init", "-q"])
            .current_dir(&dir)
            .status()
            .expect("spawn git init for the reconcile fixture");
        assert!(status.success(), "git init must succeed for the reconcile fixture");
        dir
    }

    #[test]
    fn reconcile_arms_present_available_unarmed_disarms_on_unavailability_and_drops_on_absence() {
        let dir = init_git_repo_fixture();
        let git_dir = dir.join(".git");
        let targets = WatchTargets {
            worktree: dir.clone(),
            git_dir: git_dir.clone(),
            common_dir: git_dir,
        };
        let key = crate::discovery::watch_key(&dir);
        let epoch_source: Arc<dyn EpochSource> =
            Arc::new(crate::git_state_cache::MutationEpochSource::default());
        let git_stats = Arc::new(GitSpawnStats::default());
        let registry = WatchRegistry::new(epoch_source, git_stats, WatchConfig::default());

        // present + Available + Unarmed => arm. No `#[tokio::test]` runtime
        // in this test, so `reconcile`'s arm dispatch runs inline (the same
        // fallback `WatchRegistry::new` itself uses), making this
        // deterministic without polling.
        registry.reconcile(&[(
            key.clone(),
            Some(targets.clone()),
            ws_dashboard_core::WorkRootAvailability::Available,
        )]);
        assert_eq!(
            registry.health_for(&key),
            WatchHealth::Armed,
            "a present, available, unarmed real git repo must arm on reconcile"
        );

        // present + not Available => disarm to Unarmed (bump both is
        // covered by `do_disarm`'s own unit coverage via `finish_arm`'s
        // shared post-arm/disarm rule doc comment - not re-asserted here).
        registry.reconcile(&[(
            key.clone(),
            Some(targets.clone()),
            ws_dashboard_core::WorkRootAvailability::Moved,
        )]);
        assert_eq!(
            registry.health_for(&key),
            WatchHealth::Unarmed,
            "present but unavailable must disarm to Unarmed"
        );

        // Bypass the flat 30s rate-limit guard directly (it has its own
        // dedicated tests above) so this test can immediately exercise
        // re-arming after availability returns.
        {
            let mut state = registry.inner.state.lock().expect("state lock");
            if let Some(repo) = state.repos.get_mut(&key) {
                repo.last_arm_attempt_ms = None;
            }
        }
        registry.reconcile(&[(
            key.clone(),
            Some(targets.clone()),
            ws_dashboard_core::WorkRootAvailability::Available,
        )]);
        assert_eq!(
            registry.health_for(&key),
            WatchHealth::Armed,
            "must re-arm once availability returns"
        );

        // absent from `entries` entirely => disarmed AND dropped from
        // tracking outright (not merely left Unarmed), per the ticket's
        // "drop epochs" wording - distinct from the not-Available branch,
        // which leaves the repo tracked as Unarmed.
        registry.reconcile(&[]);
        assert!(
            !registry.inner.state.lock().expect("state lock").repos.contains_key(&key),
            "a repo absent from reconcile's entries must be dropped entirely"
        );

        remove_temp(&dir);
    }

    // --- checkpoint 8: config knobs + diag snapshot ------------------------

    #[test]
    fn parse_watch_mode_accepts_case_insensitive_trimmed_values_and_rejects_the_rest() {
        for (raw, expected) in [
            ("off", Some(WatchMode::Off)),
            ("OFF", Some(WatchMode::Off)),
            ("  Off  ", Some(WatchMode::Off)),
            ("auto", Some(WatchMode::Auto)),
            ("Auto", Some(WatchMode::Auto)),
            ("force", Some(WatchMode::Force)),
            ("FORCE", Some(WatchMode::Force)),
            ("", None),
            ("offline", None),
            ("automatic", None),
        ] {
            assert_eq!(
                parse_watch_mode(raw),
                expected,
                "parse_watch_mode({raw:?}) must equal {expected:?}"
            );
        }
    }

    #[test]
    fn diag_snapshot_reports_health_epochs_and_registered_watch_count_for_an_armed_repo() {
        let dir = init_git_repo_fixture();
        let git_dir = dir.join(".git");
        let targets = WatchTargets {
            worktree: dir.clone(),
            git_dir: git_dir.clone(),
            common_dir: git_dir,
        };
        let key = crate::discovery::watch_key(&dir);
        let epoch_source: Arc<dyn EpochSource> =
            Arc::new(crate::git_state_cache::MutationEpochSource::default());
        let git_stats = Arc::new(GitSpawnStats::default());
        let registry = WatchRegistry::new(epoch_source, git_stats, WatchConfig::default());

        registry.arm_now(&key, &targets);
        assert_eq!(registry.health_for(&key), WatchHealth::Armed);

        let snapshot = registry.diag_snapshot();
        assert_eq!(snapshot.len(), 1, "exactly one tracked repo after one arm_now");
        let entry = &snapshot[0];
        assert_eq!(entry.key, key.as_str());
        assert_eq!(entry.health, WatchHealth::Armed);
        assert_eq!(
            entry.worktree_epoch, 1,
            "arming bumps both epochs once (finish_arm's shared post-arm rule)"
        );
        assert_eq!(entry.refs_epoch, 1);
        assert!(
            entry.registered_watches > 0,
            "an Armed repo must report at least one registered directory/target"
        );

        remove_temp(&dir);
    }
}
