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

func TestAPIAskAsyncImmediateStartReturnsRecoverableJobKeyAndStatus(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "ai-docs/.deps/go/README.md", "go")
	fake := &cancelAwareAPIRuntime{ready: make(chan struct{}), release: make(chan struct{})}
	released := false
	defer func() {
		if !released {
			close(fake.release)
		}
	}()
	server := NewServer(root, "test")
	server.api = fake

	startLineCh := make(chan struct {
		line string
		err  error
	}, 1)
	go func() {
		line, err := callMCPToolForTestNoFatal(server, "api.ask_async", map[string]any{
			"prompt":      "How do modules work?",
			"domain_hint": "go",
		})
		startLineCh <- struct {
			line string
			err  error
		}{line: line, err: err}
	}()
	var startLine string
	select {
	case result := <-startLineCh:
		if result.err != nil {
			t.Fatal(result.err)
		}
		startLine = result.line
	case <-time.After(time.Second):
		t.Fatal("api.ask_async did not return before blocked manager work was released")
	}
	if toolIsError(t, startLine) {
		t.Fatalf("api.ask_async returned tool error: %s", startLine)
	}
	startResponse := decodeToolJSON[apiJobStartResponse](t, startLine)
	if strings.TrimSpace(startResponse.APIJobKey) == "" {
		t.Fatalf("api.ask_async returned empty job key: %#v", startResponse)
	}
	if startResponse.ResultReady {
		t.Fatalf("api.ask_async should not report result_ready on immediate start: %#v", startResponse)
	}

	statusLine := callMCPToolForTest(t, server, "api.status", map[string]any{"api_job_key": startResponse.APIJobKey})
	if toolIsError(t, statusLine) {
		t.Fatalf("api.status could not recover job by key %q: %s", startResponse.APIJobKey, statusLine)
	}
	status := decodeToolJSON[apiJobStatusResponse](t, statusLine)
	if status.APIJobKey != startResponse.APIJobKey {
		t.Fatalf("api.status returned wrong job key: %#v", status)
	}
	if status.Prompt != "How do modules work?" || status.DomainHint != "go" {
		t.Fatalf("api.status did not preserve prompt/hint: %#v", status)
	}

	recoveredServer := NewServer(root, "test")
	recoveredLine := callMCPToolForTest(t, recoveredServer, "api.status", map[string]any{"api_job_key": startResponse.APIJobKey})
	if toolIsError(t, recoveredLine) {
		t.Fatalf("fresh server could not recover durable job %q: %s", startResponse.APIJobKey, recoveredLine)
	}
	recovered := decodeToolJSON[apiJobStatusResponse](t, recoveredLine)
	if recovered.APIJobKey != startResponse.APIJobKey || recovered.Prompt != "How do modules work?" {
		t.Fatalf("fresh server did not recover durable job state: %#v", recovered)
	}

	select {
	case <-fake.ready:
	case <-time.After(apiAsyncTestTimeout):
		t.Fatal("async manager work did not start")
	}
	close(fake.release)
	released = true
	finalStatus := waitForAPIJobReadyForTest(t, recoveredServer, startResponse.APIJobKey)
	if finalStatus.Status != apiJobStateSucceeded {
		t.Fatalf("fresh server did not recover completed job status: %#v", finalStatus)
	}
	resultLine := callMCPToolForTest(t, recoveredServer, "api.result", map[string]any{"api_job_key": startResponse.APIJobKey})
	if toolIsError(t, resultLine) || !strings.Contains(toolText(t, resultLine), "## Domain: go\nlate answer") {
		t.Fatalf("fresh server did not recover completed job result: %s", resultLine)
	}
}

func TestAPIAsyncPollingResultPreservesPartialFailureAggregation(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "ai-docs/.deps/go/README.md", "go")
	mustWrite(t, root, "ai-docs/.deps/python/README.md", "python")
	fake := &fakeAPIRuntime{
		answers: map[string]string{"go": "go ok"},
		errs:    map[string]error{"python": fmt.Errorf("python unavailable")},
	}
	server := NewServer(root, "test")
	server.api = fake

	key := startAPIJobForTest(t, server, map[string]any{"prompt": "Compare clients"})
	status := waitForAPIJobReadyForTest(t, server, key)
	if status.Status != apiJobStatePartialFailed {
		t.Fatalf("partial domain failure should be a partial_failed job, got %#v", status)
	}
	if len(status.ResolvedDomains) != 2 {
		t.Fatalf("status should expose resolved domains: %#v", status)
	}
	assertAPIDomainProgress(t, status, "go", apiDomainStateSucceeded, "")
	assertAPIDomainProgress(t, status, "python", apiDomainStateFailed, "python unavailable")

	resultLine := callMCPToolForTest(t, server, "api.result", map[string]any{"api_job_key": key})
	if toolIsError(t, resultLine) {
		t.Fatalf("partial success should not make api.result a tool error: %s", resultLine)
	}
	text := toolText(t, resultLine)
	if !strings.Contains(text, "## Domain: go\ngo ok") || !strings.Contains(text, "## Domain: python\nERROR: python unavailable") {
		t.Fatalf("api.result did not preserve synchronous aggregation boundaries:\n%s", text)
	}
	if len(fake.routeCalls) != 1 || len(fake.managerHits) != 2 {
		t.Fatalf("async job did not reuse routed manager semantics; route=%v managers=%v", fake.routeCalls, fake.managerHits)
	}
}

func TestAPIAsyncAllDomainFailureReturnsToolErrorWithMetadata(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	fake := &fakeAPIRuntime{errs: map[string]error{"go": fmt.Errorf("go failed"), "python": fmt.Errorf("python failed")}}
	server := NewServer(root, "test")
	server.api = fake

	key := startAPIJobForTest(t, server, map[string]any{"prompt": "question"})
	status := waitForAPIJobReadyForTest(t, server, key)
	if status.Status != apiJobStateFailed {
		t.Fatalf("all-domain failure should fail the job, got %#v", status)
	}
	assertAPIDomainProgress(t, status, "go", apiDomainStateFailed, "go failed")
	assertAPIDomainProgress(t, status, "python", apiDomainStateFailed, "python failed")

	resultLine := callMCPToolForTest(t, server, "api.result", map[string]any{"api_job_key": key})
	if !toolIsError(t, resultLine) {
		t.Fatalf("all-domain failure should make api.result a tool error: %s", resultLine)
	}
	text := toolText(t, resultLine)
	if !strings.Contains(text, "## Domain: go\nERROR: go failed") ||
		!strings.Contains(text, "## Domain: python\nERROR: python failed") ||
		!strings.Contains(text, "api.ask failed for all resolved domains") {
		t.Fatalf("api.result all-failure text missing synchronous metadata:\n%s", text)
	}
}

func TestAPIAsyncCancelStopsActiveWorkBestEffort(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "ai-docs/.deps/go/README.md", "go")
	fake := &cancelAwareAPIRuntime{ready: make(chan struct{}), release: make(chan struct{})}
	server := NewServer(root, "test")
	server.api = fake

	key := startAPIJobForTest(t, server, map[string]any{
		"prompt":      "How do modules work?",
		"domain_hint": "go",
	})
	select {
	case <-fake.ready:
	case <-time.After(apiAsyncTestTimeout):
		t.Fatal("manager call did not start before cancellation")
	}

	cancelLine := callMCPToolForTest(t, server, "api.cancel", map[string]any{"api_job_key": key})
	if toolIsError(t, cancelLine) {
		t.Fatalf("api.cancel returned tool error: %s", cancelLine)
	}
	cancelled := decodeToolJSON[apiJobStatusResponse](t, cancelLine)
	if !cancelled.CancelRequested && cancelled.Status != apiJobStateCancelled {
		t.Fatalf("api.cancel did not expose cancellation state: %#v", cancelled)
	}

	close(fake.release)
	status := waitForAPIJobReadyForTest(t, server, key)
	if status.Status != apiJobStateCancelled {
		t.Fatalf("cancelled job should settle as cancelled, got %#v", status)
	}
	resultLine := callMCPToolForTest(t, server, "api.result", map[string]any{"api_job_key": key})
	if !toolIsError(t, resultLine) || !strings.Contains(toolText(t, resultLine), "cancel") {
		t.Fatalf("cancelled job result should be a cancellation tool error: %s", resultLine)
	}
}

func TestAPIAsyncPreservesSynchronousAPIAskCompatibility(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "ai-docs/.deps/go/README.md", "go")
	fake := &fakeAPIRuntime{answers: map[string]string{"go": "sync go answer"}}
	server := NewServer(root, "test")
	server.api = fake

	syncLine := callMCPToolForTest(t, server, "api.ask", map[string]any{
		"prompt":      "How do modules work?",
		"domain_hint": "go",
	})
	if toolIsError(t, syncLine) {
		t.Fatalf("api.ask returned tool error after async surface was added: %s", syncLine)
	}
	if text := toolText(t, syncLine); !strings.Contains(text, "## Domain: go\nsync go answer") {
		t.Fatalf("api.ask compatibility response changed:\n%s", text)
	}

	asyncLine := callMCPToolForTest(t, server, "api.ask_async", map[string]any{
		"prompt":      "How do modules work?",
		"domain_hint": "go",
	})
	if toolIsError(t, asyncLine) {
		t.Fatalf("api.ask_async should coexist with api.ask: %s", asyncLine)
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
