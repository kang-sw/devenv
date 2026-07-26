use std::collections::{BTreeMap, HashMap};
use std::env;
use std::fmt;
use std::fs;
use std::io;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use ws_dashboard_core::{
    ActionHint, DashboardResourcesView, InstanceKind, InstanceRole, InstanceView, InteractionMode,
    OpaqueId, ResourcePath, ServerView, ViewState, WorkRootActivation, WorkRootAvailability,
    WorkRootId, WorkRootKind, WorkRootStatus, WorkRootView, WorkspaceView,
};

use crate::git_exec::{
    capture, git_timeout_from_env, GitFailureExpectation, GitSpawnStats,
};
use crate::resources::DashboardResourcesProvider;
use crate::work_root_files::{RegisteredWorkRoot, WorkRootProvenance};

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct LocalWorkRootCandidate {
    path: PathBuf,
    activation: WorkRootActivation,
}

impl LocalWorkRootCandidate {
    pub fn new(path: impl Into<PathBuf>) -> Self {
        Self {
            path: path.into(),
            activation: WorkRootActivation::Online,
        }
    }

    pub fn with_activation(path: impl Into<PathBuf>, activation: WorkRootActivation) -> Self {
        Self {
            path: path.into(),
            activation,
        }
    }
}

#[derive(Clone, Debug)]
pub struct LocalDashboardResourcesProvider {
    server_id: OpaqueId,
    server_label: String,
    candidates: Vec<LocalWorkRootCandidate>,
    registry_activations: HashMap<WorkRootId, WorkRootActivation>,
    git_probes: GitProbeCache,
    /// Shared spawn counters (the daemon's `AppState` counters when set via
    /// `with_git_spawn_stats`, otherwise a private throwaway instance). `Arc`
    /// so this cheap-to-clone builder-style struct can hold it without a
    /// lifetime parameter, mirroring `git_probes`.
    git_stats: Arc<GitSpawnStats>,
}

impl LocalDashboardResourcesProvider {
    pub fn new(candidates: Vec<LocalWorkRootCandidate>) -> Self {
        Self::with_registry_activations(candidates, HashMap::new())
    }

    pub fn with_registry_activations(
        candidates: Vec<LocalWorkRootCandidate>,
        registry_activations: HashMap<WorkRootId, WorkRootActivation>,
    ) -> Self {
        Self {
            server_id: OpaqueId::from("server-local"),
            server_label: "Local ws dashboard".to_owned(),
            candidates,
            registry_activations,
            git_probes: GitProbeCache::default(),
            git_stats: Arc::new(GitSpawnStats::default()),
        }
    }

    /// Share a caller-owned probe memo (the daemon's `AppState` cache) instead
    /// of this provider's own private one, so the routes that all run full
    /// discovery collapse onto a single set of `git` probes.
    pub fn with_git_probe_cache(mut self, git_probes: GitProbeCache) -> Self {
        self.git_probes = git_probes;
        self
    }

    /// Share a caller-owned spawn-counter handle (the daemon's `AppState`
    /// counters) instead of this provider's own private, discarded instance.
    pub fn with_git_spawn_stats(mut self, git_stats: Arc<GitSpawnStats>) -> Self {
        self.git_stats = git_stats;
        self
    }

    pub fn dashboard_resources_with_registry_sync(&self) -> DashboardResourcesSync {
        let mut workspaces = BTreeMap::<WorkspaceKey, WorkspaceBuilder>::new();
        let mut discovered_registry_roots = Vec::new();
        let mut pruned_work_root_ids = Vec::new();

        for candidate in &self.candidates {
            let owner = discover_work_root(&candidate.path, &self.git_probes, &self.git_stats);
            let workspace_key = owner.workspace_key.clone();
            let linked_paths = if owner.availability == WorkRootAvailability::Available {
                self.git_probes.worktree_paths(&candidate.path, &self.git_stats)
            } else {
                Vec::new()
            };
            let workspace = workspaces
                .entry(workspace_key.clone())
                .or_insert_with(|| WorkspaceBuilder::new(&self.server_id, workspace_key.clone()));
            workspace.push(owner, candidate.activation, true);

            for linked_path in linked_paths {
                let linked_id = local_work_root_id_for_path(&linked_path);
                if linked_id == local_work_root_id_for_path(&candidate.path)
                    || paths_equivalent(&linked_path, &candidate.path)
                {
                    continue;
                }
                let mut linked =
                    discover_work_root(&linked_path, &self.git_probes, &self.git_stats);
                linked.workspace_key = workspace_key.clone();
                let linked_activation = self
                    .registry_activations
                    .get(&linked_id)
                    .copied()
                    .unwrap_or(WorkRootActivation::Online);
                if linked.availability == WorkRootAvailability::Available {
                    discovered_registry_roots.push(RegisteredWorkRoot {
                        path: linked_path,
                        activation: linked_activation,
                        provenance: WorkRootProvenance::Discovered,
                    });
                }
                workspace.push(linked, linked_activation, false);
            }
        }

        let mut workspace_views = Vec::new();
        for workspace in workspaces.into_values() {
            if workspace.active_work_root_count == 0 {
                pruned_work_root_ids
                    .extend(workspace.work_roots.iter().map(|root| root.id.clone()));
                continue;
            }
            workspace_views.push(workspace.into_view());
        }

        DashboardResourcesSync {
            view: DashboardResourcesView {
                server: self.server_view(),
                workspaces: workspace_views,
            },
            discovered_registry_roots,
            pruned_work_root_ids,
        }
    }

    fn server_view(&self) -> ServerView {
        ServerView {
            id: self.server_id.clone(),
            label: self.server_label.clone(),
            state: ViewState {
                status: "online".to_owned(),
                loading: false,
                stale: false,
                error: None,
            },
            actions: vec![ActionHint {
                id: "refresh".to_owned(),
                label: "Refresh".to_owned(),
                enabled: true,
            }],
        }
    }
}

impl DashboardResourcesProvider for LocalDashboardResourcesProvider {
    fn dashboard_resources(&self) -> DashboardResourcesView {
        self.dashboard_resources_with_registry_sync().view
    }
}

pub struct DashboardResourcesSync {
    pub view: DashboardResourcesView,
    pub discovered_registry_roots: Vec<RegisteredWorkRoot>,
    pub pruned_work_root_ids: Vec<WorkRootId>,
}

#[derive(Clone, Debug, Eq, Ord, PartialEq, PartialOrd)]
struct WorkspaceKey {
    id: OpaqueId,
    label: String,
}

struct WorkspaceBuilder {
    id: OpaqueId,
    label: String,
    server_id: OpaqueId,
    work_roots: Vec<WorkRootView>,
    active_work_root_count: usize,
    root_unavailable_with_active_child: bool,
}

impl WorkspaceBuilder {
    fn new(server_id: &OpaqueId, key: WorkspaceKey) -> Self {
        Self {
            id: key.id,
            label: key.label,
            server_id: server_id.clone(),
            work_roots: Vec::new(),
            active_work_root_count: 0,
            root_unavailable_with_active_child: false,
        }
    }

    fn push(
        &mut self,
        discovered: DiscoveredWorkRoot,
        activation: WorkRootActivation,
        root_anchor: bool,
    ) {
        let work_root_id = local_work_root_id_for_path(&discovered.path);
        if self.work_roots.iter().any(|root| root.id == work_root_id) {
            return;
        }
        let available = discovered.availability == WorkRootAvailability::Available;
        let active = activation == WorkRootActivation::Online;
        let enabled = available && active;
        if available {
            self.active_work_root_count += 1;
        }
        if root_anchor && !available {
            self.root_unavailable_with_active_child = true;
        }

        let resource_path = ResourcePath {
            server_id: self.server_id.clone(),
            workspace_id: self.id.clone(),
            work_root_id: work_root_id.clone(),
            instance_id: None,
        };
        let main_instances = if enabled && env::var_os("WS_DASHBOARD_E2E_AGENT_FIXTURE").is_some() {
            vec![e2e_agent_fixture_instance(&resource_path)]
        } else {
            Vec::new()
        };

        self.work_roots.push(WorkRootView {
            id: work_root_id,
            resource_path,
            label: label_for_path(&discovered.path),
            kind: discovered.kind,
            activation,
            availability: discovered.availability,
            status: discovered.status,
            state: ViewState {
                status: state_status(discovered.availability, activation).to_owned(),
                loading: false,
                stale: !enabled,
                error: if active { discovered.error } else { None },
            },
            compactable: false,
            main_instances,
            actions: work_root_actions(discovered.kind, active, available),
        });
    }

    fn into_view(self) -> WorkspaceView {
        let degraded = self
            .work_roots
            .iter()
            .any(|root| root.status != WorkRootStatus::Online);
        let recovery_needed =
            self.root_unavailable_with_active_child && self.active_work_root_count > 0;

        WorkspaceView {
            id: self.id,
            label: self.label,
            state: ViewState {
                status: if recovery_needed {
                    "recoveryNeeded"
                } else if degraded {
                    "degraded"
                } else {
                    "ready"
                }
                .to_owned(),
                loading: false,
                stale: degraded || recovery_needed,
                error: if recovery_needed {
                    Some("workspace root workRoot unavailable".to_owned())
                } else {
                    degraded.then(|| "one or more workRoots need refresh".to_owned())
                },
            },
            compactable: self.work_roots.len() == 1,
            work_roots: self.work_roots,
            actions: vec![
                ActionHint {
                    id: "refreshWorkspace".to_owned(),
                    label: "Refresh workspace".to_owned(),
                    enabled: true,
                },
                ActionHint {
                    id: "workspace.remove".to_owned(),
                    label: "Remove workspace".to_owned(),
                    enabled: true,
                },
            ],
        }
    }
}

fn e2e_agent_fixture_instance(resource_path: &ResourcePath) -> InstanceView {
    let instance_id = OpaqueId::from(format!(
        "instance-e2e-agent-{}",
        resource_path.work_root_id.as_str()
    ));
    InstanceView {
        id: instance_id.clone(),
        resource_path: ResourcePath {
            server_id: resource_path.server_id.clone(),
            workspace_id: resource_path.workspace_id.clone(),
            work_root_id: resource_path.work_root_id.clone(),
            instance_id: Some(instance_id),
        },
        role: InstanceRole::Main,
        kind: InstanceKind::Agent,
        interaction_mode: InteractionMode::Direct,
        label: "E2E agent".to_owned(),
        state: ViewState {
            status: "ready".to_owned(),
            loading: false,
            stale: false,
            error: None,
        },
        sub_instances: Vec::new(),
        actions: Vec::new(),
    }
}

struct DiscoveredWorkRoot {
    path: PathBuf,
    workspace_key: WorkspaceKey,
    kind: WorkRootKind,
    status: WorkRootStatus,
    availability: WorkRootAvailability,
    error: Option<String>,
}

fn discover_work_root(
    path: &Path,
    git_probes: &GitProbeCache,
    git_stats: &GitSpawnStats,
) -> DiscoveredWorkRoot {
    let normalized = normalize_candidate_path(path);

    // CONTRACT: availability (`moved` / `missing` / `inaccessible`) is decided
    // by the uncached `fs::metadata` below and `discover_existing_dir`'s
    // uncached `fs::read_dir`. Only the `git` subprocess probes are memoized,
    // so a workRoot whose directory disappears is still reported on the very
    // next request.
    let discovered = match fs::metadata(&normalized) {
        Ok(metadata) if metadata.is_dir() => {
            discover_existing_dir(normalized, git_probes, git_stats)
        }
        Ok(_) => discovered_unusable(
            normalized,
            WorkRootStatus::Inaccessible,
            WorkRootAvailability::Inaccessible,
            "workRoot is not a directory",
        ),
        Err(error) if error.kind() == io::ErrorKind::NotFound => {
            let (status, availability) = match normalized.parent() {
                Some(parent) if parent.exists() => {
                    (WorkRootStatus::Moved, WorkRootAvailability::Moved)
                }
                _ => (WorkRootStatus::Offline, WorkRootAvailability::Missing),
            };
            discovered_unusable(
                normalized,
                status,
                availability,
                availability_status(availability),
            )
        }
        Err(error) if error.kind() == io::ErrorKind::PermissionDenied => discovered_unusable(
            normalized,
            WorkRootStatus::Inaccessible,
            WorkRootAvailability::Inaccessible,
            "permission denied",
        ),
        Err(error) => discovered_unusable(
            normalized,
            WorkRootStatus::Inaccessible,
            WorkRootAvailability::Inaccessible,
            &format!("metadata failed: {error}"),
        ),
    };
    if discovered.availability != WorkRootAvailability::Available {
        // Drop any memo this root left behind so a root that reappears
        // re-probes `git` immediately instead of after the TTL.
        git_probes.evict(&discovered.path);
    }
    discovered
}

fn discover_existing_dir(
    path: PathBuf,
    git_probes: &GitProbeCache,
    git_stats: &GitSpawnStats,
) -> DiscoveredWorkRoot {
    if let Err(error) = fs::read_dir(&path) {
        return discovered_unusable(
            path,
            WorkRootStatus::Inaccessible,
            WorkRootAvailability::Inaccessible,
            if error.kind() == io::ErrorKind::PermissionDenied {
                "permission denied"
            } else {
                "directory cannot be read"
            },
        );
    }

    match git_probes.discover(&path, git_stats) {
        Some(git) => DiscoveredWorkRoot {
            workspace_key: WorkspaceKey {
                id: OpaqueId::from(format!(
                    "workspace-local-{}",
                    stable_path_hash(&canonical_or_normalized(&git.common_dir))
                )),
                label: git.workspace_label(),
            },
            path,
            kind: git.kind,
            status: WorkRootStatus::Online,
            availability: WorkRootAvailability::Available,
            error: None,
        },
        None => DiscoveredWorkRoot {
            workspace_key: WorkspaceKey {
                id: OpaqueId::from(format!(
                    "workspace-local-{}",
                    stable_path_hash(&canonical_or_normalized(&path))
                )),
                label: label_for_path(&path),
            },
            path,
            kind: WorkRootKind::PlainDirectory,
            status: WorkRootStatus::Online,
            availability: WorkRootAvailability::Available,
            error: None,
        },
    }
}

fn discovered_unusable(
    path: PathBuf,
    status: WorkRootStatus,
    availability: WorkRootAvailability,
    error: &str,
) -> DiscoveredWorkRoot {
    DiscoveredWorkRoot {
        workspace_key: WorkspaceKey {
            id: OpaqueId::from(format!(
                "workspace-local-{}",
                stable_path_hash(&canonical_or_normalized(&path))
            )),
            label: label_for_path(&path),
        },
        path,
        kind: WorkRootKind::PlainDirectory,
        status,
        availability,
        error: Some(error.to_owned()),
    }
}

// ---------------------------------------------------------------------------
// Git probe memoization
//
// Every route that only needs ONE workRoot's git context (git toolbar status
// and branches, worktree add/remove, the canonical resources refresh) resolves
// it by running full discovery over ALL registered roots, so the two `git`
// probes below ran `2N + W` times per call, from several routes, several times
// per second at idle - measured at ~9.6 `git` spawns/second on a dogfood
// daemon, almost all of it re-answering identical questions.
//
// A short TTL memo with per-key single-flight collapses that to one spawn per
// root per TTL. Only the subprocess probes are memoized: availability
// detection stays on the uncached filesystem path (see `discover_work_root`),
// and the registry side effects in `live_dashboard_resources_with_sync` still
// run on every call.
// ---------------------------------------------------------------------------

const DEFAULT_GIT_PROBE_TTL_MS: u64 = 30_000;

/// Memo key for the `git` probes.
///
/// Deliberately NOT `local_work_root_id_for_path`: `WorkRootId` values are
/// persisted and keyed by the frontend, so their derivation must not churn,
/// while this key must aggressively collapse spellings of the same directory.
/// The work-root registry observably holds mixed-separator paths (e.g.
/// `D:/repo/.git\ws-worktree\jpeg`), and an un-normalized key would miss the
/// memo for every such root and keep spawning `git`.
#[derive(Clone, Debug, Eq, Hash, PartialEq)]
struct GitProbeKey(String);

impl GitProbeKey {
    fn for_path(path: &Path) -> Self {
        // The `\` -> `/` unification is unconditional: a Unix filename may
        // legally contain a backslash, but this is only a memo key, and a
        // Windows-authored registry entry for the same directory can reach a
        // Unix build of this code through a linked-server registry file.
        let mut key = canonical_or_normalized(path)
            .to_string_lossy()
            .replace('\\', "/");
        if cfg!(windows) {
            key = key.to_lowercase();
        }
        Self(key)
    }
}

struct CachedProbe<T> {
    probed_at: Instant,
    value: T,
}

/// TTL memo with per-key single-flight for one probe.
struct ProbeSlots<T> {
    slots: Mutex<HashMap<GitProbeKey, Arc<Mutex<Option<CachedProbe<T>>>>>>,
}

impl<T> Default for ProbeSlots<T> {
    fn default() -> Self {
        Self {
            slots: Mutex::new(HashMap::new()),
        }
    }
}

impl<T: Clone> ProbeSlots<T> {
    fn get_or_probe(&self, key: &GitProbeKey, ttl: Duration, probe: impl FnOnce() -> T) -> T {
        // Two-level lock: the map lock is released before the per-key lock is
        // acquired, so the map is never held across a `git` spawn, while the
        // per-key lock makes concurrent misses for the same key single-flight
        // - three routes missing together produce one spawn, not three.
        let slot = {
            let mut slots = self.slots.lock().expect("git probe slot map lock poisoned");
            slots.entry(key.clone()).or_default().clone()
        };
        let mut slot = slot.lock().expect("git probe slot lock poisoned");
        if let Some(cached) = slot.as_ref() {
            if cached.probed_at.elapsed() < ttl {
                return cached.value.clone();
            }
        }
        let value = probe();
        *slot = Some(CachedProbe {
            probed_at: Instant::now(),
            value: value.clone(),
        });
        value
    }

    fn evict(&self, key: &GitProbeKey) {
        self.slots
            .lock()
            .expect("git probe slot map lock poisoned")
            .remove(key);
    }

    #[cfg(test)]
    fn len(&self) -> usize {
        self.slots
            .lock()
            .expect("git probe slot map lock poisoned")
            .len()
    }

    fn clear(&self) {
        self.slots
            .lock()
            .expect("git probe slot map lock poisoned")
            .clear();
    }
}

/// Shared TTL memo for the two `git` discovery probes.
///
/// Cheap to clone (`Arc` handle); lives in `AppState` so every route shares
/// one memo.
#[derive(Clone)]
pub struct GitProbeCache {
    inner: Arc<GitProbeCacheState>,
}

struct GitProbeCacheState {
    ttl: Duration,
    discovery: ProbeSlots<Option<GitDiscovery>>,
    worktree_paths: ProbeSlots<Vec<PathBuf>>,
}

impl Default for GitProbeCache {
    fn default() -> Self {
        Self::with_ttl(git_probe_ttl_from_env())
    }
}

impl fmt::Debug for GitProbeCache {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("GitProbeCache")
            .field("ttl", &self.inner.ttl)
            .finish_non_exhaustive()
    }
}

// Identity comparison: two handles are equal exactly when they share one memo.
// Keeps `LocalDashboardResourcesProvider`'s derived `Eq` meaningful without
// making memo contents part of provider equality.
impl PartialEq for GitProbeCache {
    fn eq(&self, other: &Self) -> bool {
        Arc::ptr_eq(&self.inner, &other.inner)
    }
}

impl Eq for GitProbeCache {}

impl GitProbeCache {
    pub fn with_ttl(ttl: Duration) -> Self {
        Self {
            inner: Arc::new(GitProbeCacheState {
                ttl,
                discovery: ProbeSlots::default(),
                worktree_paths: ProbeSlots::default(),
            }),
        }
    }

    fn discover(&self, path: &Path, git_stats: &GitSpawnStats) -> Option<GitDiscovery> {
        self.inner
            .discovery
            .get_or_probe(&GitProbeKey::for_path(path), self.inner.ttl, || {
                GitDiscovery::probe(path, git_stats)
            })
    }

    fn worktree_paths(&self, path: &Path, git_stats: &GitSpawnStats) -> Vec<PathBuf> {
        self.inner
            .worktree_paths
            .get_or_probe(&GitProbeKey::for_path(path), self.inner.ttl, || {
                probe_git_worktree_paths(path, git_stats)
            })
    }

    /// Drop one root's memo so its next probe re-runs `git` immediately.
    pub fn evict(&self, path: &Path) {
        let key = GitProbeKey::for_path(path);
        self.inner.discovery.evict(&key);
        self.inner.worktree_paths.evict(&key);
    }

    /// Drop every memo. Used after this daemon itself mutates the worktree set
    /// (`git worktree add` / `git worktree remove`), so the mutation response's
    /// refreshed resources reflect the new state instead of a pre-mutation memo.
    pub fn clear(&self) {
        self.inner.discovery.clear();
        self.inner.worktree_paths.clear();
    }
}

fn git_probe_ttl_from_env() -> Duration {
    let millis = env::var("WS_DASHBOARD_GIT_PROBE_TTL_MS")
        .ok()
        .and_then(|raw| raw.trim().parse::<u64>().ok())
        .unwrap_or(DEFAULT_GIT_PROBE_TTL_MS);
    Duration::from_millis(millis)
}

#[derive(Clone)]
struct GitDiscovery {
    common_dir: PathBuf,
    worktree_dir: PathBuf,
    kind: WorkRootKind,
}

impl GitDiscovery {
    /// Spawns `git`. Call through `GitProbeCache::discover`, never directly.
    fn probe(path: &Path, git_stats: &GitSpawnStats) -> Option<Self> {
        // Single `git rev-parse` invocation queries all three values at once
        // (one output line per query flag, in flag order) instead of
        // spawning three separate `git` processes per work root.
        //
        // ExpectedNonZero: this is the check that answers "is this root a
        // git repo at all", and it exits non-zero routinely for a
        // plain-directory root - warning here would produce a continuous
        // stream at one warning per root per probe-TTL expiry.
        let outcome = capture(
            git_stats,
            path,
            &[
                "rev-parse",
                "--show-toplevel",
                "--path-format=absolute",
                "--git-common-dir",
                "--git-dir",
            ],
            GitFailureExpectation::ExpectedNonZero,
            git_timeout_from_env(),
        )
        .ok()?;

        let stdout = outcome.stdout;
        let mut lines = stdout.lines();
        let worktree_dir = non_empty_path(lines.next()?)?;
        let common_dir = non_empty_path(lines.next()?)?;
        let git_dir = non_empty_path(lines.next()?)?;
        let kind = if common_dir == git_dir {
            WorkRootKind::GitPrimaryRoot
        } else {
            WorkRootKind::GitLinkedWorktree
        };

        Some(Self {
            common_dir,
            worktree_dir,
            kind,
        })
    }

    fn workspace_label(&self) -> String {
        let root = if self.kind == WorkRootKind::GitPrimaryRoot {
            self.worktree_dir.as_path()
        } else {
            self.common_dir
                .parent()
                .unwrap_or_else(|| self.worktree_dir.as_path())
        };
        label_for_path(root)
    }
}

fn non_empty_path(line: &str) -> Option<PathBuf> {
    let trimmed = line.trim();
    (!trimmed.is_empty()).then(|| normalize_candidate_path(Path::new(trimmed)))
}

/// Spawns `git`. Call through `GitProbeCache::worktree_paths`, never directly.
fn probe_git_worktree_paths(path: &Path, git_stats: &GitSpawnStats) -> Vec<PathBuf> {
    // Runs only after the root is already known to be a repo (called from
    // `dashboard_resources_with_registry_sync` only once `discover_work_root`
    // reported it `Available`), so a non-zero exit here is genuinely
    // surprising.
    let Ok(outcome) = capture(
        git_stats,
        path,
        &["worktree", "list", "--porcelain"],
        GitFailureExpectation::Unexpected,
        git_timeout_from_env(),
    ) else {
        return Vec::new();
    };
    outcome
        .stdout
        .lines()
        .filter_map(|line| line.strip_prefix("worktree "))
        .map(|path| normalize_candidate_path(Path::new(path)))
        .collect()
}

fn normalize_candidate_path(path: &Path) -> PathBuf {
    if path.is_absolute() {
        return path.to_path_buf();
    }

    std::env::current_dir()
        .map(|cwd| cwd.join(path))
        .unwrap_or_else(|_| path.to_path_buf())
}

fn canonical_or_normalized(path: &Path) -> PathBuf {
    path.canonicalize()
        .unwrap_or_else(|_| normalize_candidate_path(path))
}

fn paths_equivalent(left: &Path, right: &Path) -> bool {
    // Deliberately NOT routed through `canonical_or_normalized`: when either
    // side fails to canonicalize, this must fall back to comparing BOTH
    // sides' `normalize_candidate_path` forms (discarding any successful
    // canonicalization on the other side too), matching the original
    // comparison semantics. `canonical_or_normalized` is a hash-key
    // derivation helper only and must not be reused here, since it would mix
    // a resolved form on one side with an unresolved form on the other.
    match (left.canonicalize(), right.canonicalize()) {
        (Ok(left), Ok(right)) => left == right,
        _ => normalize_candidate_path(left) == normalize_candidate_path(right),
    }
}

pub fn local_work_root_id_for_path(path: &Path) -> WorkRootId {
    OpaqueId::from(format!(
        "root-local-{}",
        stable_path_hash(&canonical_or_normalized(path))
    ))
}

fn label_for_path(path: &Path) -> String {
    path.file_name()
        .and_then(|name| name.to_str())
        .filter(|name| !name.is_empty())
        .map(str::to_owned)
        .unwrap_or_else(|| path.display().to_string())
}

fn state_status(
    availability: WorkRootAvailability,
    activation: WorkRootActivation,
) -> &'static str {
    if activation == WorkRootActivation::Offline {
        return "offline";
    }
    availability_status(availability)
}

fn availability_status(availability: WorkRootAvailability) -> &'static str {
    match availability {
        WorkRootAvailability::Available => "ready",
        WorkRootAvailability::Missing => "missing",
        WorkRootAvailability::Moved => "moved",
        WorkRootAvailability::Inaccessible => "inaccessible",
        WorkRootAvailability::Unknown => "unknown",
    }
}

// Per-row action hints: the shared activation actions plus, for linked
// worktrees only, the `worktree.remove` hint that gates the frontend's
// per-worktree "..." overflow menu (260525 Phase 3 B-1/B-2). The primary root
// and plain directories never expose it — a whole-workspace forget is the
// separate `workspace.remove` workspace-level action.
fn work_root_actions(kind: WorkRootKind, active: bool, available: bool) -> Vec<ActionHint> {
    let mut actions = activation_actions(active, available);
    if kind == WorkRootKind::GitLinkedWorktree {
        actions.push(ActionHint {
            id: "worktree.remove".to_owned(),
            label: "Remove worktree...".to_owned(),
            enabled: available && active,
        });
    }
    actions
}

fn activation_actions(active: bool, available: bool) -> Vec<ActionHint> {
    let mut actions = Vec::new();
    if active {
        actions.push(ActionHint {
            id: if available { "openRoot" } else { "reconnect" }.to_owned(),
            label: if available { "Open root" } else { "Reconnect" }.to_owned(),
            enabled: available,
        });
        actions.push(ActionHint {
            id: "workRoot.activation.offline".to_owned(),
            label: "Go offline".to_owned(),
            enabled: true,
        });
    } else {
        actions.push(ActionHint {
            id: "workRoot.activation.online".to_owned(),
            label: "Go online".to_owned(),
            enabled: true,
        });
    }
    actions
}

fn stable_path_hash(path: &Path) -> String {
    const FNV_OFFSET: u64 = 0xcbf29ce484222325;
    const FNV_PRIME: u64 = 0x100000001b3;

    let mut hash = FNV_OFFSET;
    for byte in path.to_string_lossy().as_bytes() {
        hash ^= u64::from(*byte);
        hash = hash.wrapping_mul(FNV_PRIME);
    }

    format!("{hash:016x}")
}

#[cfg(test)]
mod tests {
    use super::*;
    #[cfg(unix)]
    use std::os::unix::fs::{symlink, PermissionsExt};
    // Test-fixture-only helpers (`git_available`/`git`, below) spawn `git`
    // directly rather than through `git_exec::capture`, mirroring the
    // fixture-setup pattern used elsewhere in this crate (e.g.
    // `tests/routes.rs`'s own `run_git`/`init_git_repo`) - these seed repo
    // state for a test, they are not part of the production spawn-counting
    // seam.
    use std::process::Command;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::time::{SystemTime, UNIX_EPOCH};

    #[test]
    fn git_probe_memo_serves_cached_value_inside_ttl_and_reprobes_after_expiry() {
        let slots: ProbeSlots<usize> = ProbeSlots::default();
        let key = GitProbeKey::for_path(Path::new("/ws-dashboard/probe-ttl"));
        let probes = AtomicUsize::new(0);
        let probe = || probes.fetch_add(1, Ordering::SeqCst) + 1;
        let ttl = Duration::from_millis(150);

        assert_eq!(slots.get_or_probe(&key, ttl, probe), 1);
        assert_eq!(slots.get_or_probe(&key, ttl, probe), 1);
        assert_eq!(
            probes.load(Ordering::SeqCst),
            1,
            "a repeat inside the TTL must be served from the memo"
        );

        std::thread::sleep(ttl + Duration::from_millis(50));
        assert_eq!(slots.get_or_probe(&key, ttl, probe), 2);
        assert_eq!(
            probes.load(Ordering::SeqCst),
            2,
            "an expired entry must re-probe"
        );
    }

    #[test]
    fn git_probe_memo_single_flights_concurrent_misses_for_one_key() {
        let slots: Arc<ProbeSlots<usize>> = Arc::new(ProbeSlots::default());
        let key = GitProbeKey::for_path(Path::new("/ws-dashboard/probe-single-flight"));
        let probes = Arc::new(AtomicUsize::new(0));
        let ttl = Duration::from_secs(60);

        // Mirrors /git/status, /git/branches and /api/dashboard/resources all
        // missing the same root at the same instant.
        let handles: Vec<_> = (0..2)
            .map(|_| {
                let slots = Arc::clone(&slots);
                let key = key.clone();
                let probes = Arc::clone(&probes);
                std::thread::spawn(move || {
                    slots.get_or_probe(&key, ttl, || {
                        probes.fetch_add(1, Ordering::SeqCst);
                        std::thread::sleep(Duration::from_millis(100));
                        7
                    })
                })
            })
            .collect();
        for handle in handles {
            assert_eq!(handle.join().expect("probe thread"), 7);
        }

        assert_eq!(
            probes.load(Ordering::SeqCst),
            1,
            "concurrent misses for one key must collapse into a single probe"
        );
    }

    #[test]
    fn git_probe_key_collapses_mixed_separator_spellings_of_one_path() {
        // The work-root registry observably stores Windows worktree paths with
        // mixed separators; both spellings name one directory and must land on
        // one memo key, or that root keeps spawning `git` twice per pass.
        let mixed = Path::new("D:/Workspace/Repos/InspectTGV_AIDriven/.git\\ws-worktree\\jpeg");
        let unified = Path::new("D:/Workspace/Repos/InspectTGV_AIDriven/.git/ws-worktree/jpeg");

        assert_eq!(GitProbeKey::for_path(mixed), GitProbeKey::for_path(unified));
    }

    #[test]
    fn discover_work_root_reports_missing_immediately_and_evicts_the_probe_memo() {
        let root = temp_path("probe-evict");
        fs::create_dir_all(&root).expect("create root");
        // A TTL far longer than the test: only the `git` probes are memoized,
        // so availability must still flip the moment the directory goes away.
        let probes = GitProbeCache::with_ttl(Duration::from_secs(600));
        let stats = GitSpawnStats::default();

        let available = discover_work_root(&root, &probes, &stats);
        assert_eq!(available.availability, WorkRootAvailability::Available);
        assert_eq!(probes.inner.discovery.len(), 1);

        remove_temp(&root);
        let gone = discover_work_root(&root, &probes, &stats);

        assert_ne!(
            gone.availability,
            WorkRootAvailability::Available,
            "availability detection must not be served from the probe memo"
        );
        assert_eq!(
            probes.inner.discovery.len(),
            0,
            "an unavailable root must drop its memo so it re-probes on return"
        );
    }

    #[test]
    fn local_provider_maps_plain_directory_to_work_root_view() {
        let base = temp_path("plain");
        let root = base.join("plain");
        fs::create_dir_all(&root).expect("create plain workRoot");

        let view = LocalDashboardResourcesProvider::new(vec![LocalWorkRootCandidate::new(&root)])
            .dashboard_resources();

        assert_eq!(view.workspaces.len(), 1);
        assert_eq!(view.workspaces[0].label, "plain");
        assert!(view.workspaces[0].compactable);
        assert_eq!(view.workspaces[0].work_roots.len(), 1);
        let work_root = &view.workspaces[0].work_roots[0];
        assert_eq!(work_root.kind, WorkRootKind::PlainDirectory);
        assert_eq!(work_root.status, WorkRootStatus::Online);
        assert_eq!(work_root.state.status, "ready");
        assert_eq!(work_root.actions[0].id, "openRoot");
        assert!(work_root.main_instances.is_empty());

        remove_temp(&base);
    }

    #[cfg(unix)]
    #[test]
    fn local_provider_dedups_plain_directory_reached_via_symlinked_parent() {
        let base = temp_path("plain-symlink-alias");
        let real_parent = base.join("real");
        let alias_parent = base.join("real-alias");
        let root = real_parent.join("plain");
        fs::create_dir_all(&root).expect("create plain workRoot");
        symlink(&real_parent, &alias_parent).expect("create symlink alias to parent dir");
        let alias = alias_parent.join("plain");

        // The same physical (non-git) plain directory is reached two ways
        // here (direct path vs. through a symlinked parent path segment,
        // same basename so `WorkspaceKey.label` matches either way); the
        // plain-directory `workspace_key` branch of `discover_existing_dir`
        // must canonicalize before hashing its id so these collapse to a
        // single workspace/work-root, not two.
        let view = LocalDashboardResourcesProvider::new(vec![
            LocalWorkRootCandidate::new(&root),
            LocalWorkRootCandidate::new(&alias),
        ])
        .dashboard_resources();

        assert_eq!(view.workspaces.len(), 1);
        assert_eq!(
            view.workspaces[0].work_roots.len(),
            1,
            "plain directory reached via a symlinked parent path must dedup \
             to a single entry, not add a duplicate"
        );

        remove_temp(&base);
    }

    #[cfg(unix)]
    #[test]
    fn discovered_unusable_workspace_key_is_stable_across_symlink_alias() {
        let base = temp_path("unusable-symlink-alias");
        let target = base.join("target");
        let alias = base.join("alias");
        fs::create_dir_all(&target).expect("create target");
        symlink(&target, &alias).expect("create symlink alias");
        let original = fs::metadata(&target)
            .expect("target metadata")
            .permissions()
            .mode();
        fs::set_permissions(&target, fs::Permissions::from_mode(0o000))
            .expect("make target unreadable");

        // Both paths name the same physical (inaccessible) directory; the
        // `discovered_unusable` `workspace_key` must canonicalize before
        // hashing so the direct path and its symlink alias yield the same
        // bucket id.
        let probes = GitProbeCache::default();
        let stats = GitSpawnStats::default();
        let direct = discover_work_root(&target, &probes, &stats);
        let via_alias = discover_work_root(&alias, &probes, &stats);

        fs::set_permissions(&target, fs::Permissions::from_mode(original))
            .expect("restore permissions");

        assert_eq!(direct.availability, WorkRootAvailability::Inaccessible);
        assert_eq!(via_alias.availability, WorkRootAvailability::Inaccessible);
        assert_eq!(direct.workspace_key.id, via_alias.workspace_key.id);

        remove_temp(&base);
    }

    #[test]
    fn local_provider_prunes_workspaces_without_available_work_roots() {
        let parent = temp_path("missing-parent");
        fs::create_dir_all(&parent).expect("create parent");
        let moved = parent.join("moved");
        let offline = parent.join("offline").join("root");

        let view = LocalDashboardResourcesProvider::new(vec![
            LocalWorkRootCandidate::new(&moved),
            LocalWorkRootCandidate::new(&offline),
        ])
        .dashboard_resources();

        assert!(
            view.workspaces.is_empty(),
            "no-active-workRoot policy prunes unavailable-only workspaces"
        );

        remove_temp(&parent);
    }

    #[cfg(unix)]
    #[test]
    fn local_provider_prunes_unreadable_directory_without_active_child() {
        let root = temp_path("inaccessible");
        fs::create_dir_all(&root).expect("create inaccessible root");
        let original = fs::metadata(&root)
            .expect("root metadata")
            .permissions()
            .mode();
        fs::set_permissions(&root, fs::Permissions::from_mode(0o000))
            .expect("make root unreadable");

        let view = LocalDashboardResourcesProvider::new(vec![LocalWorkRootCandidate::new(&root)])
            .dashboard_resources();

        fs::set_permissions(&root, fs::Permissions::from_mode(original))
            .expect("restore permissions");

        assert!(
            view.workspaces.is_empty(),
            "unreadable root-only workspace has no active workRoot to show"
        );

        remove_temp(&root);
    }

    #[cfg(unix)]
    #[test]
    fn local_provider_prunes_symlink_when_target_disappears() {
        let base = temp_path("symlink");
        let target = base.join("target");
        let link = base.join("link");
        fs::create_dir_all(&target).expect("create target");
        symlink(&target, &link).expect("create symlink");

        let online = LocalDashboardResourcesProvider::new(vec![LocalWorkRootCandidate::new(&link)])
            .dashboard_resources();
        let online_id = online.workspaces[0].work_roots[0].id.clone();

        fs::remove_dir_all(&target).expect("remove target");

        let missing =
            LocalDashboardResourcesProvider::new(vec![LocalWorkRootCandidate::new(&link)])
                .dashboard_resources();

        assert!(online_id.as_str().starts_with("root-local-"));
        assert!(missing.workspaces.is_empty());

        remove_temp(&base);
    }

    #[test]
    fn local_provider_distinguishes_git_primary_roots_and_linked_worktrees() {
        if !git_available() {
            return;
        }

        let base = temp_path("git");
        let primary = base.join("primary");
        let linked = base.join("linked");
        fs::create_dir_all(&primary).expect("create primary");
        git(&primary, &["init"]);
        git(
            &primary,
            &["config", "user.email", "ws-dashboard@example.local"],
        );
        git(&primary, &["config", "user.name", "ws dashboard"]);
        fs::write(primary.join("README.md"), "dashboard\n").expect("write readme");
        git(&primary, &["add", "README.md"]);
        git(&primary, &["commit", "-m", "seed"]);
        git(
            &primary,
            &["worktree", "add", linked.to_str().expect("linked path")],
        );

        let view = LocalDashboardResourcesProvider::new(vec![
            LocalWorkRootCandidate::new(&primary),
            LocalWorkRootCandidate::new(&linked),
        ])
        .dashboard_resources();

        assert_eq!(view.workspaces.len(), 1);
        assert_eq!(
            view.workspaces[0].work_roots.len(),
            2,
            "primary root and its linked worktree must remain distinct entries, \
             not collapse or duplicate"
        );
        let kinds: Vec<_> = view.workspaces[0]
            .work_roots
            .iter()
            .map(|root| root.kind)
            .collect();
        assert!(kinds.contains(&WorkRootKind::GitPrimaryRoot));
        assert!(kinds.contains(&WorkRootKind::GitLinkedWorktree));
        assert!(view.workspaces[0]
            .work_roots
            .iter()
            .all(|root| root.status == WorkRootStatus::Online));

        remove_temp(&base);
    }

    #[cfg(unix)]
    #[test]
    fn local_provider_dedups_linked_worktree_reached_via_symlink_alias() {
        if !git_available() {
            return;
        }

        let base = temp_path("git-symlink-alias");
        let primary = base.join("primary");
        let linked = base.join("linked");
        let linked_alias = base.join("linked-alias");
        fs::create_dir_all(&primary).expect("create primary");
        git(&primary, &["init"]);
        git(
            &primary,
            &["config", "user.email", "ws-dashboard@example.local"],
        );
        git(&primary, &["config", "user.name", "ws dashboard"]);
        fs::write(primary.join("README.md"), "dashboard\n").expect("write readme");
        git(&primary, &["add", "README.md"]);
        git(&primary, &["commit", "-m", "seed"]);
        git(
            &primary,
            &["worktree", "add", linked.to_str().expect("linked path")],
        );
        symlink(&linked, &linked_alias).expect("create symlink alias to linked worktree");

        // The same physical linked-worktree directory is reached two ways here,
        // mirroring how git_worktree_add_submit's own top-level registration of
        // a freshly created worktree and the primary root's `git worktree list`
        // discovery of that same worktree can diverge textually even though
        // they name the same directory.
        let view = LocalDashboardResourcesProvider::new(vec![
            LocalWorkRootCandidate::new(&primary),
            LocalWorkRootCandidate::new(&linked_alias),
        ])
        .dashboard_resources();

        assert_eq!(view.workspaces.len(), 1);
        assert_eq!(
            view.workspaces[0].work_roots.len(),
            2,
            "the linked worktree reached via a symlink alias must dedup to a \
             single entry alongside the distinct primary root, not add a duplicate"
        );
        let kinds: Vec<_> = view.workspaces[0]
            .work_roots
            .iter()
            .map(|root| root.kind)
            .collect();
        assert!(kinds.contains(&WorkRootKind::GitPrimaryRoot));
        assert!(kinds.contains(&WorkRootKind::GitLinkedWorktree));

        remove_temp(&base);
    }

    #[test]
    fn local_provider_discovers_linked_worktrees_from_primary_root() {
        if !git_available() {
            return;
        }

        let base = temp_path("git-discover-linked");
        let primary = base.join("primary");
        let linked = base.join("linked");
        fs::create_dir_all(&primary).expect("create primary");
        git(&primary, &["init"]);
        git(
            &primary,
            &["config", "user.email", "ws-dashboard@example.local"],
        );
        git(&primary, &["config", "user.name", "ws dashboard"]);
        fs::write(primary.join("README.md"), "dashboard\n").expect("write readme");
        git(&primary, &["add", "README.md"]);
        git(&primary, &["commit", "-m", "seed"]);
        git(
            &primary,
            &["worktree", "add", linked.to_str().expect("linked path")],
        );

        let sync =
            LocalDashboardResourcesProvider::new(vec![LocalWorkRootCandidate::new(&primary)])
                .dashboard_resources_with_registry_sync();

        assert_eq!(sync.view.workspaces.len(), 1);
        assert_eq!(sync.view.workspaces[0].work_roots.len(), 2);
        let kinds: Vec<_> = sync.view.workspaces[0]
            .work_roots
            .iter()
            .map(|root| root.kind)
            .collect();
        assert!(kinds.contains(&WorkRootKind::GitPrimaryRoot));
        assert!(kinds.contains(&WorkRootKind::GitLinkedWorktree));
        assert_eq!(sync.discovered_registry_roots.len(), 1);
        assert!(paths_equivalent(
            &sync.discovered_registry_roots[0].path,
            &linked
        ));
        assert_eq!(
            sync.discovered_registry_roots[0].provenance,
            WorkRootProvenance::Discovered
        );

        remove_temp(&base);
    }

    fn git_available() -> bool {
        Command::new("git")
            .arg("--version")
            .output()
            .map(|output| output.status.success())
            .unwrap_or(false)
    }

    fn git(path: &Path, args: &[&str]) {
        let output = Command::new("git")
            .arg("-C")
            .arg(path)
            .args(args)
            .output()
            .expect("run git");
        assert!(
            output.status.success(),
            "git {:?} failed: {}",
            args,
            String::from_utf8_lossy(&output.stderr)
        );
    }

    fn temp_path(name: &str) -> PathBuf {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system time after epoch")
            .as_nanos();
        std::env::temp_dir().join(format!("ws-dashboard-discovery-{name}-{unique}"))
    }

    fn remove_temp(path: &Path) {
        let _ = fs::remove_dir_all(path);
    }
}
