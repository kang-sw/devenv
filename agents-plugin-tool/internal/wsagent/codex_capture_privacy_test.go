package wsagent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCodexCaptureScrubsPrivateMetadata(t *testing.T) {
	capture := &codexLiveCapture{lines: []string{
		`>> {"id":"client-id","method":"thread/start","params":{"cwd":"/example/private-work","threadId":"opaque-thread"}}`,
		`{"method":"remoteControl/status/changed","params":{"status":"disabled","serverName":"machine.private.invalid","installationId":"installation-identity"}}`,
		`{"method":"account/updated","params":{"authMode":"private-auth","planType":"private-plan","account_id":"private-account","email":"owner@example.invalid"}}`,
		`{"method":"account/rateLimits/updated","params":{"rateLimits":{"primary":{"usedPercent":88,"resetsAt":1111111111,"windowDurationMins":300},"credits":{"hasCredits":true,"balance":"private-balance"},"availableAccessPrograms":{"private-program":true},"planType":"private-plan"}}}`,
		`{"result":{"userAgent":"private-machine","codexHome":"/example/private-home","data":[{"path":"/example/private-file","hostname":"machine.private.invalid","sourcePath":"/example/private-source","environmentId":"private-environment","projectId":"private-project","runtimeWorkspaceRoots":["/example/private-work"],"gitInfo":{"originUrl":"private-origin"},"instructionSources":["private-instructions"],"model":"synthetic-model"}]}}`,
	}}
	path := t.TempDir() + "/capture.jsonl"
	capture.save(t, path, nil)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"private-", "private.", "@", "/example/"} {
		if strings.Contains(string(raw), marker) {
			t.Fatalf("capture retained private metadata marker %q", marker)
		}
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 5 || !strings.HasPrefix(lines[0], ">> ") {
		t.Fatal("sanitization changed line order or client-line direction")
	}
	for _, line := range lines {
		var message map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, ">> ")), &message); err != nil {
			t.Fatal(err)
		}
		assertCodexCapturePrivacy(t, message)
	}
	var first codexWire
	_ = json.Unmarshal([]byte(strings.TrimPrefix(lines[0], ">> ")), &first)
	if rawString(first.ID) != "client-id" || jsonAt(first.Params, "threadId") != "opaque-thread" {
		t.Fatal("sanitization must preserve opaque protocol correlation ids")
	}
	var limits codexWire
	_ = json.Unmarshal([]byte(lines[3]), &limits)
	if jsonAt(limits.Params, "rateLimits", "primary", "windowDurationMins") != float64(300) {
		t.Fatal("sanitization must preserve the limit window's protocol shape")
	}
}

func TestCodexHostedFixturePrivacy(t *testing.T) {
	messages := readCodexFixture(t, "codex-0.160.0-hosted-lead.jsonl")
	accounts, limits, remote := 0, 0, 0
	for _, message := range messages {
		raw, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		assertCodexCapturePrivacy(t, value)
		switch message.Method {
		case "account/updated":
			accounts++
		case "account/rateLimits/updated":
			limits++
		case "remoteControl/status/changed":
			remote++
		}
	}
	if accounts == 0 || limits == 0 || remote == 0 {
		t.Fatal("privacy checks must exercise recorded account, quota and remote-control messages")
	}
}

func assertCodexCapturePrivacy(t *testing.T, value any) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			switch strings.ToLower(strings.ReplaceAll(key, "_", "")) {
			case "cwd", "hostname", "accountid", "email", "installationid", "environmentid", "projectid",
				"codexhome", "runtimeworkspaceroots", "sourcepath", "path", "gitinfo", "instructionsources",
				"credits", "individuallimit", "spendcontrolreached", "ratelimitreachedtype", "availableaccessprograms":
				t.Errorf("capture retained private field %q", key)
			case "plantype":
				if child != "synthetic-plan" {
					t.Error("plan must be synthetic")
				}
			case "authmode":
				if child != "synthetic-auth" {
					t.Error("authentication mode must be synthetic")
				}
			case "usedpercent":
				if child != float64(42) {
					t.Error("quota consumption must be synthetic")
				}
			case "resetsat":
				if child != float64(2_000_000_000) {
					t.Error("quota reset must be synthetic")
				}
			case "useragent":
				if child != "synthetic-agent" {
					t.Error("user agent must not retain machine details")
				}
			case "servername":
				if strings.Contains(child.(string), ".") {
					t.Error("server name must not be a hostname")
				}
			}
			assertCodexCapturePrivacy(t, child)
		}
	case []any:
		for _, child := range value {
			assertCodexCapturePrivacy(t, child)
		}
	case string:
		if strings.Contains(value, "@") || strings.Contains(value, ".local") || strings.Contains(value, "/Users/") {
			t.Error("capture contains an email, local hostname or user home")
		}
	}
}
