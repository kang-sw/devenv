package wsdashboard

import (
	"context"
	"encoding/json"
)

// TrackerProjectionHost is additive: hosts implementing only TrackerHost keep
// serving /view, and the new routes answer 404 so a new page can fall back.
// Projection is immutable after return, including everything Detail captures.
// It must not render markdown or encode detail payloads for unrequested items.
// Every returned string has the same credential redaction as TrackerHost.View.
type TrackerProjectionHost interface {
	Projection(context.Context, string) (TrackerProjection, error)
}

// TrackerSummaryRow carries list/handle fields, never bodies, body revisions,
// comment/event payloads, rendered HTML or attachment metadata. Data also holds
// this item's assignment/request/rule records and notices, so stale flags and
// indirect derivations take the same per-item diff path as direct edits.
type TrackerSummaryRow struct {
	Data          json.RawMessage `json:"data"`
	DetailVersion string          `json:"detailVersion"`
	Seen          TrackerSeen     `json:"seen"`
}

// TrackerProjection is one coherent host view. Globals are independently
// replaceable fields (viewer, members, itemOrder, my, stats, ...); a delta sends
// only changed fields, not a whole itemOrder on every event. Feed is a bounded
// latest-N activity list with plain-text snippets, not detail payloads.
// Keys associates badge/read notification keys with their item; Seen is the
// host's initial marks. Dashboard-local seen writes overlay them without a
// second host read. Detail returns only requested item bundles from THIS view;
// the dashboard attaches the row's version and the published cursor.
type TrackerProjection struct {
	Items   map[string]TrackerSummaryRow
	Globals map[string]json.RawMessage
	Feed    json.RawMessage
	Keys    map[string]TrackerSeen
	Seen    TrackerSeen
	Detail  func(context.Context, []string) (map[string]json.RawMessage, error)
}

type TrackerDetailBundle struct {
	DetailVersion string          `json:"detailVersion"`
	Data          json.RawMessage `json:"data"`
}
