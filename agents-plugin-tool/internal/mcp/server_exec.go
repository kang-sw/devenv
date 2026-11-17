package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/execjob"
)

func execResponse(id json.RawMessage, r execjob.Response, err error, includeStreams bool) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	return toolTextResponse(id, formatExecResponse(r, includeStreams), nil)
}

func formatExecResponse(r execjob.Response, includeStreams bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "exec_key: %s\n", r.ExecKey)
	fmt.Fprintf(&b, "status: %s\n", r.Status)
	fmt.Fprintf(&b, "result_ready: %t\n", r.ResultReady)
	if r.PID != 0 {
		fmt.Fprintf(&b, "pid: %d\n", r.PID)
	}
	if r.ExitCode != 0 || r.ResultReady {
		fmt.Fprintf(&b, "exit_code: %d\n", r.ExitCode)
	}
	if r.StartedAt != "" {
		fmt.Fprintf(&b, "started_at: %s\n", r.StartedAt)
	}
	if r.UpdatedAt != "" {
		fmt.Fprintf(&b, "updated_at: %s\n", r.UpdatedAt)
	}
	if r.CompletedAt != "" {
		fmt.Fprintf(&b, "completed_at: %s\n", r.CompletedAt)
	}
	fmt.Fprintf(&b, "stdout_bytes: %d\n", r.StdoutBytes)
	fmt.Fprintf(&b, "stderr_bytes: %d\n", r.StderrBytes)
	fmt.Fprintf(&b, "combined_bytes: %d\n", r.CombinedBytes)
	if r.Error != "" {
		fmt.Fprintf(&b, "error: %s\n", r.Error)
	}
	if r.Guidance != "" {
		fmt.Fprintf(&b, "guidance: %s\n", r.Guidance)
	}
	if includeStreams && (r.Stdout != "" || r.Stderr != "") {
		b.WriteString("========== stdout ==========" + "\n")
		b.WriteString(r.Stdout)
		if r.Stdout != "" && !strings.HasSuffix(r.Stdout, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("========== stderr ==========" + "\n")
		b.WriteString(r.Stderr)
		if r.Stderr != "" && !strings.HasSuffix(r.Stderr, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func execRawTailResponse(id json.RawMessage, r execjob.RawTailResponse, err error) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "exec_key: %s\n", r.ExecKey)
	fmt.Fprintf(&b, "stream: %s\n", r.Stream)
	b.WriteString("========== text ==========" + "\n")
	b.WriteString(r.Text)
	if r.Text != "" && !strings.HasSuffix(r.Text, "\n") {
		b.WriteString("\n")
	}
	return toolTextResponse(id, b.String(), nil)
}

func execRawReadResponse(id json.RawMessage, r execjob.RawReadResponse, err error) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "exec_key: %s\n", r.ExecKey)
	fmt.Fprintf(&b, "stream: %s\n", r.Stream)
	fmt.Fprintf(&b, "offset: %d\n", r.Offset)
	fmt.Fprintf(&b, "next_offset: %d\n", r.NextOffset)
	fmt.Fprintf(&b, "limit: %d\n", r.Limit)
	fmt.Fprintf(&b, "size: %d\n", r.Size)
	fmt.Fprintf(&b, "eof: %t\n", r.EOF)
	b.WriteString("========== text ==========" + "\n")
	b.WriteString(r.Text)
	if r.Text != "" && !strings.HasSuffix(r.Text, "\n") {
		b.WriteString("\n")
	}
	return toolTextResponse(id, b.String(), nil)
}

func execRawGrepResponse(id json.RawMessage, r execjob.RawGrepResponse, err error) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "exec_key: %s\n", r.ExecKey)
	fmt.Fprintf(&b, "stream: %s\n", r.Stream)
	fmt.Fprintf(&b, "matches: %d\n", len(r.Matches))
	fmt.Fprintf(&b, "truncated: %t\n", r.Truncated)
	b.WriteString("========== matches ==========" + "\n")
	for i, m := range r.Matches {
		if i > 0 {
			b.WriteString("--\n")
		}
		for _, line := range m.Before {
			fmt.Fprintf(&b, "%d- %s\n", m.Line, line)
		}
		fmt.Fprintf(&b, "%d: %s\n", m.Line, m.Text)
		for _, line := range m.After {
			fmt.Fprintf(&b, "%d+ %s\n", m.Line, line)
		}
	}
	return toolTextResponse(id, b.String(), nil)
}
