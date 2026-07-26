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

// NOTE: this module is built up checkpoint-by-checkpoint (Lead Disposition
// D6); pieces land here before every call site is wired in later checkpoints.
// `dead_code` is allowed for the duration of that build-out and removed once
// the module is fully wired into `resources.rs`/`git_toolbar.rs`/`server.rs`
// (the final checkpoint), so the crate-wide clippy warning count does not
// grow permanently.
#![allow(dead_code)]

use std::ffi::OsStr;
use std::fs;
use std::path::{Path, PathBuf};

use crate::git_exec::{capture, git_timeout_from_env, GitFailureExpectation, GitSpawnStats};

/// Which cache axis an observed filesystem event invalidates. Mirrors
/// `git_state_cache`'s two independently revalidated slots.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum EpochKind {
    Worktree,
    Refs,
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
}
