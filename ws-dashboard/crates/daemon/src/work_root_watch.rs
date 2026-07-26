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
use std::path::{Path, PathBuf};

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
}
