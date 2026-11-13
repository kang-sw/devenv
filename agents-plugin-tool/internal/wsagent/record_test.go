package wsagent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordUnknownMembersRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra string
	}{
		{name: "legacy"},
		{name: "newer", extra: `,
			"future_recovery": {"sequence": 9007199254740993, "items": [true, "saved", null]},
			"future_null": null,
			"future_string": "keep me",
			"unknown": [1, 2, 3]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "recordroundtrip")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			fixture := `{
				"schema_version": 1,
				"reply_id": "recordroundtrip",
				"profile": "chat",
				"backend": "pi",
				"root": "workspace",
				"ws_session_key": "private-key",
				"created_at": "2026-11-13T00:00:00Z",
				"holder_id": "old-holder",
				"MODEL": "old-model"` + tc.extra + `}`
			if err := os.WriteFile(recordPath(dir), []byte(fixture), 0o600); err != nil {
				t.Fatal(err)
			}
			var original map[string]json.RawMessage
			if err := json.Unmarshal([]byte(fixture), &original); err != nil {
				t.Fatal(err)
			}
			for pass := 0; pass < 2; pass++ {
				rec, err := readRecord(dir)
				if err != nil {
					t.Fatal(err)
				}
				if pass == 0 && (rec.HolderID != "old-holder" || rec.Model != "old-model") {
					t.Fatalf("known fields were not decoded: holder %q, model %q", rec.HolderID, rec.Model)
				}
				rec.Label = "updated"
				rec.HolderID, rec.Model = "", ""
				if err := writeRecord(dir, rec); err != nil {
					t.Fatal(err)
				}
				raw, err := os.ReadFile(recordPath(dir))
				if err != nil {
					t.Fatal(err)
				}
				var saved map[string]json.RawMessage
				if err := json.Unmarshal(raw, &saved); err != nil {
					t.Fatal(err)
				}
				if string(saved["schema_version"]) != "1" || string(saved["label"]) != `"updated"` {
					t.Fatalf("schema or known-field edit changed: %s", raw)
				}
				for _, key := range []string{"holder_id", "model", "MODEL"} {
					if _, exists := saved[key]; exists {
						t.Fatalf("cleared optional field %q resurrected: %s", key, raw)
					}
				}
				for _, key := range []string{"future_recovery", "future_null", "future_string", "unknown"} {
					want, exists := original[key]
					got, kept := saved[key]
					if exists != kept {
						t.Fatalf("unknown member %q presence = %v, want %v", key, kept, exists)
					}
					if !exists {
						continue
					}
					var compactWant, compactGot bytes.Buffer
					if err := json.Compact(&compactWant, want); err != nil {
						t.Fatal(err)
					}
					if err := json.Compact(&compactGot, got); err != nil {
						t.Fatal(err)
					}
					if compactWant.String() != compactGot.String() {
						t.Fatalf("unknown member %q = %s, want %s", key, got, want)
					}
				}
			}
		})
	}
}
