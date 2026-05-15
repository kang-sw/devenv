use std::collections::HashMap;

use axum::extract::{Path, Query};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::Serialize;
use ws_dashboard_core::{InstanceEvent, InstanceEventFixtures, InstanceEventTranscript};

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstanceEventStreamResponse {
    pub stream_id: String,
    pub events: Vec<InstanceEvent>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct InstanceEventStreamError {
    error: String,
}

#[derive(Clone, Debug, Default)]
pub struct MockInstanceEventTranscriptProvider;

impl MockInstanceEventTranscriptProvider {
    pub fn fixtures(&self) -> InstanceEventFixtures {
        serde_json::from_str(include_str!("../tests/fixtures/instance_events.json"))
            .expect("instance event fixture is valid")
    }

    pub fn transcript(&self, stream_id: &str) -> Option<InstanceEventTranscript> {
        self.fixtures()
            .transcripts
            .into_iter()
            .find(|transcript| transcript.stream_id == stream_id)
    }

    pub fn events_after(&self, stream_id: &str, after: Option<&str>) -> Option<Vec<InstanceEvent>> {
        let transcript = self.transcript(stream_id)?;
        let after_sequence = match after {
            Some(cursor) => match transcript
                .events
                .iter()
                .find(|event| event.cursor == cursor)
            {
                Some(event) => Some(event.sequence),
                None => return Some(Vec::new()),
            },
            None => None,
        };

        Some(
            transcript
                .events
                .into_iter()
                .filter(|event| {
                    after_sequence
                        .map(|sequence| event.sequence > sequence)
                        .unwrap_or(true)
                })
                .collect(),
        )
    }
}

pub async fn instance_events(
    Path(stream_id): Path<String>,
    Query(query): Query<HashMap<String, String>>,
) -> Response {
    let provider = MockInstanceEventTranscriptProvider;
    let after = query.get("after").map(String::as_str);
    let Some(events) = provider.events_after(&stream_id, after) else {
        return (
            StatusCode::NOT_FOUND,
            Json(InstanceEventStreamError {
                error: "stream not found".to_owned(),
            }),
        )
            .into_response();
    };

    Json(InstanceEventStreamResponse { stream_id, events }).into_response()
}

#[cfg(test)]
mod tests {
    use super::*;
    use ws_dashboard_core::InstanceEventCategory;

    #[test]
    fn fixture_covers_output_status_error_end_and_empty_streams() {
        let fixtures = MockInstanceEventTranscriptProvider::default().fixtures();

        assert_eq!(fixtures.transcripts.len(), 2);
        let main = fixtures
            .transcripts
            .iter()
            .find(|transcript| transcript.stream_id == "stream-devenv-main")
            .expect("main transcript");
        assert_eq!(
            main.resource_path.work_root_id.as_str(),
            "root-devenv-primary"
        );
        assert_eq!(
            main.resource_path.instance_id.as_ref().unwrap().as_str(),
            "instance-devenv-main"
        );

        let categories: Vec<_> = main.events.iter().map(|event| event.category).collect();
        assert!(categories.contains(&InstanceEventCategory::Output));
        assert!(categories.contains(&InstanceEventCategory::Status));
        assert!(categories.contains(&InstanceEventCategory::Error));
        assert!(categories.contains(&InstanceEventCategory::End));
        assert!(main.events.iter().any(|event| event.terminal));

        let empty = fixtures
            .transcripts
            .iter()
            .find(|transcript| transcript.stream_id == "stream-empty")
            .expect("empty transcript");
        assert!(empty.events.is_empty());
    }

    #[test]
    fn fixture_provider_supports_cursor_backfill() {
        let provider = MockInstanceEventTranscriptProvider;

        let all = provider
            .events_after("stream-devenv-main", None)
            .expect("all events");
        let after_second = provider
            .events_after("stream-devenv-main", Some("0000000002"))
            .expect("events after second cursor");

        assert_eq!(all.len(), 5);
        assert_eq!(after_second.len(), 3);
        assert_eq!(after_second[0].cursor, "0000000003");
        assert_eq!(
            after_second[0]
                .resource_path
                .instance_id
                .as_ref()
                .unwrap()
                .as_str(),
            "instance-devenv-main"
        );
        assert!(provider
            .events_after("stream-devenv-main", Some("does-not-exist"))
            .expect("unknown cursor returns empty backfill")
            .is_empty());
        assert!(provider.events_after("missing", None).is_none());
    }
}
