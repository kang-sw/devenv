package wslive

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolInputSummaryPreference(t *testing.T) {
	for _, tc := range []struct {
		input, properties map[string]any
		want              string
	}{
		{map[string]any{"summary": " primary ", "toolSummary": "secondary", "tool_summary": "last"}, nil, "primary"},
		{map[string]any{"summary": " \n ", "toolSummary": "camel"}, nil, "camel"},
		{map[string]any{"summary": 1, "toolSummary": nil, "tool_summary": "snake"}, nil, "snake"},
		{map[string]any{"summary": "domain", "toolSummary": "metadata"}, map[string]any{"summary": nil}, "metadata"},
		{map[string]any{"summary": "domain"}, map[string]any{"summary": nil}, ""},
		{map[string]any{"command": "ls"}, nil, ""},
		{nil, nil, ""},
	} {
		if got := ToolInputSummary(tc.input, tc.properties); got != tc.want {
			t.Fatalf("ToolInputSummary(%v, %v) = %q, want %q", tc.input, tc.properties, got, tc.want)
		}
	}
}

func TestToolSummariesReachRunningFinishedAndPermissionRows(t *testing.T) {
	for _, key := range ToolSummaryKeys {
		t.Run(key, func(t *testing.T) {
			st := NewState(ownerKey)
			input := map[string]any{
				key:     "Review " + ownerKey + " password=hunter2 " + strings.Repeat("x", ArgsPreviewCap),
				"query": "arguments must not displace the summary",
			}
			raw, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			want := st.argsPreview(string(raw))
			if !strings.HasPrefix(want, "Review id:") || !strings.Contains(want, "password=***") || strings.Contains(want, ownerKey) || strings.Contains(want, "hunter2") || len([]rune(want)) != ArgsPreviewCap {
				t.Fatalf("summary not preferred, scrubbed and capped: %q", want)
			}
			mustApply(t, st, Event{Kind: KindPermission, Tool: "mcp_ws_runtime_read", Args: input}, 1)
			if got := st.Snapshot(2).Lead.Permission.ArgsPreview; got != want {
				t.Fatalf("permission summary = %q, want %q", got, want)
			}
			mustApply(t, st, Event{Kind: KindPermission, Clear: true}, 3)
			mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, running(1, 1, 0, "mcp_ws_runtime_read", string(raw)))}, 4)
			if got := st.Snapshot(5).Lead.Batches[0].Running.ArgsPreview; got != want {
				t.Fatalf("running summary = %q, want %q", got, want)
			}
			c := call("mcp_ws_runtime_read", 10, 20, false)
			c["argsPreview"] = string(raw)
			mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, withCalls(finishedAt(1, 10, 20, 1, 0, "mcp_ws_runtime_read"), c))}, 30)
			if got := st.Snapshot(31).Lead.ToolGap.Batches[0].CallArgs[0].ArgsPreview; got != want {
				t.Fatalf("finished summary = %q, want %q", got, want)
			}
			res := mustApply(t, st, Event{Kind: KindReply, Final: true, Text: "done"}, 40)
			if got := res.Ring[0].Tools.Batches[0].CallArgs[0].ArgsPreview; got != want {
				t.Fatalf("ring summary = %q, want %q", got, want)
			}
		})
	}
}
