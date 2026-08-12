package wsnote

import (
	"encoding/json"
	"testing"
)

// TestRecordUnmarshalJSONMissingVisibleDefaultsTrue verifies the migration
// contract: a record stored before the "visible" field existed has no
// "visible" key at all in its JSON, and that absence must decode as true
// (visible) — the OPPOSITE of encoding/json's ordinary zero-value behavior
// for a plain bool field, which is exactly why Record has a custom
// UnmarshalJSON.
func TestRecordUnmarshalJSONMissingVisibleDefaultsTrue(t *testing.T) {
	raw := []byte(`{"key":"legacy.key","value":"v","priority":1,"written_at":"2026-08-01T00:00:00Z"}`)

	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !rec.Visible {
		t.Fatalf("Record.Visible = false for a payload missing \"visible\", want true (migration default)")
	}
	if rec.Key != "legacy.key" || rec.Value != "v" || rec.Priority != 1 || rec.WrittenAt != "2026-08-01T00:00:00Z" {
		t.Fatalf("Record fields not decoded correctly: %+v", rec)
	}
}

// TestRecordUnmarshalJSONExplicitFalsePreserved verifies an explicit
// "visible":false is NOT overridden by the migration default — only a
// genuinely absent key defaults to true.
func TestRecordUnmarshalJSONExplicitFalsePreserved(t *testing.T) {
	raw := []byte(`{"key":"muted.key","value":"v","priority":1,"written_at":"2026-08-01T00:00:00Z","visible":false}`)

	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if rec.Visible {
		t.Fatalf("Record.Visible = true for an explicit \"visible\":false payload, want false preserved")
	}
}

// TestRecordUnmarshalJSONExplicitTruePreserved is the mirror-image sanity
// check: an explicit "visible":true round-trips as true too (not just as
// the "happens to be the default" case).
func TestRecordUnmarshalJSONExplicitTruePreserved(t *testing.T) {
	raw := []byte(`{"key":"visible.key","value":"v","priority":1,"written_at":"2026-08-01T00:00:00Z","visible":true}`)

	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !rec.Visible {
		t.Fatalf("Record.Visible = false for an explicit \"visible\":true payload, want true preserved")
	}
}

// TestRecordMarshalJSONAlwaysEmitsVisible verifies MarshalJSON (the default,
// plain-struct-tag encoder — no custom override) always emits an explicit
// "visible" key, so every freshly-written record is self-describing on disk
// and never depends on the migration default going forward.
func TestRecordMarshalJSONAlwaysEmitsVisible(t *testing.T) {
	rec := Record{Key: "k", Value: "v", Priority: 1, WrittenAt: "t", Visible: false}
	out, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var round map[string]any
	if err := json.Unmarshal(out, &round); err != nil {
		t.Fatalf("Unmarshal round-trip: %v", err)
	}
	if _, ok := round["visible"]; !ok {
		t.Fatalf("Marshal output %s missing explicit \"visible\" key", out)
	}
	if round["visible"] != false {
		t.Fatalf("Marshal output %s has visible=%v, want false", out, round["visible"])
	}
}
