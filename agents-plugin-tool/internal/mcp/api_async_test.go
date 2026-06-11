package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const apiAsyncTestTimeout = 10 * time.Second

func TestAPIAsyncMCPToolsListed(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	server := NewServer(root, "test")

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n"
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	line := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"]
	for _, name := range []string{"api.ask", "api.ask_async", "api.status", "api.result", "api.cancel"} {
		if !strings.Contains(line, name) {
			t.Fatalf("tools/list missing %s: %s", name, line)
		}
	}
	if !strings.Contains(line, "api_job_key") {
		t.Fatalf("async API tools missing api_job_key schema: %s", line)
	}
}

func callMCPToolForTest(t *testing.T, server *Server, name string, args map[string]any) string {
	t.Helper()
	line, err := callMCPToolForTestNoFatal(server, name, args)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func callMCPToolForTestNoFatal(server *Server, name string, args map[string]any) (string, error) {
	if args == nil {
		args = map[string]any{}
	}
	rawArgs, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, name, rawArgs) + "\n"
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		return "", fmt.Errorf("ServeStdio returned error: %w", err)
	}
	byID, err := responseLinesByIDNoFatal(strings.Split(strings.TrimSpace(out.String()), "\n"))
	if err != nil {
		return "", err
	}
	line, ok := byID["1"]
	if !ok {
		return "", fmt.Errorf("missing response id 1: %s", out.String())
	}
	return line, nil
}

func responseLinesByIDNoFatal(lines []string) (map[string]string, error) {
	byID := make(map[string]string, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var resp struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			return nil, err
		}
		byID[strings.Trim(string(resp.ID), `"`)] = line
	}
	return byID, nil
}

func decodeToolJSON[T any](t *testing.T, line string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(toolText(t, line)), &value); err != nil {
		t.Fatalf("decode tool JSON failed: %v\n%s", err, line)
	}
	return value
}

func startAPIJobForTest(t *testing.T, server *Server, args map[string]any) string {
	t.Helper()
	line := callMCPToolForTest(t, server, "api.ask_async", args)
	if toolIsError(t, line) {
		t.Fatalf("api.ask_async returned tool error: %s", line)
	}
	start := decodeToolJSON[apiJobStartResponse](t, line)
	if strings.TrimSpace(start.APIJobKey) == "" {
		t.Fatalf("api.ask_async returned empty job key: %#v", start)
	}
	return start.APIJobKey
}

func waitForAPIJobReadyForTest(t *testing.T, server *Server, key string) apiJobStatusResponse {
	t.Helper()
	var last apiJobStatusResponse
	for deadline := time.Now().Add(apiAsyncTestTimeout); time.Now().Before(deadline); {
		line := callMCPToolForTest(t, server, "api.status", map[string]any{"api_job_key": key})
		if toolIsError(t, line) {
			t.Fatalf("api.status returned tool error: %s", line)
		}
		status := decodeToolJSON[apiJobStatusResponse](t, line)
		last = status
		if status.ResultReady || status.Status == apiJobStateFailed || status.Status == apiJobStateCancelled {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("api job %q did not become ready; last status: %#v", key, last)
	return apiJobStatusResponse{}
}

func assertAPIDomainProgress(t *testing.T, status apiJobStatusResponse, domain, wantStatus, wantError string) {
	t.Helper()
	for _, progress := range status.Domains {
		if progress.Domain != domain {
			continue
		}
		if progress.Status != wantStatus {
			t.Fatalf("domain %q status = %q, want %q in %#v", domain, progress.Status, wantStatus, status)
		}
		if wantError != "" && !strings.Contains(progress.Error, wantError) {
			t.Fatalf("domain %q error = %q, want containing %q in %#v", domain, progress.Error, wantError, status)
		}
		if wantError == "" && progress.Error != "" {
			t.Fatalf("domain %q unexpected error = %q in %#v", domain, progress.Error, status)
		}
		return
	}
	t.Fatalf("domain %q missing from status progress: %#v", domain, status)
}

type cancelAwareAPIRuntime struct {
	mu      sync.Mutex
	calls   []string
	ready   chan struct{}
	release chan struct{}
	once    sync.Once
}

func (f *cancelAwareAPIRuntime) Route(ctx context.Context, root, prompt string) (string, error) {
	return "go\n", nil
}

func (f *cancelAwareAPIRuntime) AskManager(ctx context.Context, root, domain, prompt string) (string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, filepath.Join(root, domain)+":"+prompt)
	f.mu.Unlock()
	f.once.Do(func() { close(f.ready) })
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-f.release:
		return "late answer", nil
	}
}
