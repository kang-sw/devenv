use serde::{Deserialize, Serialize};

use crate::ResourcePath;

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstanceEventFixtures {
    pub transcripts: Vec<InstanceEventTranscript>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstanceEventTranscript {
    pub stream_id: String,
    pub resource_path: ResourcePath,
    pub events: Vec<InstanceEvent>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstanceEvent {
    pub stream_id: String,
    pub resource_path: ResourcePath,
    pub cursor: String,
    pub sequence: u64,
    pub timestamp: String,
    pub category: InstanceEventCategory,
    pub payload: InstanceEventPayload,
    pub terminal: bool,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum InstanceEventCategory {
    Output,
    Status,
    Error,
    End,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstanceEventPayload {
    pub text: Option<String>,
    pub status: Option<String>,
    pub message: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::OpaqueId;

    #[test]
    fn instance_event_envelope_serializes_dashboard_contract_names() {
        let transcript = InstanceEventTranscript {
            stream_id: "stream-main".to_owned(),
            resource_path: crate::ResourcePath {
                server_id: OpaqueId::from("server-local"),
                workspace_id: OpaqueId::from("workspace-devenv"),
                work_root_id: OpaqueId::from("root-devenv-primary"),
                instance_id: Some(OpaqueId::from("instance-devenv-main")),
            },
            events: vec![InstanceEvent {
                stream_id: "stream-main".to_owned(),
                resource_path: crate::ResourcePath {
                    server_id: OpaqueId::from("server-local"),
                    workspace_id: OpaqueId::from("workspace-devenv"),
                    work_root_id: OpaqueId::from("root-devenv-primary"),
                    instance_id: Some(OpaqueId::from("instance-devenv-main")),
                },
                cursor: "0000000001".to_owned(),
                sequence: 1,
                timestamp: "2026-05-16T00:00:00Z".to_owned(),
                category: InstanceEventCategory::Output,
                payload: InstanceEventPayload {
                    text: Some("hello".to_owned()),
                    ..InstanceEventPayload::default()
                },
                terminal: false,
            }],
        };

        let value = serde_json::to_value(transcript).expect("serialize transcript");

        assert_eq!(value["streamId"], "stream-main");
        assert_eq!(value["resourcePath"]["workRootId"], "root-devenv-primary");
        assert_eq!(value["events"][0]["streamId"], "stream-main");
        assert_eq!(
            value["events"][0]["resourcePath"]["instanceId"],
            "instance-devenv-main"
        );
        assert_eq!(value["events"][0]["cursor"], "0000000001");
        assert_eq!(value["events"][0]["category"], "output");
        assert_eq!(value["events"][0]["payload"]["text"], "hello");
        assert!(value.get("stream_id").is_none());
        assert!(value["resourcePath"].get("work_root_id").is_none());
    }
}
