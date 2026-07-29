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
    ///
    /// CONTRACT (260725 Phase 5 review cycle 1, finding B): the map write and
    /// the broadcast send happen under ONE held write-guard - never take the
    /// guard, write, drop it, and THEN send. Dropping the guard before
    /// sending would let two concurrent calls for the SAME `terminal_id`
    /// interleave as "A inserts, B inserts, B sends, A sends", which
    /// broadcasts in the opposite order from the map write and pins an
    /// already-connected subscriber on a stale state indefinitely (the
    /// permanently-stuck-spinner symptom this whole feature exists to
    /// prevent). Sampling `now_ms()` after the guard is taken (rather than
    /// before, as a prior version of this function did) is part of the same
    /// fix: a timestamp sampled before the lock could itself be inverted
    /// relative to lock/send order, so a client could not even fall back to
    /// ordering by `updated_at_ms`. Nothing between taking the guard and
    /// sending ever awaits, so holding it across `self.tx.send(...)` cannot
    /// span a suspension point or block another async task's executor.
    pub fn record_and_publish(
        &self,
        terminal_id: String,
        work_root_id: WorkRootId,
        state: TurnState,
    ) -> AttentionEventView {
        let mut entries = self.entries.write().expect("attention hub lock poisoned");
        let view = AttentionEventView {
            event_type: "terminal.attentionChanged".to_owned(),
            terminal_id: terminal_id.clone(),
            work_root_id,
            state,
            updated_at_ms: now_ms(),
        };
        entries.insert(terminal_id, view.clone());
        let _ = self.tx.send(view.clone());
        drop(entries);
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
        // Deliberately NOT a `loop`: every arm below is terminal, so a
        // wrapping loop would be dead (`clippy::never_loop`, deny-by-default)
        // and would also mislead a reader into expecting a skip-and-retry
        // arm this stream does not have.
        match rx.recv().await {
            Ok(event) => {
                let payload = serde_json::to_string(&event).unwrap_or_else(|_| "{}".to_owned());
                Some((
                    Ok::<Event, Infallible>(Event::default().event("attention").data(payload)),
                    rx,
                ))
            }
            // Deliberate divergence from `document_events`'s `continue` -
            // see this module's own CONTRACT comment above for why
            // attention state ends the stream on lag instead of skipping
            // forward.
            Err(broadcast::error::RecvError::Lagged(_)) => None,
            Err(broadcast::error::RecvError::Closed) => None,
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

    // CONTRACT (260725 Phase 5 review cycle 1, finding B): a true OS-thread
    // race that lands exactly in the (pre-fix) gap between "drop the entries
    // write lock" and "send on the broadcast channel" is a few-instructions
    // wide and not reliably forceable without an artificial delay hook this
    // module deliberately does not add to production code (that would be
    // its own scope creep). Instead this test hammers `record_and_publish`
    // from many threads for the SAME `terminal_id` and asserts an invariant
    // that holds BY CONSTRUCTION once map-write and send share one critical
    // section: whichever call is serialized last is definitionally both the
    // map's final value AND the last value observed on the broadcast
    // channel, for every possible interleaving the scheduler picks - so the
    // assertion cannot pass "by luck" the way a single fixed-timing
    // reproduction could. Reverting the fix (dropping the guard before the
    // send) reopens the gap this stresses; see the implementer report for
    // the actual mutation-testing run and failure message.
    #[test]
    fn record_and_publish_keeps_broadcast_order_consistent_with_the_map_under_concurrent_writers() {
        use std::sync::Arc;
        use std::thread;

        let hub = Arc::new(AttentionHub::default());
        let mut rx = hub.subscribe();
        let terminal_id = "term_race".to_owned();
        let root = work_root("root_race");

        let handles: Vec<_> = (0..8u32)
            .map(|i| {
                let hub = hub.clone();
                let terminal_id = terminal_id.clone();
                let root = root.clone();
                thread::spawn(move || {
                    for j in 0..50u32 {
                        let state = if (i + j) % 2 == 0 {
                            TurnState::Working
                        } else {
                            TurnState::Ready
                        };
                        hub.record_and_publish(terminal_id.clone(), root.clone(), state);
                    }
                })
            })
            .collect();
        for handle in handles {
            handle.join().expect("writer thread must not panic");
        }

        let snapshot = hub.snapshot();
        assert_eq!(snapshot.len(), 1, "all 8 threads write the same terminal_id");
        let final_state = snapshot[0].state;

        // Drain every frame the broadcast channel still has buffered,
        // skipping over any lag (400 sends against a 64-capacity channel
        // will lag) rather than stopping at the first gap - we only care
        // about the LAST value the receiver can observe, which lag-skipping
        // does not change.
        let mut last_received = None;
        loop {
            match rx.try_recv() {
                Ok(event) => last_received = Some(event.state),
                Err(broadcast::error::TryRecvError::Lagged(_)) => continue,
                Err(_) => break,
            }
        }

        assert_eq!(
            last_received,
            Some(final_state),
            "the last broadcast frame must match the map's final state - a stale `working` \
             while the snapshot already says `ready` is exactly the permanently-stuck-spinner \
             symptom finding B describes"
        );
    }
}
