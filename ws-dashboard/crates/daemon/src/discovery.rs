use std::collections::{BTreeMap, HashMap};
use std::env;
use std::fs;
use std::io;
use std::path::{Path, PathBuf};
use std::process::Command;

use ws_dashboard_core::{
    ActionHint, DashboardResourcesView, InstanceKind, InstanceRole, InstanceView, InteractionMode,
    OpaqueId, ResourcePath, ServerView, ViewState, WorkRootActivation, WorkRootAvailability,
    WorkRootId, WorkRootKind, WorkRootStatus, WorkRootView, WorkspaceView,
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

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct LocalDashboardResourcesProvider {
    server_id: OpaqueId,
    server_label: String,
    candidates: Vec<LocalWorkRootCandidate>,
    registry_activations: HashMap<WorkRootId, WorkRootActivation>,
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
        }
    }

    pub fn dashboard_resources_with_registry_sync(&self) -> DashboardResourcesSync {
        let mut workspaces = BTreeMap::<WorkspaceKey, WorkspaceBuilder>::new();
        let mut discovered_registry_roots = Vec::new();
        let mut pruned_work_root_ids = Vec::new();

        for candidate in &self.candidates {
            let owner = discover_work_root(&candidate.path);
            let workspace_key = owner.workspace_key.clone();
            let linked_paths = if owner.availability == WorkRootAvailability::Available {
                git_worktree_paths(&candidate.path)
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
                let mut linked = discover_work_root(&linked_path);
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
            actions: activation_actions(active, available),
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

fn discover_work_root(path: &Path) -> DiscoveredWorkRoot {
    let normalized = normalize_candidate_path(path);

    match fs::metadata(&normalized) {
        Ok(metadata) if metadata.is_dir() => discover_existing_dir(normalized),
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
    }
}

fn discover_existing_dir(path: PathBuf) -> DiscoveredWorkRoot {
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

    match GitDiscovery::discover(&path) {
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
                id: OpaqueId::from(format!("workspace-local-{}", stable_path_hash(&path))),
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
            id: OpaqueId::from(format!("workspace-local-{}", stable_path_hash(&path))),
            label: label_for_path(&path),
        },
        path,
        kind: WorkRootKind::PlainDirectory,
        status,
        availability,
        error: Some(error.to_owned()),
    }
}

struct GitDiscovery {
    common_dir: PathBuf,
    worktree_dir: PathBuf,
    kind: WorkRootKind,
}

impl GitDiscovery {
    fn discover(path: &Path) -> Option<Self> {
        // Single `git rev-parse` invocation queries all three values at once
        // (one output line per query flag, in flag order) instead of
        // spawning three separate `git` processes per work root.
        let output = Command::new("git")
            .arg("-C")
            .arg(path)
            .args([
                "rev-parse",
                "--show-toplevel",
                "--path-format=absolute",
                "--git-common-dir",
                "--git-dir",
            ])
            .output()
            .ok()?;

        if !output.status.success() {
            return None;
        }

        let stdout = String::from_utf8(output.stdout).ok()?;
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

fn git_worktree_paths(path: &Path) -> Vec<PathBuf> {
    let output = match Command::new("git")
        .arg("-C")
        .arg(path)
        .args(["worktree", "list", "--porcelain"])
        .output()
    {
        Ok(output) if output.status.success() => output,
        _ => return Vec::new(),
    };
    let Ok(raw) = String::from_utf8(output.stdout) else {
        return Vec::new();
    };
    raw.lines()
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
    canonical_or_normalized(left) == canonical_or_normalized(right)
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
    use std::time::{SystemTime, UNIX_EPOCH};

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
