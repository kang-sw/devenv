package wsagent

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseGeminiStreamJSONCapturesSessionTextAndIgnoresNoise(t *testing.T) {
	raw := strings.Join([]string{
		"Gemini CLI notice before JSON",
		`{"type":"init","init":{"session_id":"gemini-session-1"}}`,
		`{"type":"message","message":{"role":"assistant","content":"hello "}}`,
		`{"type":"message","role":"assistant","content":[{"type":"tool_use","text":"ignored"},{"text":"world"}]}`,
		`{"type":"tool_use","tool_use":{"name":"ignored"}}`,
		`{"type":"tool_result","tool_result":{"content":"ignored"}}`,
		`{"type":"result","result":{"status":"success","duration_ms":12}}`,
		"Gemini CLI notice after JSON",
	}, "\n")
	var captured []string
	result, err := parseGeminiStreamJSON(strings.NewReader(raw), func(sessionID string) error {
		captured = append(captured, sessionID)
		return nil
	})
	if err != nil {
		t.Fatalf("parseGeminiStreamJSON returned error: %v", err)
	}
	if result.SessionID != "gemini-session-1" || result.Text != "hello world" ||
		result.FinalEventShape != "result/success" {
		t.Fatalf("result = %+v", result)
	}
	if len(captured) != 1 || captured[0] != "gemini-session-1" {
		t.Fatalf("captured sessions = %#v", captured)
	}
}

func TestParseGeminiStreamJSONFailures(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "terminal error",
			raw:  `{"type":"result","result":{"status":"error","error":{"type":"auth","message":"login required"}}}`,
			want: "auth: login required",
		},
		{
			name: "missing terminal",
			raw: strings.Join([]string{
				`{"type":"init","init":{"session_id":"gemini-session-1"}}`,
				`{"type":"message","message":{"role":"assistant","content":"hello"}}`,
			}, "\n"),
			want: "missing terminal result",
		},
		{
			name: "missing session",
			raw: strings.Join([]string{
				`{"type":"message","message":{"role":"assistant","content":"hello"}}`,
				`{"type":"result","result":{"status":"success"}}`,
			}, "\n"),
			want: "missing init.session_id",
		},
		{
			name: "missing assistant text",
			raw: strings.Join([]string{
				`{"type":"init","init":{"session_id":"gemini-session-1"}}`,
				`{"type":"message","message":{"role":"user","content":"not final"}}`,
				`{"type":"result","result":{"status":"success"}}`,
			}, "\n"),
			want: "missing assistant message.content",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseGeminiStreamJSON(strings.NewReader(tt.raw), nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parseGeminiStreamJSON error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestBuildGeminiInvocationUsesStdinPromptForFirstCall(t *testing.T) {
	invocation, err := buildGeminiInvocation(RunnerRequest{
		Prompt: "sentinel line 1\nsentinel line 2",
		Model:  "gemini",
	})
	if err != nil {
		t.Fatalf("buildGeminiInvocation returned error: %v", err)
	}
	if invocation.PromptStdin != "sentinel line 1\nsentinel line 2" || invocation.PromptDelivery != "stdin" {
		t.Fatalf("prompt delivery mismatch: %+v", invocation)
	}
	joined := strings.Join(invocation.Args, "\x00")
	for _, want := range []string{
		"--output-format\x00stream-json",
		"--approval-mode\x00yolo",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("gemini args missing %q: %+v", want, invocation.Args)
		}
	}
	for _, notWant := range []string{"sentinel line", "-m\x00", "--resume"} {
		if strings.Contains(joined, notWant) {
			t.Fatalf("gemini args contained %q: %+v", notWant, invocation.Args)
		}
	}
}

func TestBuildGeminiInvocationResumesWithConcreteModelAndSystemPrompt(t *testing.T) {
	systemPromptPath := filepath.Join(t.TempDir(), "system.md")
	if err := os.WriteFile(systemPromptPath, []byte("system text"), 0o644); err != nil {
		t.Fatal(err)
	}
	invocation, err := buildGeminiInvocation(RunnerRequest{
		Prompt:           "resume sentinel",
		Model:            "gemini-2.5-pro",
		SessionID:        "session-123",
		SystemPromptPath: systemPromptPath,
	})
	if err != nil {
		t.Fatalf("buildGeminiInvocation returned error: %v", err)
	}
	if !strings.Contains(invocation.PromptStdin, "System instructions:") ||
		!strings.Contains(invocation.PromptStdin, "system text") ||
		!strings.Contains(invocation.PromptStdin, "User prompt:") ||
		!strings.Contains(invocation.PromptStdin, "resume sentinel") {
		t.Fatalf("stdin prompt missing system/user block: %q", invocation.PromptStdin)
	}
	joined := strings.Join(invocation.Args, "\x00")
	for _, want := range []string{
		"-m\x00gemini-2.5-pro",
		"--resume\x00session-123",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("gemini args missing %q: %+v", want, invocation.Args)
		}
	}
	if strings.Contains(joined, "resume sentinel") || strings.Contains(joined, "system text") {
		t.Fatalf("prompt leaked into argv: %+v", invocation.Args)
	}
}

func TestGeminiRunnerExecutesFakeBinaryAndCapturesDiagnostics(t *testing.T) {
	repo := initRepo(t)
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gemini.log")
	writeFakeGeminiExecutable(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GEMINI_FAKE_LOG", logPath)
	t.Setenv("GEMINI_FAKE_FAIL", "")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var capturedSession string
	result, err := GeminiRunner{}.Call(RunnerRequest{
		Root:        repo,
		Prompt:      "hello gemini",
		Model:       "gemini-2.5-flash",
		Stdout:      &stdout,
		Stderr:      &stderr,
		Timeout:     time.Minute,
		ToolProfile: "leaf",
		OnSessionID: func(sessionID string) error {
			capturedSession = sessionID
			return nil
		},
	})
	if err != nil {
		t.Fatalf("GeminiRunner.Call returned error: %v", err)
	}
	if result.SessionID != "gemini-session" || capturedSession != "gemini-session" ||
		result.Text != "gemini reply" || result.BackendVersion != "gemini fake 1.2.3" ||
		result.PromptDelivery != "stdin" || result.FinalEventShape != "result/success" {
		t.Fatalf("result/session = %+v/%q", result, capturedSession)
	}
	if !strings.Contains(stdout.String(), "Gemini notice") || !strings.Contains(stdout.String(), `"content":"reply"`) {
		t.Fatalf("stdout diagnostics missing stream: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "fake diagnostic") {
		t.Fatalf("stderr diagnostics missing stream: %q", stderr.String())
	}
	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logRaw)
	for _, want := range []string{
		"--output-format stream-json --approval-mode yolo -m gemini-2.5-flash",
		"ENV:leaf",
		"STDIN:hello gemini",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("fake gemini log missing %q:\n%s", want, log)
		}
	}
}

func TestGeminiRunnerSessionCallbackErrorCancelsProcess(t *testing.T) {
	repo := initRepo(t)
	binDir := t.TempDir()
	writeFakeGeminiExecutable(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GEMINI_FAKE_LOG", filepath.Join(t.TempDir(), "gemini.log"))
	t.Setenv("GEMINI_FAKE_FAIL", "")
	t.Setenv("GEMINI_FAKE_SLEEP_AFTER_INIT", "1")

	start := time.Now()
	_, err := GeminiRunner{}.Call(RunnerRequest{
		Root:    repo,
		Prompt:  "callback fail",
		Timeout: time.Second,
		OnSessionID: func(string) error {
			return errors.New("state write failed")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "handle gemini session id") ||
		!strings.Contains(err.Error(), "state write failed") {
		t.Fatalf("GeminiRunner.Call error = %v, want callback error", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("callback error was not propagated promptly: elapsed=%s err=%v", elapsed, err)
	}
}

func TestGeminiRunnerNonZeroExitPreservesStderr(t *testing.T) {
	repo := initRepo(t)
	binDir := t.TempDir()
	writeFakeGeminiExecutable(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GEMINI_FAKE_LOG", filepath.Join(t.TempDir(), "gemini.log"))
	t.Setenv("GEMINI_FAKE_FAIL", "1")

	_, err := GeminiRunner{}.Call(RunnerRequest{Root: repo, Prompt: "fail", Timeout: time.Minute})
	if err == nil || !strings.Contains(err.Error(), "gemini failed") || !strings.Contains(err.Error(), "login required") {
		t.Fatalf("GeminiRunner.Call error = %v, want stderr-preserving failure", err)
	}
}

func writeFakeGeminiExecutable(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "gemini")
	if runtime.GOOS == "windows" {
		path += ".cmd"
		body := `@echo off
powershell -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%~dp0gemini.ps1" %*
exit /b %ERRORLEVEL%
`
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatalf("write fake gemini cmd: %v", err)
		}
		ps1Path := filepath.Join(dir, "gemini.ps1")
		ps1Body := `if ($args.Count -gt 0 -and $args[0] -eq "--version") {
  Write-Output "gemini fake 1.2.3"
  exit 0
}
$prompt = [Console]::In.ReadToEnd()
Add-Content -Path $env:GEMINI_FAKE_LOG -Value ("ARGS:" + ($args -join " "))
Add-Content -Path $env:GEMINI_FAKE_LOG -Value ("ENV:" + $env:WS_MCP_TOOL_PROFILE)
Add-Content -Path $env:GEMINI_FAKE_LOG -Value ("STDIN:" + $prompt)
if ($env:GEMINI_FAKE_FAIL -eq "1") {
  [Console]::Error.WriteLine("login required")
  exit 41
}
[Console]::Error.WriteLine("fake diagnostic")
Write-Output "Gemini notice"
Write-Output '{"type":"init","init":{"session_id":"gemini-session"}}'
if ($env:GEMINI_FAKE_SLEEP_AFTER_INIT -eq "1") {
  Start-Sleep -Seconds 10
  exit 0
}
Write-Output '{"type":"message","message":{"role":"assistant","content":"gemini "}}'
Write-Output '{"type":"message","role":"assistant","content":"reply"}'
Write-Output '{"type":"result","result":{"status":"success"}}'
`
		if err := os.WriteFile(ps1Path, []byte(ps1Body), 0o755); err != nil {
			t.Fatalf("write fake gemini ps1: %v", err)
		}
		return path
	}
	body := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'gemini fake 1.2.3\n'
  exit 0
fi
prompt=$(cat)
{
  printf 'ARGS:%s\n' "$*"
  printf 'ENV:%s\n' "$WS_MCP_TOOL_PROFILE"
  printf 'STDIN:%s\n' "$prompt"
} >> "$GEMINI_FAKE_LOG"
if [ "$GEMINI_FAKE_FAIL" = "1" ]; then
  printf 'login required\n' >&2
  exit 41
fi
printf 'fake diagnostic\n' >&2
printf 'Gemini notice\n'
printf '{"type":"init","init":{"session_id":"gemini-session"}}\n'
if [ "$GEMINI_FAKE_SLEEP_AFTER_INIT" = "1" ]; then
  sleep 10
  exit 0
fi
printf '{"type":"message","message":{"role":"assistant","content":"gemini "}}\n'
printf '{"type":"message","role":"assistant","content":"reply"}\n'
printf '{"type":"result","result":{"status":"success"}}\n'
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake gemini: %v", err)
	}
	return path
}
