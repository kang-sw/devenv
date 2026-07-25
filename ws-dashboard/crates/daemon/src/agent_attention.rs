// Server-wide, work-root-independent SSE stream of per-terminal turn-state
// ("attention") transitions (260725 Phase 5,
// ai-docs/spec/ws-web-dashboard/index.md#260726-dashboard-terminal-attention-event-stream).
//
// CONTRACT: structurally mirrors `work_root_files::DocumentEventHub` (a
// `broadcast::Sender` wrapped in a small hub struct) but is NOT a drop-in
// copy of that hub's semantics in two load-bearing ways:
//
// 1. `DocumentEventHub` carries no snapshot state - document events are
//    point-in-time invalidations with a reread-on-focus fallback elsewhere.
//    `AttentionHub` DOES carry a snapshot (`entries`), because a browser
//    reconnect that missed a `working` -> `ready` transition while
//    disconnected would otherwise show a permanently stale indicator with no
//    other signal to correct it.
// 2. `document_events`'s SSE handler treats `RecvError::Lagged` as a
//    "silently skip forward, stream stays open" `continue` - safe there only
//    because document content is always re-readable on demand. `attention_events`
//    below instead ENDS the stream on `Lagged`, deliberately diverging: the
//    browser's native `EventSource` auto-reconnect re-enters this handler and
//    receives a fresh, complete snapshot, which is the resync mechanism - a
//    silent skip could leave a `ready` transition permanently unobserved
//    until an unrelated later event happens to arrive.
//
// This stream is server-wide (no `{work_root_id}` path segment) unlike every
// other SSE route in this crate: attention is keyed by `terminal_id` across
// the whole daemon, and the ticket's verification requirement ("reaches the
// client with no Activity Console pane open" for a non-selected workRoot)
// requires it to be selection-independent.

use std::collections::HashMap;
use std::convert::Infallible;
use std::sync::{Arc, RwLock};
use std::time::{SystemTime, UNIX_EPOCH};

use axum::extract::State;
use axum::response::sse::{Event, Sse};
use axum::response::{IntoResponse, Response};
use futures_util::{stream, StreamExt};
use serde::Serialize;
use tokio::sync::broadcast;
use ws_dashboard_core::WorkRootId;

use crate::agent_turn_state::TurnState;
use crate::router::AppState;

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AttentionEventView {
    #[serde(rename = "type")]
    pub event_type: String,
    pub terminal_id: String,
    pub work_root_id: WorkRootId,
    pub state: TurnState,
    pub updated_at_ms: u128,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct AttentionSnapshotView {
    items: Vec<AttentionEventView>,
}

#[derive(Clone, Debug)]
pub struct AttentionHub {
    tx: broadcast::Sender<AttentionEventView>,
    // CONTRACT: keyed by `terminal_id` - THE snapshot source for a fresh
    // connection (see `attention_events` below). Written only by
    // `record_and_publish` (turn-state POSTs) and `forget` (the
    // `TerminalRegistry` removal choke points, `terminal.rs::remove` /
    // `remove_for_work_roots` - NEVER called from a route handler directly).
    entries: Arc<RwLock<HashMap<String, AttentionEventView>>>,
}

impl Default for AttentionHub {
    fn default() -> Self {
        let (tx, _rx) = broadcast::channel(64);
        Self {
            tx,
            entries: Arc::new(RwLock::new(HashMap::new())),
        }
    }
}

impl AttentionHub {
    pub fn subscribe(&self) -> broadcast::Receiver<AttentionEventView> {
        self.tx.subscribe()
    }

    pub fn snapshot(&self) -> Vec<AttentionEventView> {
        self.entries
            .read()
            .expect("attention hub lock poisoned")
            .values()
            .cloned()
            .collect()
    }

    /// Records `terminal_id`'s new state in the snapshot map, then broadcasts
    /// it. Returns the built view so a caller (currently just
    /// `post_terminal_turn_state`) never needs to reconstruct it separately.
    pub fn record_and_publish(
        &self,
        terminal_id: String,
        work_root_id: WorkRootId,
        state: TurnState,
    ) -> AttentionEventView {
        let view = AttentionEventView {
            event_type: "terminal.attentionChanged".to_owned(),
            terminal_id: terminal_id.clone(),
            work_root_id,
            state,
            updated_at_ms: now_ms(),
        };
        self.entries
            .write()
            .expect("attention hub lock poisoned")
            .insert(terminal_id, view.clone());
        let _ = self.tx.send(view.clone());
        view
    }

    /// Removes `terminal_id`'s snapshot entry. Called ONLY from
    /// `TerminalRegistry`'s `remove`/`remove_for_work_roots` choke points -
    /// never from a route handler - so a closed terminal never lingers in a
    /// reconnect's snapshot.
    pub fn forget(&self, terminal_id: &str) {
        self.entries
            .write()
            .expect("attention hub lock poisoned")
            .remove(terminal_id);
    }
}

fn now_ms() -> u128 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis())
        .unwrap_or_default()
}

pub async fn attention_events(State(state): State<AppState>) -> Response {
    // CONTRACT (subscribe-before-snapshot ordering, load-bearing): registering
    // the broadcast receiver BEFORE reading the snapshot map closes the race
    // where a `record_and_publish` call lands between the two reads - if
    // snapshot were taken first, a write landing after the snapshot read but
    // before subscribe (i.e. before the sender registers this receiver) would
    // vanish from BOTH the snapshot and the live stream. With subscribe
    // first, that same write is guaranteed to already be visible to this
    // receiver's buffer even if the snapshot read (moments later) also missed
    // it - the client observes it twice (snapshot-omitted state topped up by
    // the very next `attention` frame) rather than never. A harmless
    // duplicate/stale-then-fresh transition beats a silent loss.
    let rx = state.attention.subscribe();
    let snapshot = state.attention.snapshot();

    let snapshot_payload = serde_json::to_string(&AttentionSnapshotView { items: snapshot })
        .unwrap_or_else(|_| "{\"items\":[]}".to_owned());
    let initial = stream::once(async move {
        Ok::<Event, Infallible>(Event::default().event("attentionSnapshot").data(snapshot_payload))
    });

    let live = stream::unfold(rx, |mut rx| async move {
        loop {
            match rx.recv().await {
                Ok(event) => {
                    let payload = serde_json::to_string(&event).unwrap_or_else(|_| "{}".to_owned());
                    return Some((
                        Ok::<Event, Infallible>(Event::default().event("attention").data(payload)),
                        rx,
                    ));
                }
                // Deliberate divergence from `document_events`'s `continue` -
                // see this module's own CONTRACT comment above for why
                // attention state ends the stream on lag instead of skipping
                // forward.
                Err(broadcast::error::RecvError::Lagged(_)) => return None,
                Err(broadcast::error::RecvError::Closed) => return None,
            }
        }
    });

    Sse::new(initial.chain(live)).into_response()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn work_root(id: &str) -> WorkRootId {
        WorkRootId::from(id.to_owned())
    }

    #[test]
    fn record_and_publish_populates_the_snapshot() {
        let hub = AttentionHub::default();
        hub.record_and_publish("term_a".to_owned(), work_root("root_a"), TurnState::Working);

        let snapshot = hub.snapshot();
        assert_eq!(snapshot.len(), 1, "snapshot must contain the recorded entry");
        assert_eq!(snapshot[0].terminal_id, "term_a");
        assert_eq!(snapshot[0].work_root_id, work_root("root_a"));
        assert_eq!(snapshot[0].state, TurnState::Working);
        assert_eq!(snapshot[0].event_type, "terminal.attentionChanged");
    }

    #[test]
    fn record_and_publish_overwrites_the_same_terminal_id() {
        let hub = AttentionHub::default();
        hub.record_and_publish("term_a".to_owned(), work_root("root_a"), TurnState::Working);
        hub.record_and_publish("term_a".to_owned(), work_root("root_a"), TurnState::Ready);

        let snapshot = hub.snapshot();
        assert_eq!(
            snapshot.len(),
            1,
            "a second transition for the same terminal must replace, not append"
        );
        assert_eq!(snapshot[0].state, TurnState::Ready);
    }

    #[test]
    fn forget_removes_the_snapshot_entry() {
        let hub = AttentionHub::default();
        hub.record_and_publish("term_a".to_owned(), work_root("root_a"), TurnState::Working);
        hub.forget("term_a");

        assert!(
            hub.snapshot().is_empty(),
            "forget must remove the entry so a reconnect never reports a closed terminal"
        );
    }

    #[test]
    fn forget_on_an_unknown_terminal_id_is_a_harmless_no_op() {
        let hub = AttentionHub::default();
        hub.forget("term_does_not_exist");
        assert!(hub.snapshot().is_empty());
    }

    #[tokio::test]
    async fn subscribe_receives_a_published_event() {
        let hub = AttentionHub::default();
        let mut rx = hub.subscribe();

        let published =
            hub.record_and_publish("term_a".to_owned(), work_root("root_a"), TurnState::Idle);

        let received = rx.recv().await.expect("subscriber receives the broadcast event");
        assert_eq!(received, published);
    }
}
