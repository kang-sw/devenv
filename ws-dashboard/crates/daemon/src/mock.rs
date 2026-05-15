use ws_dashboard_core::{
    ActionHint, DashboardResourcesView, InstanceKind, InstanceRole, InstanceView, InteractionMode,
    OpaqueId, ResourcePath, ServerView, ViewState, WorkRootKind, WorkRootStatus, WorkRootView,
    WorkspaceView,
};

use crate::resources::DashboardResourcesProvider;

#[derive(Clone, Debug, Default)]
pub struct MockDashboardResourcesProvider;

impl DashboardResourcesProvider for MockDashboardResourcesProvider {
    fn dashboard_resources(&self) -> DashboardResourcesView {
        // CONTRACT: deterministic fixture-backed data must cover singleton
        // chains, multi-root workspaces, all workRoot kinds, offline or
        // inaccessible roots, main/sub instances, state fields, and actions.
        DashboardResourcesView {
            server: ServerView {
                id: OpaqueId::from("server-local"),
                label: "Local ws dashboard".to_owned(),
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
            },
            workspaces: vec![
                WorkspaceView {
                    id: OpaqueId::from("workspace-devenv"),
                    label: "devenv".to_owned(),
                    state: ViewState {
                        status: "ready".to_owned(),
                        loading: false,
                        stale: false,
                        error: None,
                    },
                    compactable: false,
                    work_roots: vec![
                        WorkRootView {
                            id: OpaqueId::from("root-devenv-primary"),
                            resource_path: ResourcePath {
                                server_id: OpaqueId::from("server-local"),
                                workspace_id: OpaqueId::from("workspace-devenv"),
                                work_root_id: OpaqueId::from("root-devenv-primary"),
                                instance_id: None,
                            },
                            label: "devenv primary".to_owned(),
                            kind: WorkRootKind::GitPrimaryRoot,
                            status: WorkRootStatus::Online,
                            state: ViewState {
                                status: "ready".to_owned(),
                                loading: false,
                                stale: false,
                                error: None,
                            },
                            compactable: false,
                            main_instances: vec![InstanceView {
                                id: OpaqueId::from("instance-devenv-main"),
                                resource_path: ResourcePath {
                                    server_id: OpaqueId::from("server-local"),
                                    workspace_id: OpaqueId::from("workspace-devenv"),
                                    work_root_id: OpaqueId::from("root-devenv-primary"),
                                    instance_id: Some(OpaqueId::from("instance-devenv-main")),
                                },
                                role: InstanceRole::Main,
                                kind: InstanceKind::Harness,
                                interaction_mode: InteractionMode::Direct,
                                label: "Main harness".to_owned(),
                                state: ViewState {
                                    status: "active".to_owned(),
                                    loading: false,
                                    stale: false,
                                    error: None,
                                },
                                sub_instances: vec![InstanceView {
                                    id: OpaqueId::from("instance-devenv-subagent"),
                                    resource_path: ResourcePath {
                                        server_id: OpaqueId::from("server-local"),
                                        workspace_id: OpaqueId::from("workspace-devenv"),
                                        work_root_id: OpaqueId::from("root-devenv-primary"),
                                        instance_id: Some(OpaqueId::from(
                                            "instance-devenv-subagent",
                                        )),
                                    },
                                    role: InstanceRole::Sub,
                                    kind: InstanceKind::Agent,
                                    interaction_mode: InteractionMode::Delegated,
                                    label: "Review agent".to_owned(),
                                    state: ViewState {
                                        status: "idle".to_owned(),
                                        loading: false,
                                        stale: true,
                                        error: None,
                                    },
                                    sub_instances: vec![],
                                    actions: vec![ActionHint {
                                        id: "inspect".to_owned(),
                                        label: "Inspect".to_owned(),
                                        enabled: true,
                                    }],
                                }],
                                actions: vec![ActionHint {
                                    id: "open".to_owned(),
                                    label: "Open".to_owned(),
                                    enabled: true,
                                }],
                            }],
                            actions: vec![ActionHint {
                                id: "openRoot".to_owned(),
                                label: "Open root".to_owned(),
                                enabled: true,
                            }],
                        },
                        WorkRootView {
                            id: OpaqueId::from("root-devenv-linked"),
                            resource_path: ResourcePath {
                                server_id: OpaqueId::from("server-local"),
                                workspace_id: OpaqueId::from("workspace-devenv"),
                                work_root_id: OpaqueId::from("root-devenv-linked"),
                                instance_id: None,
                            },
                            label: "devenv linked worktree".to_owned(),
                            kind: WorkRootKind::GitLinkedWorktree,
                            status: WorkRootStatus::Online,
                            state: ViewState {
                                status: "syncing".to_owned(),
                                loading: true,
                                stale: false,
                                error: None,
                            },
                            compactable: false,
                            main_instances: vec![InstanceView {
                                id: OpaqueId::from("instance-linked-main"),
                                resource_path: ResourcePath {
                                    server_id: OpaqueId::from("server-local"),
                                    workspace_id: OpaqueId::from("workspace-devenv"),
                                    work_root_id: OpaqueId::from("root-devenv-linked"),
                                    instance_id: Some(OpaqueId::from("instance-linked-main")),
                                },
                                role: InstanceRole::Main,
                                kind: InstanceKind::Terminal,
                                interaction_mode: InteractionMode::Direct,
                                label: "Linked shell".to_owned(),
                                state: ViewState {
                                    status: "loading".to_owned(),
                                    loading: true,
                                    stale: false,
                                    error: None,
                                },
                                sub_instances: vec![],
                                actions: vec![ActionHint {
                                    id: "open".to_owned(),
                                    label: "Open".to_owned(),
                                    enabled: false,
                                }],
                            }],
                            actions: vec![ActionHint {
                                id: "focusRoot".to_owned(),
                                label: "Focus root".to_owned(),
                                enabled: true,
                            }],
                        },
                        WorkRootView {
                            id: OpaqueId::from("root-devenv-offline"),
                            resource_path: ResourcePath {
                                server_id: OpaqueId::from("server-local"),
                                workspace_id: OpaqueId::from("workspace-devenv"),
                                work_root_id: OpaqueId::from("root-devenv-offline"),
                                instance_id: None,
                            },
                            label: "offline archive".to_owned(),
                            kind: WorkRootKind::PlainDirectory,
                            status: WorkRootStatus::Offline,
                            state: ViewState {
                                status: "offline".to_owned(),
                                loading: false,
                                stale: true,
                                error: Some("workRoot is offline".to_owned()),
                            },
                            compactable: false,
                            main_instances: vec![],
                            actions: vec![ActionHint {
                                id: "reconnect".to_owned(),
                                label: "Reconnect".to_owned(),
                                enabled: false,
                            }],
                        },
                    ],
                    actions: vec![ActionHint {
                        id: "openWorkspace".to_owned(),
                        label: "Open workspace".to_owned(),
                        enabled: true,
                    }],
                },
                WorkspaceView {
                    id: OpaqueId::from("workspace-notes"),
                    label: "notes".to_owned(),
                    state: ViewState {
                        status: "degraded".to_owned(),
                        loading: false,
                        stale: true,
                        error: Some("one workRoot is inaccessible".to_owned()),
                    },
                    compactable: true,
                    work_roots: vec![WorkRootView {
                        id: OpaqueId::from("root-notes"),
                        resource_path: ResourcePath {
                            server_id: OpaqueId::from("server-local"),
                            workspace_id: OpaqueId::from("workspace-notes"),
                            work_root_id: OpaqueId::from("root-notes"),
                            instance_id: None,
                        },
                        label: "notes".to_owned(),
                        kind: WorkRootKind::PlainDirectory,
                        status: WorkRootStatus::Inaccessible,
                        state: ViewState {
                            status: "inaccessible".to_owned(),
                            loading: false,
                            stale: true,
                            error: Some("permission denied".to_owned()),
                        },
                        compactable: true,
                        main_instances: vec![InstanceView {
                            id: OpaqueId::from("instance-notes-main"),
                            resource_path: ResourcePath {
                                server_id: OpaqueId::from("server-local"),
                                workspace_id: OpaqueId::from("workspace-notes"),
                                work_root_id: OpaqueId::from("root-notes"),
                                instance_id: Some(OpaqueId::from("instance-notes-main")),
                            },
                            role: InstanceRole::Main,
                            kind: InstanceKind::Viewer,
                            interaction_mode: InteractionMode::Passive,
                            label: "Notes viewer".to_owned(),
                            state: ViewState {
                                status: "blocked".to_owned(),
                                loading: false,
                                stale: true,
                                error: Some("waiting for access".to_owned()),
                            },
                            sub_instances: vec![],
                            actions: vec![ActionHint {
                                id: "inspect".to_owned(),
                                label: "Inspect".to_owned(),
                                enabled: true,
                            }],
                        }],
                        actions: vec![ActionHint {
                            id: "reveal".to_owned(),
                            label: "Reveal".to_owned(),
                            enabled: false,
                        }],
                    }],
                    actions: vec![ActionHint {
                        id: "refreshWorkspace".to_owned(),
                        label: "Refresh workspace".to_owned(),
                        enabled: true,
                    }],
                },
            ],
        }
    }
}
