package wsagent

import "testing"

func TestParseGeminiStreamJSONSkeletonTargets(t *testing.T) {
	t.Skip("skeleton: implement Gemini stream-json parser tests")
	// CONTRACT: Cover init.session_id capture, assistant message.content chunk
	// accumulation, terminal success, terminal error, tool events, non-JSON stdout
	// noise, missing terminal result, missing session id, and missing assistant text.
}

func TestBuildGeminiInvocationSkeletonTargets(t *testing.T) {
	t.Skip("skeleton: implement Gemini invocation builder tests")
	// CONTRACT: Cover stream-json/yolo args, concrete model arg, resume arg,
	// stdin prompt delivery, no prompt leakage into argv, and system prompt
	// prepending from SystemPromptPath.
}

func TestGeminiRunnerSkeletonTargets(t *testing.T) {
	t.Skip("skeleton: implement Gemini runner subprocess tests")
	// CONTRACT: Cover fake gemini executable execution, stdout diagnostic teeing,
	// stderr preservation on nonzero exit, ToolProfile env propagation, and
	// OnSessionID callback timing. Cover backend version capture when the fake
	// executable receives the selected version flag.
}
