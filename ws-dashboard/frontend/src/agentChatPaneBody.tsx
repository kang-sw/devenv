import { useEffect, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { Bot, History } from "lucide-react";
import { useDismissableMenu } from "./dismissableMenu.js";
import {
  beginRealStreamingTurn,
  realAgentChatHarness,
  steerActivitySession as realSteerActivitySession,
  type RealStreamingHandle,
} from "./activitySessionClient.js";
import {
  agentChatHarnessLabel,
  stubBeginStreamingTurn,
  stubSteerActivitySession,
  type StubStreamingHandle,
} from "./activitySessionStub.js";
import {
  agentChatHarnesses,
  type AgentChatHarness,
  type AgentChatPaneState,
} from "./agentChatSessions.js";
import { mergeStreamingTranscriptBlocks } from "./agentChatStreamMerge.js";
import { AgentChatTranscriptBubbles, type ChatBubble } from "./agentChatBubbles.js";
import type { ActivityItem, TranscriptBlock } from "./workRootActivity.js";

export type AgentChatPaneActions = {
  onClose: (pane: AgentChatPaneState) => void;
  onStartHarness: (pane: AgentChatPaneState, harness: AgentChatHarness) => void;
  onResumeHistoryItem: (pane: AgentChatPaneState, item: ActivityItem) => void;
  onLoadHistory: (
    workRootId: string,
    serverRoute: string | null | undefined,
  ) => Promise<{ items: ActivityItem[] }>;
  // Phase 3 (`260711-feat-ws-dashboard-agent-activity-chat-ui`): base send
  // input, "fork from here" (shipped live), and the scaffold-only "resume
  // from here" no-op (never invoked - `isResumeFromHereEnabled` always
  // returns `false`, so `AgentChatPaneBody` never wires a real click
  // through to it; kept here only so the action surface documents the
  // isolation seam the ticket requires).
  onSendMessage: (pane: AgentChatPaneState, text: string) => void;
  onForkFromBubble: (pane: AgentChatPaneState, bubble: ChatBubble) => void;
  onResumeFromBubble: (pane: AgentChatPaneState, bubble: ChatBubble) => void;
  // `260720-bug-dashboard-fork-from-here-cutcursor-resolution` Phase 1:
  // called with each real transcript-poll delta (`beginRealStreamingTurn`'s
  // `onUpdate`) so the canonical session's still-optimistic `user-sent-...`
  // blocks get reconciled to daemon-confirmed cursors as they arrive - see
  // `reconcileAgentChatTranscript` / `reconcileOptimisticUserCursors`.
  onReconcileTranscript: (
    pane: AgentChatPaneState,
    blocks: readonly TranscriptBlock[],
  ) => void;
  isActivePane: (pane: AgentChatPaneState) => boolean;
};

// Phase 3 (`260711-feat-ws-dashboard-agent-activity-chat-ui`) mid-turn
// queuing state: a message accepted while a turn is already in flight is
// rendered as its own pending bubble with a badge, cleared once the FIFO
// queue dequeues and actually sends it (see `AgentChatPaneBody`'s
// `beginSimulatedTurn`/`onComplete`).
type PendingChatMessage = {
  readonly id: string;
  readonly text: string;
  readonly steering: boolean;
};

export function AgentChatPaneBody({
  pane,
  actions,
}: {
  pane: AgentChatPaneState;
  actions: AgentChatPaneActions;
}) {
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyItems, setHistoryItems] = useState<ActivityItem[]>([]);
  const [historyLoading, setHistoryLoading] = useState(false);
  const historyRef = useRef<HTMLDivElement | null>(null);
  useDismissableMenu(historyOpen, historyRef, () => setHistoryOpen(false));

  // CONTRACT: `beginSimulatedTurn`'s `onComplete` callback (Phase 3) can
  // fire long after the render that created it - e.g. `pane` reflects the
  // moment "first message" was sent, but by the time that turn's stub
  // stream completes several seconds later, `pane.session` has moved on.
  // A queued dequeue's `beginSimulatedTurn(next.text)` re-invocation must
  // send against the *current* session, not the stale one closed over at
  // the original call site (Dockview's `contentRevision`-gated panel param
  // push means `AgentChatPaneBody` does not necessarily re-render between
  // every transcript change either - see `agentChatWorkbenchPane`'s
  // `contentRevision` comment). `paneRef` is updated on every render and
  // read instead of the closed-over `pane` inside every handler below.
  const paneRef = useRef(pane);
  paneRef.current = pane;

  // Stub-side per-line streaming demo (`260711` Phase 2): once a session is
  // active, grow one synthetic agent-turn block over several ticks so the
  // bubble transcript renders incrementally, mirroring what a live streaming
  // harness backend would eventually push. Purely additive to the session's
  // own `transcript.blocks` - never mutates them.
  const [streamingBlocks, setStreamingBlocks] = useState<Record<string, TranscriptBlock>>({});
  const activeActivityId = pane.session?.activityId ?? null;
  useEffect(() => {
    if (!activeActivityId) {
      return;
    }
    setStreamingBlocks({});
    // Real harnesses (Codex/Claude) already hydrate their transcript via
    // `startNewAgentChatSession`/`resumeAgentChatSession` before the session
    // reaches this pane, so there is no canned "session started" demo
    // monologue to play for them - only the stub-backed harnesses (OpenCode)
    // get the Phase 2 demo intro.
    const sessionHarness = pane.session?.harness;
    if (sessionHarness && realAgentChatHarness(sessionHarness)) {
      return;
    }
    const handle = stubBeginStreamingTurn((block) => {
      setStreamingBlocks((current) => ({ ...current, [block.cursor]: block }));
    });
    return () => handle.stop();
  }, [activeActivityId]);

  // Phase 3 mid-turn queuing / base send-input state. Local to this
  // pane-body instance - `AgentChatPaneBody` is already re-mounted per pane
  // (`key={pane.paneId}` at its call site in `agentChatWorkbenchPane`), so a
  // plain per-instance `useState`/`useRef` gives the same per-pane isolation
  // a `Record<string, ...>` keyed by `pane.paneId` would, mirroring how
  // `streamingBlocks` above is already pane-body-local rather than lifted
  // into `AgentChatPaneState`.
  const [turnInFlight, setTurnInFlight] = useState(false);
  const [pendingMessages, setPendingMessages] = useState<PendingChatMessage[]>([]);
  const pendingRef = useRef<PendingChatMessage[]>([]);
  // Phase 2 (260713-fix usability-polish): net-new auto-scroll for the
  // newest pending/queued bubble - there is no other scroll-management
  // precedent in this transcript container to follow. `lastPendingBubbleRef`
  // tracks the DOM node of only the most-recently-queued bubble;
  // `pendingCountRef` tracks the previous `pendingMessages.length` so the
  // scroll effect below can tell a queue-growth (new message queued) apart
  // from a queue-shrink (dequeue/revert), which must not re-trigger a scroll.
  const lastPendingBubbleRef = useRef<HTMLDivElement | null>(null);
  const pendingCountRef = useRef(0);
  const turnSequenceRef = useRef(0);
  const pendingSequenceRef = useRef(0);
  const streamHandlesRef = useRef<Array<StubStreamingHandle | RealStreamingHandle>>([]);
  const [promptValue, setPromptValue] = useState("");
  const [promptHistory, setPromptHistory] = useState<string[]>([]);
  const [historyIndex, setHistoryIndex] = useState<number | null>(null);

  useEffect(() => {
    return () => {
      for (const handle of streamHandlesRef.current) {
        handle.stop();
      }
    };
  }, []);

  // A history-entry resume can swap in a different `activityId` under the
  // same pane/tab; the mid-turn queue and prompt history are scoped to the
  // conversation currently showing, not the tab, so they reset alongside
  // the streaming-demo state above.
  useEffect(() => {
    setTurnInFlight(false);
    pendingRef.current = [];
    setPendingMessages([]);
    setPromptValue("");
    setPromptHistory([]);
    setHistoryIndex(null);
  }, [activeActivityId]);

  // Scroll the newest queued/pending bubble into view when it is added
  // (Phase 2, 260713-fix usability-polish) - on a tall transcript the
  // pending bubble can otherwise land below the fold with no visible
  // confirmation that the message was queued. Only fires on queue growth
  // (a new message just queued via `submitPrompt`'s mid-turn branch), not
  // on queue shrink (dequeue via `onTurnComplete` or `revertPending`), so
  // this stays a "new pending bubble appeared" signal rather than firing on
  // every `pendingMessages` reference change.
  useEffect(() => {
    if (pendingMessages.length > pendingCountRef.current) {
      lastPendingBubbleRef.current?.scrollIntoView({ block: "nearest" });
    }
    pendingCountRef.current = pendingMessages.length;
  }, [pendingMessages]);

  // The concrete "turn-in-flight / batch-boundary" mechanism (Phase 3
  // Escalation 1): starts a real send (`actions.onSendMessage`, which
  // appends the real transcript block) plus a distinct-cursor/turnId stub
  // stream, and on that stream's natural `onComplete` clears in-flight and
  // dequeues+re-sends the next queued message (if any) - this is what
  // visibly clears a pending badge and starts its own new streamed reply.
  function beginSimulatedTurn(text: string, options?: { readonly alreadyDelivered?: boolean }) {
    const currentPane = paneRef.current;
    if (!currentPane.session) {
      return;
    }
    // `actions.onSendMessage` (-> `sendAgentChatMessage`) already fires the
    // real `/prompt` POST for Codex/Claude sessions
    // (`260713-feat-ws-dashboard-agent-chat-real-adapter-wiring` Phase 1) -
    // do not send it again here, only fetch the resulting transcript once.
    // `options.alreadyDelivered` is set when this turn's text was already
    // delivered to the real Codex process via `turn/steer` while it was
    // queued (see `submitPrompt`'s `steering` branch): resending it here via
    // `/prompt` would double-deliver the same text, so the send is skipped
    // and only the resulting transcript is fetched.
    if (!options?.alreadyDelivered) {
      actions.onSendMessage(currentPane, text);
    }
    setPromptHistory((current) => [...current, text]);
    setTurnInFlight(true);
    turnSequenceRef.current += 1;
    const cursor = `user-turn-${turnSequenceRef.current}`;
    const onTurnComplete = () => {
      setTurnInFlight(false);
      const queue = pendingRef.current;
      if (queue.length > 0) {
        const [next, ...rest] = queue;
        pendingRef.current = rest;
        setPendingMessages(rest);
        // `next.steering` is only ever true for a Codex session (the only
        // harness whose capability table enables `steer`), and only when the
        // real `turn/steer` control call was actually fired in `submitPrompt`
        // - so a dequeued steering entry must not be re-sent via `/prompt`.
        beginSimulatedTurn(next.text, { alreadyDelivered: next.steering });
      }
    };
    const realHarness = realAgentChatHarness(currentPane.session.harness);
    if (realHarness) {
      const { workRootId, activityId, serverRoute } = currentPane.session;
      const handle = beginRealStreamingTurn(
        workRootId,
        realHarness,
        activityId,
        serverRoute,
        (blocks) => {
          setStreamingBlocks((current) => {
            const next = { ...current };
            for (const block of blocks) {
              next[block.cursor] = block;
            }
            return next;
          });
          // `260720-bug-dashboard-fork-from-here-cutcursor-resolution`
          // Phase 1: reconcile the canonical session's optimistic
          // `user-sent-...` blocks against this poll's daemon-confirmed
          // blocks, so a later "fork from here" click on a live user bubble
          // sends a cursor the daemon can actually resolve.
          actions.onReconcileTranscript(paneRef.current, blocks);
        },
        onTurnComplete,
      );
      streamHandlesRef.current.push(handle);
      return;
    }
    const handle = stubBeginStreamingTurn(
      (block) => {
        setStreamingBlocks((current) => ({ ...current, [block.cursor]: block }));
      },
      { cursor, turnId: cursor, onComplete: onTurnComplete },
    );
    streamHandlesRef.current.push(handle);
  }

  function revertPending(id: string) {
    const entry = pendingRef.current.find((candidate) => candidate.id === id);
    if (!entry) {
      return;
    }
    pendingRef.current = pendingRef.current.filter((candidate) => candidate.id !== id);
    setPendingMessages(pendingRef.current);
    setPromptValue(entry.text);
    setHistoryIndex(null);
  }

  function submitPrompt(rawText: string) {
    const text = rawText.trim();
    const currentPane = paneRef.current;
    if (!text || !currentPane.session) {
      return;
    }
    setPromptValue("");
    setHistoryIndex(null);
    if (!turnInFlight) {
      beginSimulatedTurn(text);
      return;
    }
    // Already mid-turn: queue locally (FIFO), rendered as a pending bubble
    // until the in-flight turn's `onComplete` dequeues it. If the harness
    // reports `steer: true` (Codex only, per the capability table), also
    // fire the real `turn/steer` control call for Codex
    // (`260713-feat-ws-dashboard-agent-chat-real-adapter-wiring` Phase 1) -
    // or the stub no-op for any other (unadapted) harness - fire-and-forget.
    // Actual delivery timing either way is still governed by the same local
    // FIFO/`onComplete` above, since no real duplex exists yet.
    pendingSequenceRef.current += 1;
    const steering = currentPane.session.capabilities.steer;
    const entry: PendingChatMessage = {
      id: `pending-${Date.now().toString(36)}-${pendingSequenceRef.current}`,
      text,
      steering,
    };
    pendingRef.current = [...pendingRef.current, entry];
    setPendingMessages(pendingRef.current);
    if (steering) {
      const realHarness = realAgentChatHarness(currentPane.session.harness);
      if (realHarness === "codex") {
        void realSteerActivitySession(
          currentPane.session.workRootId,
          currentPane.session.activityId,
          text,
          currentPane.session.serverRoute,
        );
      } else {
        void stubSteerActivitySession({
          workRootId: currentPane.session.workRootId,
          activityId: currentPane.session.activityId,
          text,
          serverRoute: currentPane.session.serverRoute,
        });
      }
    }
  }

  // Prompt-box up/down history traversal (Phase 3, no existing in-browser
  // precedent - see the plan's Codebase Findings). Only intercepts the
  // arrow key when the caret carries no selection and sits at the very
  // start of the field (standard REPL convention), so normal in-text
  // cursor movement is left alone. Landing on a value that still matches a
  // live pending entry performs the same revert as the explicit revert
  // button (pulls it back into the input and cancels its queued send).
  function navigateHistory(direction: -1 | 1) {
    if (promptHistory.length === 0) {
      return;
    }
    let nextIndex: number;
    if (direction === -1) {
      nextIndex = historyIndex === null ? promptHistory.length - 1 : Math.max(0, historyIndex - 1);
    } else {
      if (historyIndex === null) {
        return;
      }
      nextIndex = historyIndex + 1;
      if (nextIndex >= promptHistory.length) {
        setHistoryIndex(null);
        setPromptValue("");
        return;
      }
    }
    setHistoryIndex(nextIndex);
    const text = promptHistory[nextIndex]!;
    const stillPending = pendingRef.current.find((entry) => entry.text === text);
    if (stillPending) {
      revertPending(stillPending.id);
      return;
    }
    setPromptValue(text);
  }

  function handlePromptKeyDown(event: ReactKeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      submitPrompt(promptValue);
      return;
    }
    if (event.key !== "ArrowUp" && event.key !== "ArrowDown") {
      return;
    }
    const input = event.currentTarget;
    const caretAtStart = input.selectionStart === 0 && input.selectionEnd === 0;
    if (!caretAtStart) {
      return;
    }
    event.preventDefault();
    navigateHistory(event.key === "ArrowUp" ? -1 : 1);
    // navigateHistory only queues a setPromptValue (or revertPending, which
    // also calls setPromptValue) React state update; the DOM value/selection
    // has not landed yet when this handler returns, so setSelectionRange
    // here would operate on stale content. Defer to the next paint via
    // requestAnimationFrame so the reset applies after React commits the
    // recalled text. Reset to column 0 (not select-all) so the very next
    // ArrowUp/ArrowDown still satisfies the caretAtStart guard above,
    // letting repeated presses keep walking through history.
    requestAnimationFrame(() => {
      input.setSelectionRange(0, 0);
    });
  }

  if (pane.session) {
    const { session } = pane;
    const transcriptBlocks = mergeStreamingTranscriptBlocks(
      session.transcript.blocks,
      streamingBlocks,
    );
    return (
      <div className="agent-chat-pane" data-agent-chat-pane-state="active">
        <div className="agent-chat-pane-header">
          <span className="agent-chat-pane-harness">
            {agentChatHarnessLabel[session.harness]}
          </span>
          <span className="agent-chat-pane-title">{session.title}</span>
        </div>
        <div className="agent-chat-pane-transcript" data-testid="agent-chat-transcript">
          <AgentChatTranscriptBubbles
            blocks={transcriptBlocks}
            sourceKind={session.transcript.source.kind}
            capabilities={session.capabilities}
            onForkFromBubble={(bubble) => actions.onForkFromBubble(pane, bubble)}
            onResumeFromBubble={(bubble) => actions.onResumeFromBubble(pane, bubble)}
          />
          {pendingMessages.map((entry, index) => (
            <div
              className="agent-chat-bubble agent-chat-bubble-user agent-chat-bubble-pending"
              data-agent-chat-bubble-kind="user"
              data-agent-chat-bubble-align="right"
              data-testid="agent-chat-pending-bubble"
              key={entry.id}
              ref={index === pendingMessages.length - 1 ? lastPendingBubbleRef : undefined}
            >
              <div className="agent-chat-bubble-body">{entry.text}</div>
              <div className="agent-chat-bubble-actions">
                <span className="agent-chat-pending-badge" data-testid="agent-chat-pending-badge">
                  {entry.steering ? "steering…" : "queued for next turn"}
                </span>
                <button
                  type="button"
                  className="agent-chat-bubble-revert"
                  data-command-id="agentChat.pending.revert"
                  data-pending-id={entry.id}
                  onClick={() => revertPending(entry.id)}
                >
                  되돌리기
                </button>
              </div>
            </div>
          ))}
        </div>
        <div className="agent-chat-prompt-box">
          <input
            className="agent-chat-prompt-input"
            data-testid="agent-chat-prompt-input"
            type="text"
            placeholder="Send a message…"
            value={promptValue}
            onChange={(event) => {
              setPromptValue(event.target.value);
              setHistoryIndex(null);
            }}
            onKeyDown={handlePromptKeyDown}
          />
          <button
            type="button"
            className="agent-chat-prompt-send"
            data-command-id="agentChat.prompt.send"
            disabled={promptValue.trim().length === 0}
            onClick={() => submitPrompt(promptValue)}
          >
            Send
          </button>
        </div>
      </div>
    );
  }

  const openHistory = () => {
    setHistoryOpen(true);
    setHistoryLoading(true);
    void actions
      .onLoadHistory(pane.workRootId, pane.serverRoute)
      .then((response) => setHistoryItems(response.items))
      .catch(() => setHistoryItems([]))
      .finally(() => setHistoryLoading(false));
  };

  return (
    <div className="agent-chat-pane" data-agent-chat-pane-state="empty">
      <div className="agent-chat-pane-topbar" ref={historyRef}>
        <button
          className="agent-chat-resume-control"
          data-command-id="agentChat.history.open"
          type="button"
          onClick={openHistory}
        >
          <History aria-hidden="true" size={15} strokeWidth={1.8} />
          resume a past conversation
        </button>
        {historyOpen ? (
          <div
            className="agent-chat-history-popover"
            data-testid="agent-chat-history-popover"
            role="dialog"
          >
            {historyLoading ? (
              <div className="agent-chat-history-empty">Loading…</div>
            ) : historyItems.length === 0 ? (
              <div className="agent-chat-history-empty">
                No past conversations for this work root.
              </div>
            ) : (
              <ul className="agent-chat-history-list">
                {historyItems.map((item) => (
                  <li key={item.id}>
                    <button
                      className="agent-chat-history-item"
                      data-history-item-id={item.id}
                      type="button"
                      onClick={() => {
                        setHistoryOpen(false);
                        actions.onResumeHistoryItem(pane, item);
                      }}
                    >
                      <span className="agent-chat-history-item-harness">
                        {item.source.harness ?? item.kind}
                      </span>
                      <span className="agent-chat-history-item-title">
                        {item.label}
                      </span>
                      <span className="agent-chat-history-item-meta">
                        {item.updatedAt ?? ""}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        ) : null}
      </div>
      <div className="agent-chat-tiles" data-testid="agent-chat-tiles">
        {agentChatHarnesses.map((harness) => (
          <button
            className="agent-chat-tile"
            data-agent-chat-tile={harness}
            disabled={pane.starting}
            key={harness}
            type="button"
            onClick={() => actions.onStartHarness(pane, harness)}
          >
            <Bot aria-hidden="true" size={22} strokeWidth={1.6} />
            <span>{agentChatHarnessLabel[harness]}</span>
          </button>
        ))}
      </div>
      {pane.error ? (
        <div className="agent-chat-pane-error" role="alert">
          {pane.error}
        </div>
      ) : null}
    </div>
  );
}
