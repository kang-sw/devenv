use serde::{Deserialize, Serialize};

use crate::{
    InstanceId, InstanceKind, InstanceRole, InteractionMode, ResourcePath,
    ServerConnectionStatus, ServerId, ServerKind, WorkRootActivation, WorkRootAvailability,
    WorkRootId, WorkRootKind, WorkRootStatus, WorkspaceId,
};

// CONTRACT: The first visible dashboard API returns the full hierarchy instead
// of pre-collapsing compactable singleton rows.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DashboardResourcesView {
    pub server: ServerView,
    pub workspaces: Vec<WorkspaceView>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DashboardServersView {
    pub servers: Vec<ServerConnectionView>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ServerConnectionView {
    pub id: ServerId,
    pub label: String,
    pub kind: ServerKind,
    pub status: ServerConnectionStatus,
    pub state: ViewState,
    pub actions: Vec<ActionHint>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ServerView {
    pub id: ServerId,
    pub label: String,
    pub state: ViewState,
    pub actions: Vec<ActionHint>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceView {
    pub id: WorkspaceId,
    pub label: String,
    pub state: ViewState,
    pub compactable: bool,
    pub work_roots: Vec<WorkRootView>,
    pub actions: Vec<ActionHint>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkRootView {
    pub id: WorkRootId,
    pub resource_path: ResourcePath,
    pub label: String,
    pub kind: WorkRootKind,
    pub activation: WorkRootActivation,
    pub availability: WorkRootAvailability,
    pub status: WorkRootStatus,
    pub state: ViewState,
    pub compactable: bool,
    pub main_instances: Vec<InstanceView>,
    pub actions: Vec<ActionHint>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstanceView {
    pub id: InstanceId,
    pub resource_path: ResourcePath,
    pub role: InstanceRole,
    pub kind: InstanceKind,
    pub interaction_mode: InteractionMode,
    pub label: String,
    pub state: ViewState,
    pub sub_instances: Vec<InstanceView>,
    pub actions: Vec<ActionHint>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ViewState {
    pub status: String,
    pub loading: bool,
    pub stale: bool,
    pub error: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ActionHint {
    pub id: String,
    pub label: String,
    pub enabled: bool,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{
        InstanceKind, InstanceRole, InteractionMode, OpaqueId, ResourcePath,
        ServerConnectionStatus, ServerKind, WorkRootActivation, WorkRootAvailability,
        WorkRootKind, WorkRootStatus,
    };

    #[test]
    fn dashboard_resources_view_preserves_full_hierarchy_shape() {
        // CONTRACT: serialized JSON must expose server, workspaces, workRoots,
        // mainInstances, and subInstances using camelCase dashboard API names.
        let value = serde_json::to_value(sample_view()).expect("serialize dashboard resources");

        assert!(value.get("server").is_some());
        assert!(value.get("workspaces").is_some());
        assert!(value.get("work_roots").is_none());
        assert!(value.get("main_instances").is_none());
        assert!(value.get("sub_instances").is_none());

        let workspace = &value["workspaces"][0];
        assert!(workspace.get("workRoots").is_some());
        assert!(workspace.get("work_roots").is_none());
        assert_eq!(
            workspace["workRoots"][0]["resourcePath"]["workRootId"],
            "root-primary"
        );
        assert_eq!(workspace["workRoots"][0]["activation"], "online");
        assert_eq!(workspace["workRoots"][0]["availability"], "available");

        let main_instance = &workspace["workRoots"][0]["mainInstances"][0];
        assert_eq!(main_instance["role"], "main");
        assert!(main_instance.get("subInstances").is_some());
        assert!(main_instance.get("main_instances").is_none());
        assert!(main_instance.get("sub_instances").is_none());
        assert_eq!(main_instance["subInstances"][0]["role"], "sub");
    }

    #[test]
    fn view_rows_include_state_and_action_hints() {
        // CONTRACT: rows expose status/loading/stale/error state and action
        // hints so frontend empty/error/loading/action UI can use one shape.
        let value = serde_json::to_value(sample_view()).expect("serialize dashboard resources");

        assert_eq!(
            value["server"]["state"],
            serde_json::json!({
                "status": "online",
                "loading": false,
                "stale": false,
                "error": null
            })
        );
        assert_eq!(value["server"]["actions"][0]["id"], "refresh");
        assert_eq!(value["server"]["actions"][0]["enabled"], true);
        assert_eq!(value["workspaces"][0]["compactable"], false);
        assert_eq!(value["workspaces"][0]["workRoots"][0]["compactable"], true);
        assert_eq!(
            value["workspaces"][0]["workRoots"][0]["mainInstances"][0]["actions"][0]["label"],
            "Open"
        );
    }

    #[test]
    fn dashboard_servers_view_serializes_connection_metadata() {
        let value = serde_json::to_value(DashboardServersView {
            servers: vec![ServerConnectionView {
                id: OpaqueId::from("server-remote"),
                label: "Remote".to_owned(),
                kind: ServerKind::SshRemote,
                status: ServerConnectionStatus::AuthRequired,
                state: ViewState {
                    status: "authRequired".to_owned(),
                    loading: false,
                    stale: false,
                    error: None,
                },
                actions: vec![ActionHint {
                    id: "enterPassphrase".to_owned(),
                    label: "Enter passphrase".to_owned(),
                    enabled: true,
                }],
            }],
        })
        .expect("serialize server list");

        assert_eq!(value["servers"][0]["id"], "server-remote");
        assert_eq!(value["servers"][0]["kind"], "sshRemote");
        assert_eq!(value["servers"][0]["status"], "authRequired");
        assert_eq!(
            value["servers"][0]["actions"][0]["id"],
            "enterPassphrase"
        );
    }

    fn sample_view() -> DashboardResourcesView {
        DashboardResourcesView {
            server: ServerView {
                id: OpaqueId::from("server-local"),
                label: "Local".to_owned(),
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
            workspaces: vec![WorkspaceView {
                id: OpaqueId::from("workspace-devenv"),
                label: "devenv".to_owned(),
                state: ViewState {
                    status: "ready".to_owned(),
                    loading: false,
                    stale: false,
                    error: None,
                },
                compactable: false,
                work_roots: vec![WorkRootView {
                    id: OpaqueId::from("root-primary"),
                    resource_path: ResourcePath {
                        server_id: OpaqueId::from("server-local"),
                        workspace_id: OpaqueId::from("workspace-devenv"),
                        work_root_id: OpaqueId::from("root-primary"),
                        instance_id: None,
                    },
                    label: "Primary root".to_owned(),
                    kind: WorkRootKind::GitPrimaryRoot,
                    activation: WorkRootActivation::Online,
                    availability: WorkRootAvailability::Available,
                    status: WorkRootStatus::Online,
                    state: ViewState {
                        status: "ready".to_owned(),
                        loading: false,
                        stale: false,
                        error: None,
                    },
                    compactable: true,
                    main_instances: vec![InstanceView {
                        id: OpaqueId::from("instance-main"),
                        resource_path: ResourcePath {
                            server_id: OpaqueId::from("server-local"),
                            workspace_id: OpaqueId::from("workspace-devenv"),
                            work_root_id: OpaqueId::from("root-primary"),
                            instance_id: Some(OpaqueId::from("instance-main")),
                        },
                        role: InstanceRole::Main,
                        kind: InstanceKind::Harness,
                        interaction_mode: InteractionMode::Direct,
                        label: "Main".to_owned(),
                        state: ViewState {
                            status: "ready".to_owned(),
                            loading: false,
                            stale: false,
                            error: None,
                        },
                        sub_instances: vec![InstanceView {
                            id: OpaqueId::from("instance-sub"),
                            resource_path: ResourcePath {
                                server_id: OpaqueId::from("server-local"),
                                workspace_id: OpaqueId::from("workspace-devenv"),
                                work_root_id: OpaqueId::from("root-primary"),
                                instance_id: Some(OpaqueId::from("instance-sub")),
                            },
                            role: InstanceRole::Sub,
                            kind: InstanceKind::Agent,
                            interaction_mode: InteractionMode::Delegated,
                            label: "Sub".to_owned(),
                            state: ViewState {
                                status: "idle".to_owned(),
                                loading: false,
                                stale: false,
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
                }],
                actions: vec![ActionHint {
                    id: "openWorkspace".to_owned(),
                    label: "Open workspace".to_owned(),
                    enabled: true,
                }],
            }],
        }
    }
}
