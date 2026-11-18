package wsagent

import "testing"

func TestPiMCPOnlyIgnoresPackageAndIsolationWarnings(t *testing.T) {
	for _, warning := range []string{"", "invalid hosted package; using wsflow.", piNoSDKWarning} {
		if PiMCPOnly(Record{Backend: BackendPi, Lead: true, PluginWarning: warning}, nil) {
			t.Fatalf("native extension incorrectly classified as MCP-only: %q", warning)
		}
	}
	if !PiMCPOnly(Record{Backend: BackendPi, Lead: true, PluginWarning: "Hosted package fallback. Bundled Pi extension unavailable; using ws MCP bridge"}, nil) {
		t.Fatal("extension bridge fallback must stay MCP-only")
	}
}
