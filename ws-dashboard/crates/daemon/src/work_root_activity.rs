use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::{SystemTime, UNIX_EPOCH};

use axum::extract::{Path as AxumPath, Query, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::{Deserialize, Serialize};
use ws_dashboard_core::{
    NamedAgentActivityView, NamedAgentCallActivityView, WorkRootActivitySummary,
    WorkRootActivityView, WorkRootId,
};

use crate::router::AppState;

/// Upper bound applied to the wsagent-reported backend-error string so an
/// oversized `current/state.json` error cannot bloat the projection response.
/// Daemon-emitted diagnostics are fixed short constants and need no bounding.
const MAX_BOUNDED_TEXT: usize = 280;

const DIAG_METADATA_MISSING: &str = "agent metadata missing";
const DIAG_METADATA_UNREADABLE: &str = "agent metadata unreadable";
const DIAG_STATUS_UNAVAILABLE: &str = "agent status unavailable";
const DIAG_STATUS_UNRECOGNIZED: &str = "agent status unrecognized";
const DIAG_CURRENT_CALL_UNREADABLE: &str = "current call state unreadable";

const STATUS_UNAVAILABLE: &str = "unavailable";
const MAX_RECENT_ACTIVITY_LIMIT: usize = 30;

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct WorkRootActivityError {
    error: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkRootActivityQuery {
    recent_limit: Option<usize>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct WorkRootActivityProjectionConfig {
    // HINT: The wsstate Go manager accepts `WS_CACHE_HOME` as its cache-home
    // override. The dashboard keeps that override daemon-side so tests can
    // point at fixture cache trees without making browser API identity depend
    // on cache paths.
    pub cache_home: Option<PathBuf>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct WorkRootActivityProjector {
    cache_home: Option<PathBuf>,
}

impl WorkRootActivityProjector {
    pub fn new(config: WorkRootActivityProjectionConfig) -> Self {
        Self {
            cache_home: config.cache_home,
        }
    }

    pub async fn project(
        &self,
        work_root_id: WorkRootId,
        root_path: &Path,
    ) -> WorkRootActivityView {
        self.project_with_recent_limit(work_root_id, root_path, None)
            .await
    }

    pub async fn project_with_recent_limit(
        &self,
        work_root_id: WorkRootId,
        root_path: &Path,
        recent_limit: Option<usize>,
    ) -> WorkRootActivityView {
        // CONTRACT: Phase 1 reads wsstate/wsagent agent records for this opened
        // workRoot through daemon-owned projection logic. Browser callers never
        // receive cache paths, host paths, session ids, pids, or stream paths.
        //
        // The wsstate Git layout discovery, synchronous cache scanning, and JSON
        // parsing all run on a blocking pool so they never stall an Axum async
        // worker thread.
        let cache_home = self.cache_home.clone();
        let root_path = root_path.to_path_buf();
        let recent_limit = normalize_recent_activity_limit(recent_limit);
        tokio::task::spawn_blocking(move || {
            project_blocking(
                work_root_id,
                &root_path,
                cache_home.as_deref(),
                recent_limit,
            )
        })
        .await
        .expect("workRoot activity projection task panicked")
    }
}

pub async fn work_root_activity(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
    Query(query): Query<WorkRootActivityQuery>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let Some(root_path) = state.opened_work_roots.resolve(&work_root_id) else {
        return activity_error(StatusCode::NOT_FOUND, "unknown workRoot");
    };

    Json(
        state
            .work_root_activity
            .project_with_recent_limit(work_root_id, &root_path, query.recent_limit)
            .await,
    )
    .into_response()
}

fn normalize_recent_activity_limit(limit: Option<usize>) -> Option<usize> {
    limit
        .filter(|limit| *limit > 0)
        .map(|limit| limit.min(MAX_RECENT_ACTIVITY_LIMIT))
}

fn activity_error(status: StatusCode, message: &str) -> Response {
    (
        status,
        Json(WorkRootActivityError {
            error: message.to_owned(),
        }),
    )
        .into_response()
}

/// Resolve the wsstate named-agent directory for an opened workRoot under an
/// explicit cache home (the `WS_CACHE_HOME` cache root).
///
/// This mirrors `agents-plugin-tool/internal/wsstate` layout derivation: the
/// canonical Git worktree root and common root select a worktree key, and
/// agents live at `<cacheHome>/proj/<worktreeKey>/agents`. Returns `None` when
/// the `git` binary is unavailable or the workRoot is not a resolvable
/// non-bare Git worktree; all of those collapse to the same "no wsstate
/// layout, no agents" empty projection. Exposed so daemon route tests can seed
/// fixture cache trees at the same location the projector reads.
pub fn resolve_work_root_agents_dir(cache_home: &Path, root_path: &Path) -> Option<PathBuf> {
    let identity = git_identity(root_path)?;
    let project_key = short_hash(&canonical_path_bytes(&identity.common_root));
    let worktree_key = if identity.worktree_root == identity.common_root {
        project_key
    } else {
        let worktree_id = short_hash(&canonical_path_bytes(&identity.worktree_root));
        format!("{project_key}@{worktree_id}")
    };
    Some(cache_home.join("proj").join(worktree_key).join("agents"))
}

fn project_blocking(
    work_root_id: WorkRootId,
    root_path: &Path,
    cache_home: Option<&Path>,
    recent_limit: Option<usize>,
) -> WorkRootActivityView {
    let agents = resolve_cache_root(cache_home)
        .and_then(|cache_root| resolve_work_root_agents_dir(&cache_root, root_path))
        .map(|agents_dir| scan_named_agents(&agents_dir, recent_limit))
        .unwrap_or_default();

    let summary = summarize(&agents);
    let degraded = agents.iter().any(|agent| !agent.diagnostics.is_empty());

    WorkRootActivityView {
        work_root_id,
        status: if degraded { "degraded" } else { "ok" }.to_owned(),
        summary,
        agents,
    }
}

fn summarize(agents: &[NamedAgentActivityView]) -> WorkRootActivitySummary {
    let mut summary = WorkRootActivitySummary {
        total: agents.len(),
        ..WorkRootActivitySummary::default()
    };
    for agent in agents {
        match agent.status.as_str() {
            "running" => summary.active += 1,
            "blocked" => summary.blocked += 1,
            "failed" => summary.failed += 1,
            STATUS_UNAVAILABLE => summary.unavailable += 1,
            _ => {}
        }
    }
    summary
}

/// Scan `<worktree>/agents` for `agents/*/agent.json` plus optional
/// `current/state.json`, mapping each directory into a bounded row.
fn scan_named_agents(
    agents_dir: &Path,
    recent_limit: Option<usize>,
) -> Vec<NamedAgentActivityView> {
    let Ok(entries) = std::fs::read_dir(agents_dir) else {
        // No agents directory yet (or unreadable): an empty, healthy
        // projection rather than a route failure.
        return Vec::new();
    };

    let mut agent_dirs = Vec::new();
    for entry in entries.flatten() {
        let Ok(file_type) = entry.file_type() else {
            continue;
        };
        if !file_type.is_dir() {
            continue;
        }
        let agent_key = entry.file_name().to_string_lossy().into_owned();
        if agent_key.is_empty() {
            continue;
        }
        let agent_dir = entry.path();
        agent_dirs.push(RecentAgentDir {
            modified_at: agent_record_modified_at(&agent_dir),
            agent_key,
            agent_dir,
        });
    }

    if let Some(limit) = recent_limit {
        // CONTRACT: hot-path refreshes can ask for only the recently changed
        // rows. Use portable filesystem modification times from the agent dir
        // plus key child files instead of platform-specific watchers here.
        agent_dirs.sort_by(|left, right| {
            right
                .modified_at
                .cmp(&left.modified_at)
                .then_with(|| left.agent_key.cmp(&right.agent_key))
        });
        agent_dirs.truncate(limit);
    }

    let mut rows = agent_dirs
        .into_iter()
        .map(|entry| named_agent_row(&entry.agent_dir, entry.agent_key))
        .collect::<Vec<_>>();
    rows.sort_by(|left, right| left.agent_id.cmp(&right.agent_id));
    rows
}

struct RecentAgentDir {
    agent_key: String,
    agent_dir: PathBuf,
    modified_at: SystemTime,
}

fn agent_record_modified_at(agent_dir: &Path) -> SystemTime {
    let mut latest = modified_at(agent_dir);
    for relative in [
        "agent.json",
        "output.md",
        "current/state.json",
        "current/runtime.jsonl",
        "current/stdout",
        "current/stderr",
    ] {
        let candidate = modified_at(&agent_dir.join(relative));
        if candidate > latest {
            latest = candidate;
        }
    }
    latest
}

fn modified_at(path: &Path) -> SystemTime {
    std::fs::metadata(path)
        .and_then(|metadata| metadata.modified())
        .unwrap_or(UNIX_EPOCH)
}

fn named_agent_row(agent_dir: &Path, agent_key: String) -> NamedAgentActivityView {
    let metadata = match std::fs::read(agent_dir.join("agent.json")) {
        Ok(raw) => match serde_json::from_slice::<AgentMetadata>(&raw) {
            Ok(metadata) => Ok(metadata),
            Err(_) => Err(DIAG_METADATA_UNREADABLE),
        },
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Err(DIAG_METADATA_MISSING),
        Err(_) => Err(DIAG_METADATA_UNREADABLE),
    };

    let metadata = match metadata {
        Ok(metadata) => metadata,
        Err(diagnostic) => {
            // CONTRACT: a malformed or missing record degrades only its own row
            // instead of failing the whole route.
            return NamedAgentActivityView {
                agent_id: agent_key,
                name: None,
                backend: None,
                harness: None,
                tier: None,
                model: None,
                effort: None,
                status: STATUS_UNAVAILABLE.to_owned(),
                last_call_at: None,
                session_present: false,
                current_call: None,
                detail_hints: Vec::new(),
                diagnostics: vec![diagnostic.to_owned()],
            };
        }
    };

    let mut diagnostics = Vec::new();
    let (status, status_diagnostic) = agent_status(&metadata.status);
    if let Some(diagnostic) = status_diagnostic {
        diagnostics.push(diagnostic.to_owned());
    }

    let (current_call, current_call_diagnostic) = read_current_call(agent_dir);
    if let Some(diagnostic) = current_call_diagnostic {
        diagnostics.push(diagnostic.to_owned());
    }

    let mut detail_hints = Vec::new();
    if !metadata.last_output_path.is_empty() {
        // CONTRACT: surface that output exists without leaking the cache path.
        detail_hints.push("recent output available".to_owned());
    }

    NamedAgentActivityView {
        agent_id: agent_key,
        name: non_empty(metadata.name),
        backend: non_empty(metadata.backend),
        harness: non_empty(metadata.harness),
        tier: non_empty(metadata.tier),
        model: non_empty(metadata.model),
        effort: non_empty(metadata.effort),
        status,
        last_call_at: non_empty(metadata.last_call_at),
        // CONTRACT: collapse the private session id into a presence flag.
        session_present: !metadata.session_id.is_empty(),
        current_call,
        detail_hints,
        diagnostics,
    }
}

fn read_current_call(
    agent_dir: &Path,
) -> (Option<NamedAgentCallActivityView>, Option<&'static str>) {
    let raw = match std::fs::read(agent_dir.join("current").join("state.json")) {
        Ok(raw) => raw,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return (None, None),
        Err(_) => return (None, Some(DIAG_CURRENT_CALL_UNREADABLE)),
    };

    let Ok(state) = serde_json::from_slice::<CurrentCallState>(&raw) else {
        return (None, Some(DIAG_CURRENT_CALL_UNREADABLE));
    };

    let active = matches!(state.status.as_str(), "queued" | "running");
    let terminal = matches!(state.status.as_str(), "completed" | "failed" | "cancelled");

    (
        Some(NamedAgentCallActivityView {
            status: state.status,
            active,
            terminal,
            execution_id: non_empty(state.execution_id),
            started_at: non_empty(state.started_at),
            updated_at: non_empty(state.updated_at),
            finished_at: non_empty(state.finished_at),
            cleanup_needed: state.cleanup_needed,
            error: non_empty(state.error).map(|error| bounded(&error)),
        }),
        None,
    )
}

/// Map a wsagent agent status into a dashboard status plus an optional bounded
/// diagnostic for empty or unrecognized values.
fn agent_status(raw: &str) -> (String, Option<&'static str>) {
    match raw {
        "idle" | "running" | "blocked" | "failed" | "erased" => (raw.to_owned(), None),
        "" => (STATUS_UNAVAILABLE.to_owned(), Some(DIAG_STATUS_UNAVAILABLE)),
        _ => (
            STATUS_UNAVAILABLE.to_owned(),
            Some(DIAG_STATUS_UNRECOGNIZED),
        ),
    }
}

fn non_empty(value: String) -> Option<String> {
    (!value.is_empty()).then_some(value)
}

fn bounded(value: &str) -> String {
    if value.chars().count() <= MAX_BOUNDED_TEXT {
        return value.to_owned();
    }
    value.chars().take(MAX_BOUNDED_TEXT).collect()
}

/// Resolve the wsstate cache root, mirroring `wsstate.CacheRoot`: an explicit
/// daemon override, then `WS_CACHE_HOME`, then `~/.cache/ws@kang-sw-devenv`.
fn resolve_cache_root(cache_home: Option<&Path>) -> Option<PathBuf> {
    if let Some(cache_home) = cache_home {
        return Some(cache_home.to_path_buf());
    }
    if let Some(env) = std::env::var_os("WS_CACHE_HOME") {
        if !env.is_empty() {
            return Some(PathBuf::from(env));
        }
    }
    Some(home_dir()?.join(".cache").join("ws@kang-sw-devenv"))
}

fn home_dir() -> Option<PathBuf> {
    #[cfg(windows)]
    let key = "USERPROFILE";
    #[cfg(not(windows))]
    let key = "HOME";
    std::env::var_os(key)
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
}

struct GitIdentity {
    worktree_root: PathBuf,
    common_root: PathBuf,
}

/// Discover the canonical Git worktree root and common root for `root_path`,
/// matching `wsstate.gitIdentity`. Returns `None` when the `git` binary is
/// unavailable, the path is not in a Git repository, or the repository is bare.
fn git_identity(root_path: &Path) -> Option<GitIdentity> {
    let toplevel = git_output(root_path, &["rev-parse", "--show-toplevel"])?;
    let common_git_dir = git_output(
        root_path,
        &["rev-parse", "--path-format=absolute", "--git-common-dir"],
    )?;

    let worktree_root = std::fs::canonicalize(&toplevel).ok()?;
    let common_git_dir = std::fs::canonicalize(&common_git_dir).ok()?;
    // wsstate only supports non-bare repositories: the common dir must be a
    // `.git` directory whose parent is the common root.
    if common_git_dir.file_name().and_then(|name| name.to_str()) != Some(".git") {
        return None;
    }
    let common_root = std::fs::canonicalize(common_git_dir.parent()?).ok()?;

    Some(GitIdentity {
        worktree_root,
        common_root,
    })
}

fn git_output(repo: &Path, args: &[&str]) -> Option<String> {
    let output = Command::new("git")
        .arg("-C")
        .arg(repo)
        .args(args)
        .output()
        .ok()?;
    if !output.status.success() {
        return None;
    }
    let value = String::from_utf8(output.stdout).ok()?;
    let trimmed = value.trim();
    (!trimmed.is_empty()).then(|| trimmed.to_owned())
}

/// Byte representation of a canonical path used as the SHA-256 hashing input
/// for wsstate project/worktree keys.
///
/// `wsstate.shortHash` hashes the raw bytes of a `filepath`-cleaned path. To
/// stay key-compatible this:
/// - strips the Windows `\\?\` / `\\?\UNC\` verbatim prefix that
///   `std::fs::canonicalize` adds (Go's `filepath` produces no such prefix);
/// - on Unix hashes the raw `OsStr` bytes rather than a lossy UTF-8 string, so
///   a non-UTF-8 path still derives the same key as the Go tool.
fn canonical_path_bytes(path: &Path) -> Vec<u8> {
    #[cfg(windows)]
    {
        let raw = path.to_string_lossy();
        let normalized = if let Some(rest) = raw.strip_prefix(r"\\?\UNC\") {
            format!(r"\\{rest}")
        } else if let Some(rest) = raw.strip_prefix(r"\\?\") {
            rest.to_owned()
        } else {
            raw.into_owned()
        };
        normalized.into_bytes()
    }
    #[cfg(not(windows))]
    {
        use std::os::unix::ffi::OsStrExt;
        path.as_os_str().as_bytes().to_vec()
    }
}

/// First eight lowercase hex characters of the SHA-256 digest of `value`,
/// matching `wsstate.shortHash` used for project and worktree keys.
fn short_hash(value: &[u8]) -> String {
    let digest = sha256(value);
    let mut hex = String::with_capacity(8);
    for byte in &digest[..4] {
        hex.push_str(&format!("{byte:02x}"));
    }
    hex
}

/// Minimal SHA-256 (FIPS 180-4) over `data`.
///
/// Implemented locally to keep wsstate-compatible path-key derivation without
/// adding a hashing crate; the value is a cache-layout key, not a security
/// primitive.
fn sha256(data: &[u8]) -> [u8; 32] {
    const K: [u32; 64] = [
        0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4,
        0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe,
        0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f,
        0x4a7484aa, 0x5cb0a9dc, 0x76f988da, 0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7,
        0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc,
        0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b,
        0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070, 0x19a4c116,
        0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
        0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7,
        0xc67178f2,
    ];

    let mut h: [u32; 8] = [
        0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab,
        0x5be0cd19,
    ];

    let bit_len = (data.len() as u64).wrapping_mul(8);
    let mut message = data.to_vec();
    message.push(0x80);
    while message.len() % 64 != 56 {
        message.push(0);
    }
    message.extend_from_slice(&bit_len.to_be_bytes());

    for block in message.chunks_exact(64) {
        let mut w = [0u32; 64];
        for (index, word) in w.iter_mut().take(16).enumerate() {
            let offset = index * 4;
            *word = u32::from_be_bytes([
                block[offset],
                block[offset + 1],
                block[offset + 2],
                block[offset + 3],
            ]);
        }
        for index in 16..64 {
            let s0 = w[index - 15].rotate_right(7)
                ^ w[index - 15].rotate_right(18)
                ^ (w[index - 15] >> 3);
            let s1 = w[index - 2].rotate_right(17)
                ^ w[index - 2].rotate_right(19)
                ^ (w[index - 2] >> 10);
            w[index] = w[index - 16]
                .wrapping_add(s0)
                .wrapping_add(w[index - 7])
                .wrapping_add(s1);
        }

        let [mut a, mut b, mut c, mut d, mut e, mut f, mut g, mut hh] = h;
        for index in 0..64 {
            let s1 = e.rotate_right(6) ^ e.rotate_right(11) ^ e.rotate_right(25);
            let ch = (e & f) ^ ((!e) & g);
            let t1 = hh
                .wrapping_add(s1)
                .wrapping_add(ch)
                .wrapping_add(K[index])
                .wrapping_add(w[index]);
            let s0 = a.rotate_right(2) ^ a.rotate_right(13) ^ a.rotate_right(22);
            let maj = (a & b) ^ (a & c) ^ (b & c);
            let t2 = s0.wrapping_add(maj);

            hh = g;
            g = f;
            f = e;
            e = d.wrapping_add(t1);
            d = c;
            c = b;
            b = a;
            a = t1.wrapping_add(t2);
        }

        h[0] = h[0].wrapping_add(a);
        h[1] = h[1].wrapping_add(b);
        h[2] = h[2].wrapping_add(c);
        h[3] = h[3].wrapping_add(d);
        h[4] = h[4].wrapping_add(e);
        h[5] = h[5].wrapping_add(f);
        h[6] = h[6].wrapping_add(g);
        h[7] = h[7].wrapping_add(hh);
    }

    let mut digest = [0u8; 32];
    for (index, word) in h.iter().enumerate() {
        digest[index * 4..index * 4 + 4].copy_from_slice(&word.to_be_bytes());
    }
    digest
}

/// Subset of `wsagent` `agent.json` the projection needs. Private fields such
/// as `pid`, stream paths, and the raw session id are intentionally not
/// deserialized so they cannot reach the browser response.
#[derive(Debug, Default, Deserialize)]
struct AgentMetadata {
    #[serde(default)]
    name: String,
    #[serde(default)]
    backend: String,
    #[serde(default)]
    harness: String,
    #[serde(default)]
    tier: String,
    #[serde(default)]
    model: String,
    #[serde(default)]
    effort: String,
    #[serde(default)]
    session_id: String,
    #[serde(default)]
    status: String,
    #[serde(default)]
    last_call_at: String,
    #[serde(default)]
    last_output_path: String,
}

/// Subset of `wsagent` `current/state.json` the projection needs. PID, stream
/// paths, and the session id are intentionally omitted.
#[derive(Debug, Default, Deserialize)]
struct CurrentCallState {
    #[serde(default)]
    status: String,
    #[serde(default)]
    execution_id: String,
    #[serde(default)]
    started_at: String,
    #[serde(default)]
    updated_at: String,
    #[serde(default)]
    finished_at: String,
    #[serde(default)]
    error: String,
    #[serde(default)]
    cleanup_needed: bool,
}

#[cfg(test)]
mod tests {
    use super::*;

    fn hex(bytes: &[u8]) -> String {
        bytes.iter().map(|byte| format!("{byte:02x}")).collect()
    }

    #[test]
    fn sha256_matches_known_answer_vectors() {
        // FIPS 180-4 / NIST known-answer vectors. The 56-byte and one-million
        // byte inputs both span multiple 64-byte blocks, exercising the
        // `chunks_exact(64)` loop and the full 32-byte digest, not just the
        // single-block path or the leading bytes used by `short_hash`.
        assert_eq!(
            hex(&sha256(b"abc")),
            "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
        );
        assert_eq!(
            hex(&sha256(b"")),
            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
        );
        // 56 bytes -> two blocks after padding.
        assert_eq!(
            hex(&sha256(
                b"abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"
            )),
            "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1"
        );
        // One million 'a' bytes -> many blocks.
        let million_a = vec![b'a'; 1_000_000];
        assert_eq!(
            hex(&sha256(&million_a)),
            "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0"
        );
    }

    #[test]
    fn short_hash_matches_known_sha256_prefix() {
        // SHA-256("abc") = ba7816bf8f01cfea...; wsstate keys take the first
        // eight hex characters.
        assert_eq!(short_hash(b"abc"), "ba7816bf");
        // SHA-256("") = e3b0c44298fc1c14...
        assert_eq!(short_hash(b""), "e3b0c442");
    }

    #[cfg(not(windows))]
    #[test]
    fn canonical_path_bytes_uses_raw_unix_bytes() {
        // On Unix the hashing input is the exact path bytes, no verbatim
        // prefix and no lossy UTF-8 substitution.
        assert_eq!(
            canonical_path_bytes(Path::new("/tmp/ws-root")),
            b"/tmp/ws-root".to_vec()
        );
    }

    #[cfg(windows)]
    #[test]
    fn canonical_path_bytes_strips_windows_verbatim_prefix() {
        // `std::fs::canonicalize` emits `\\?\` verbatim paths; wsstate's
        // `filepath`-based path has no such prefix, so it must be stripped
        // before hashing or Windows keys diverge from the Go tool.
        assert_eq!(
            canonical_path_bytes(Path::new(r"\\?\C:\repo")),
            br"C:\repo".to_vec()
        );
        assert_eq!(
            canonical_path_bytes(Path::new(r"\\?\UNC\server\share")),
            br"\\server\share".to_vec()
        );
    }

    #[test]
    fn agent_status_maps_known_and_degraded_values() {
        assert_eq!(agent_status("idle"), ("idle".to_owned(), None));
        assert_eq!(agent_status("running"), ("running".to_owned(), None));
        assert_eq!(agent_status("blocked"), ("blocked".to_owned(), None));
        assert_eq!(agent_status("failed"), ("failed".to_owned(), None));
        assert_eq!(agent_status("erased"), ("erased".to_owned(), None));
        assert_eq!(
            agent_status(""),
            (STATUS_UNAVAILABLE.to_owned(), Some(DIAG_STATUS_UNAVAILABLE))
        );
        assert_eq!(
            agent_status("mystery"),
            (
                STATUS_UNAVAILABLE.to_owned(),
                Some(DIAG_STATUS_UNRECOGNIZED)
            )
        );
    }

    #[test]
    fn bounded_truncates_to_the_text_limit() {
        let long = "x".repeat(MAX_BOUNDED_TEXT + 50);
        assert_eq!(bounded(&long).chars().count(), MAX_BOUNDED_TEXT);
        assert_eq!(bounded("short"), "short");
    }

    #[test]
    fn resolve_cache_root_prefers_explicit_override_then_env() {
        // Explicit daemon override wins and is returned verbatim.
        let override_root = PathBuf::from("/fixture/cache-home");
        assert_eq!(
            resolve_cache_root(Some(override_root.as_path())),
            Some(override_root.clone())
        );

        // With no override, `WS_CACHE_HOME` is honored. No other test reads
        // this variable, so the set/clear stays self-contained.
        let previous = std::env::var_os("WS_CACHE_HOME");
        std::env::set_var("WS_CACHE_HOME", "/fixture/env-cache");
        assert_eq!(
            resolve_cache_root(None),
            Some(PathBuf::from("/fixture/env-cache"))
        );
        match previous {
            Some(value) => std::env::set_var("WS_CACHE_HOME", value),
            None => std::env::remove_var("WS_CACHE_HOME"),
        }
    }
}
