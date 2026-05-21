import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  buildActivityDetailToggleCommand,
  buildActivityRefreshCommand,
  buildActivitySelectItemCommand,
  buildActivityTranscriptLoadMoreCommand,
  type DashboardCommandDispatcher,
} from "./commands";
import {
  acknowledgeActivityItem,
  defaultActivitySelection,
  fetchWorkRootActivityTranscript,
  initializeActivityDirtyItems,
  orderActivityItems,
  preserveActivitySelection,
  shouldApplyActivityTranscriptRequest,
  shouldLoadMoreActivityTranscript,
  transcriptBlockView,
  type ActivityAcknowledgements,
  type ActivityItem,
  type ActivityTranscript,
  type ActivityTranscriptFetchOptions,
  type TranscriptBlock,
  type WorkRootActivityView,
} from "./workRootActivity";

export type ActivityTranscriptLoader = (
  workRootId: string,
  activityId: string,
  options?: ActivityTranscriptFetchOptions,
) => Promise<ActivityTranscript>;

type ActivityTranscriptLoadState =
  | { phase: "idle"; transcript: null; error: null; loadingMore: false }
  | {
      phase: "loading";
      transcript: ActivityTranscript | null;
      error: null;
      loadingMore: false;
    }
  | {
      phase: "ready";
      transcript: ActivityTranscript;
      error: null;
      loadingMore: boolean;
    }
  | {
      phase: "error";
      transcript: ActivityTranscript | null;
      error: string;
      loadingMore: false;
    };

type TranscriptRequestKey = {
  workRootId: string;
  activityId: string | null;
  requestId: number;
};

export function ActivityConsole({
  view,
  onCommand,
  loadTranscript = fetchWorkRootActivityTranscript,
}: {
  view: WorkRootActivityView;
  onCommand: DashboardCommandDispatcher;
  loadTranscript?: ActivityTranscriptLoader;
}) {
  const orderedItems = useMemo(
    () => orderActivityItems(view.items),
    [view.items],
  );
  const [selectedItemId, setSelectedItemId] = useState<string | null>(() =>
    view.selectedItemId &&
    view.items.some((item) => item.id === view.selectedItemId)
      ? view.selectedItemId
      : defaultActivitySelection(view.items),
  );
  const [acknowledgements, setAcknowledgements] =
    useState<ActivityAcknowledgements>({});
  const [seenRevisions, setSeenRevisions] = useState<ActivityAcknowledgements>(
    {},
  );
  const dirtyItems = useMemo(
    () =>
      initializeActivityDirtyItems(view.items, acknowledgements, seenRevisions),
    [view.items, acknowledgements, seenRevisions],
  );
  const [expandedDetails, setExpandedDetails] = useState<Set<string>>(
    () => new Set(),
  );
  const [transcriptState, setTranscriptState] =
    useState<ActivityTranscriptLoadState>({
      phase: "idle",
      transcript: null,
      error: null,
      loadingMore: false,
    });
  const transcriptRequestSeq = useRef(0);
  const currentTranscriptRequest = useRef<TranscriptRequestKey>({
    workRootId: view.workRootId,
    activityId: selectedItemId,
    requestId: 0,
  });

  useEffect(() => {
    setAcknowledgements({});
    setSeenRevisions({});
    setExpandedDetails(new Set());
    setTranscriptState({
      phase: "idle",
      transcript: null,
      error: null,
      loadingMore: false,
    });
  }, [view.workRootId]);

  useEffect(() => {
    setSelectedItemId((current) =>
      preserveActivitySelection(
        view.items,
        current ?? view.selectedItemId ?? null,
      ),
    );
    setSeenRevisions((current) => {
      const next = { ...current };
      for (const item of view.items) {
        if (!Object.prototype.hasOwnProperty.call(next, item.id)) {
          next[item.id] =
            item.updatedAt ?? item.transcript.cursor ?? item.status;
        }
      }
      return next;
    });
  }, [view.workRootId, view.items, view.selectedItemId]);

  const selectedItem =
    orderedItems.find((item) => item.id === selectedItemId) ?? null;
  const selectedRevision = selectedItem
    ? (selectedItem.updatedAt ??
      selectedItem.transcript.cursor ??
      selectedItem.status)
    : null;

  const acknowledgeSelected = useCallback((item: ActivityItem) => {
    setAcknowledgements((current) => acknowledgeActivityItem(current, item));
    setSeenRevisions((current) => acknowledgeActivityItem(current, item));
  }, []);

  const requestTranscript = useCallback(
    (mode: "replace" | "append") => {
      const item = selectedItem;
      if (!item || !item.transcript.available) {
        currentTranscriptRequest.current = {
          workRootId: view.workRootId,
          activityId: item?.id ?? null,
          requestId: transcriptRequestSeq.current,
        };
        if (!item) {
          setTranscriptState({
            phase: "idle",
            transcript: null,
            error: null,
            loadingMore: false,
          });
        } else {
          setTranscriptState({
            phase: "ready",
            transcript: {
              workRootId: view.workRootId,
              activityId: item.id,
              status: item.transcript.status,
              sourceStatus: item.transcript.status,
              live: item.live,
              source: item.source,
              blocks: [],
              nextCursor: null,
              hasMore: false,
              diagnostics: item.diagnostics,
            },
            error: null,
            loadingMore: false,
          });
        }
        return;
      }
      const cursor =
        mode === "append" && transcriptState.phase === "ready"
          ? transcriptState.transcript.nextCursor
          : undefined;
      if (mode === "append" && !cursor) {
        return;
      }
      const requestId = transcriptRequestSeq.current + 1;
      transcriptRequestSeq.current = requestId;
      const expected = {
        workRootId: view.workRootId,
        activityId: item.id,
        requestId,
      };
      currentTranscriptRequest.current = expected;
      setTranscriptState((current) =>
        mode === "append" && current.phase === "ready"
          ? { ...current, loadingMore: true }
          : {
              phase: "loading",
              transcript: null,
              error: null,
              loadingMore: false,
            },
      );
      void loadTranscript(view.workRootId, item.id, {
        cursor: cursor ?? undefined,
        limit: 40,
      })
        .then((transcript) => {
          if (
            !shouldApplyActivityTranscriptRequest(
              expected,
              currentTranscriptRequest.current,
              transcript,
            )
          ) {
            return;
          }
          setTranscriptState((current) => {
            if (mode === "append" && current.phase === "ready") {
              return {
                phase: "ready",
                transcript: {
                  ...transcript,
                  blocks: [...current.transcript.blocks, ...transcript.blocks],
                },
                error: null,
                loadingMore: false,
              };
            }
            return {
              phase: "ready",
              transcript,
              error: null,
              loadingMore: false,
            };
          });
        })
        .catch((error) => {
          if (
            !shouldApplyActivityTranscriptRequest(
              expected,
              currentTranscriptRequest.current,
            )
          ) {
            return;
          }
          setTranscriptState({
            phase: "error",
            transcript: null,
            error:
              error instanceof Error ? error.message : "transcript unavailable",
            loadingMore: false,
          });
        });
    },
    [loadTranscript, selectedItem, transcriptState, view.workRootId],
  );

  useEffect(() => {
    currentTranscriptRequest.current = {
      workRootId: view.workRootId,
      activityId: selectedItemId,
      requestId: transcriptRequestSeq.current + 1,
    };
    setTranscriptState({
      phase: "loading",
      transcript: null,
      error: null,
      loadingMore: false,
    });
    if (selectedItem) {
      acknowledgeSelected(selectedItem);
    }
    requestTranscript("replace");
  }, [view.workRootId, selectedItemId, selectedRevision]);

  const handleSelect = (item: ActivityItem) => {
    onCommand(buildActivitySelectItemCommand(item.id), {
      "activity.selectItem": () => {
        acknowledgeSelected(item);
        setSelectedItemId(item.id);
      },
    });
  };

  const handleLoadMore = () => {
    if (!selectedItem) return;
    onCommand(buildActivityTranscriptLoadMoreCommand(selectedItem.id), {
      "activity.transcript.loadMore": () => requestTranscript("append"),
    });
  };

  const handleRefresh = () => {
    onCommand(buildActivityRefreshCommand(view.workRootId), {
      "activity.refresh": () => requestTranscript("replace"),
    });
  };

  const handleToggleDetail = (block: TranscriptBlock) => {
    if (!selectedItem) return;
    const detailKey = block.cursor;
    onCommand(buildActivityDetailToggleCommand(selectedItem.id, detailKey), {
      "activity.detail.toggle": () => {
        setExpandedDetails((current) => {
          const next = new Set(current);
          if (next.has(detailKey)) next.delete(detailKey);
          else next.add(detailKey);
          return next;
        });
      },
    });
  };

  const transcript =
    transcriptState.phase === "ready" ? transcriptState.transcript : null;
  return (
    <section className="activity-console" data-activity-console="ready">
      <div className="activity-console-summary" aria-label="Activity summary">
        <span className="meta-chip">{view.status}</span>
        <span className="meta-chip">{view.summary.total} total</span>
        <span className="meta-chip">{view.summary.active} active</span>
        <span className="meta-chip">{view.summary.blocked} attention</span>
      </div>
      {orderedItems.length === 0 ? (
        <p
          className="workroot-activity-empty"
          data-activity-console-state="empty"
        >
          No activity for this workRoot.
        </p>
      ) : (
        <>
          <ActivityRibbon
            dirtyItems={dirtyItems}
            items={orderedItems}
            onSelect={handleSelect}
            selectedItemId={selectedItemId}
          />
          <TranscriptBlockViewer
            expandedDetails={expandedDetails}
            onLoadMore={handleLoadMore}
            onRefresh={handleRefresh}
            onToggleDetail={handleToggleDetail}
            selectedItem={selectedItem}
            state={transcriptState}
            transcript={transcript}
          />
        </>
      )}
    </section>
  );
}

export function ActivityRibbon({
  dirtyItems,
  items,
  onSelect,
  selectedItemId,
}: {
  dirtyItems: ReadonlySet<string>;
  items: readonly ActivityItem[];
  onSelect: (item: ActivityItem) => void;
  selectedItemId: string | null;
}) {
  return (
    <div className="activity-ribbon" aria-label="Activity ribbon">
      {items.map((item) => (
        <button
          className="activity-ribbon-item"
          data-command-id="activity.selectItem"
          data-activity-id={item.id}
          data-selected={item.id === selectedItemId ? "true" : "false"}
          data-dirty={dirtyItems.has(item.id) ? "true" : "false"}
          key={item.id}
          type="button"
          onClick={() => onSelect(item)}
        >
          <span className="activity-ribbon-cue" aria-hidden="true" />
          <span className="activity-ribbon-meta">
            {item.source.label || item.kind}
          </span>
          <span className="activity-ribbon-title">{item.label}</span>
          <span className="activity-ribbon-status">{item.status}</span>
        </button>
      ))}
    </div>
  );
}

export function TranscriptBlockViewer({
  expandedDetails,
  onLoadMore,
  onRefresh,
  onToggleDetail,
  selectedItem,
  state,
  transcript,
}: {
  expandedDetails: ReadonlySet<string>;
  onLoadMore: () => void;
  onRefresh: () => void;
  onToggleDetail: (block: TranscriptBlock) => void;
  selectedItem: ActivityItem | null;
  state: ActivityTranscriptLoadState;
  transcript: ActivityTranscript | null;
}) {
  return (
    <section className="activity-transcript" aria-label="Activity transcript">
      <div className="activity-transcript-head">
        <div className="activity-transcript-title">
          <strong>{selectedItem?.label ?? "Activity"}</strong>
          <span>{selectedItem?.status ?? "unavailable"}</span>
        </div>
      </div>
      {state.phase === "loading" ? (
        <div className="workroot-activity-state">Loading transcript</div>
      ) : state.phase === "error" ? (
        <div className="workroot-activity-state workroot-activity-state-error">
          <span>Transcript unavailable</span>
          <button
            className="activity-console-control"
            data-command-id="activity.refresh"
            type="button"
            onClick={onRefresh}
          >
            Retry
          </button>
        </div>
      ) : transcript && transcript.blocks.length > 0 ? (
        <div
          className="activity-transcript-scroll"
          onScroll={(event) => {
            const element = event.currentTarget;
            if (
              shouldLoadMoreActivityTranscript(
                element,
                transcript.hasMore,
                state.loadingMore,
              )
            ) {
              onLoadMore();
            }
          }}
        >
          {transcript.blocks.map((block) => (
            <ActivityTranscriptBlock
              block={block}
              expanded={expandedDetails.has(block.cursor)}
              key={block.cursor}
              onToggle={() => onToggleDetail(block)}
              sourceKind={transcript.source.kind}
            />
          ))}
          {transcript.hasMore ? (
            <button
              className="activity-console-control activity-load-more"
              data-command-id="activity.transcript.loadMore"
              disabled={state.loadingMore}
              type="button"
              onClick={onLoadMore}
            >
              {state.loadingMore ? "Loading" : "Load more"}
            </button>
          ) : null}
        </div>
      ) : (
        <div
          className="workroot-activity-state"
          data-activity-transcript-state="empty"
        >
          Transcript is {selectedItem?.transcript.status ?? "empty"}.
        </div>
      )}
    </section>
  );
}

function ActivityTranscriptBlock({
  block,
  expanded,
  onToggle,
  sourceKind,
}: {
  block: TranscriptBlock;
  expanded: boolean;
  onToggle: () => void;
  sourceKind: string;
}) {
  const view = transcriptBlockView(block, sourceKind);
  const detailVisible =
    view.mode === "expanded" || view.mode === "terminal" || expanded;
  return (
    <article
      className="activity-transcript-block"
      data-block-mode={view.mode}
      data-block-tone={view.tone}
    >
      <div className="activity-transcript-block-head">
        <span>{view.summary}</span>
        {view.mode === "compact" && view.detail ? (
          <button
            className="activity-detail-toggle"
            data-command-id="activity.detail.toggle"
            type="button"
            onClick={onToggle}
          >
            {expanded ? "Less" : "More"}
          </button>
        ) : null}
      </div>
      {detailVisible && view.detail ? (
        <pre className="activity-transcript-block-detail">{view.detail}</pre>
      ) : null}
    </article>
  );
}
