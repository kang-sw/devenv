package wsagent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestArgsPreviewPrefersToolSummary(t *testing.T) {
	for _, key := range wslive.ToolSummaryKeys {
		raw, err := json.Marshal(map[string]any{key: "  Explain this call  ", "command": "echo unrelated", "path": "/other"})
		if err != nil {
			t.Fatal(err)
		}
		if got := argsPreview(raw); got != "Explain this call" {
			t.Fatalf("%s preview = %q", key, got)
		}
	}
	for _, raw := range []string{
		`{"summary":" ","toolSummary":42,"command":"echo normal"}`,
		`{"command":"echo normal"}`,
	} {
		if got := argsPreview(json.RawMessage(raw)); got != "echo normal" {
			t.Fatalf("absent/invalid summary changed normal preview: %s = %q", raw, got)
		}
	}
	if got := argsPreview(json.RawMessage(`{"other":1}`)); !strings.Contains(got, `"other":1`) {
		t.Fatalf("raw JSON fallback changed: %s", got)
	}
}
