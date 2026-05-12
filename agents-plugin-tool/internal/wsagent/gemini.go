package wsagent

import (
	"fmt"
	"io"
)

// CONTRACT: GeminiRunner is the ws named-agent backend adapter for Gemini CLI.
// It must keep registration, async lifecycle, diagnostics, cancel, and recall
// behavior on the shared Manager path; only Gemini invocation and stream-json
// parsing belong here.
type GeminiRunner struct{}

type geminiInvocation struct {
	Args           []string
	PromptStdin    string
	PromptDelivery string
}

func (GeminiRunner) Call(req RunnerRequest) (RunnerResult, error) {
	// CONTRACT: Invoke `gemini --output-format stream-json --approval-mode yolo`,
	// pass concrete model and resume args when present, send the final prompt via
	// stdin, preserve stdout/stderr diagnostic streams, and return RunnerResult
	// only after a terminal Gemini result event. Capture a Gemini CLI version when
	// the local binary exposes one and surface it through RunnerResult.
	// Mirror CodexRunner subprocess setup for stdout teeing, timeout, ToolProfile,
	// process-group handling, and OnSessionID timing.
	// HOLE: Implement Gemini CLI subprocess execution and stream parser hookup.
	return RunnerResult{}, fmt.Errorf("gemini runner not implemented")
}

func buildGeminiInvocation(req RunnerRequest) (geminiInvocation, error) {
	// CONTRACT: Build argv without leaking prompt contents into argv.
	// Start with `gemini --output-format stream-json --approval-mode yolo`; append
	// `-m <model>` for non-shorthand concrete models and `--resume <session_id>`
	// for resumed calls.
	// HOLE: Read SystemPromptPath and prepend a clear system-instruction block to
	// PromptStdin when one is present.
	return geminiInvocation{}, fmt.Errorf("gemini invocation builder not implemented")
}

func parseGeminiStreamJSON(r io.Reader, onSessionID func(string) error) (RunnerResult, error) {
	// CONTRACT: Ignore non-JSON stdout notices, capture init.session_id, append
	// assistant message.content chunks in order, require result.status=="success",
	// and fail on terminal error, missing terminal result, missing session id, or
	// missing assistant text.
	// Use bufio.Reader.ReadBytes like Codex JSONL parsing so large event lines are
	// supported.
	// HOLE: Parse Gemini stream-json events and call onSessionID immediately after
	// the first init.session_id is available.
	return RunnerResult{}, fmt.Errorf("gemini stream-json parser not implemented")
}
