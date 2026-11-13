package wsagent

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wslive"
)

// claudeReconnectLever ends the name of a tool a lead calls to ask for an
// MCP reconnect (the ws mod's request_mcp_reconnect, under any server).
const claudeReconnectLever = "__request_mcp_reconnect"

// claudeCompactionLever ends the name of the tool a lead calls to ask for a
// compaction at the end of its turn (the ws mod's request_compaction, under
// any server).
const claudeCompactionLever = "__request_compaction"

// claudeEffortType is an effort-carrier subagent type: one whose name,
// after any "<plugin>:" prefix, is effort-<level>.
var claudeEffortType = regexp.MustCompile(`(^|:)effort-(low|medium|high|xhigh|max)$`)

// claudeSpawnOf reads what an Agent call's input asks of its subagent.
func claudeSpawnOf(input json.RawMessage) claudeSpawn {
	var in struct {
		Model        string `json:"model"`
		SubagentType string `json:"subagent_type"`
	}
	_ = json.Unmarshal(input, &in)
	spawn := claudeSpawn{model: strings.TrimSpace(in.Model)}
	if m := claudeEffortType.FindStringSubmatch(strings.TrimSpace(in.SubagentType)); m != nil {
		spawn.effort = m[2]
	}
	return spawn
}

// claudeSub is a running subagent task the lead's Agent call started.
type claudeSub struct {
	task string
}

// subagentReply maps a subagent's assistant line to its telemetry: the
// model that answered and the request's context size (uncached input plus
// cache reads and writes). The stream carries no output rate: without
// partial messages, each content block arrives whole and its usage keeps
// the request's starting output count.
func (m *claudeMapper) subagentReply(l claudeLine, now int64) []Event {
	sub := m.subs[l.ParentToolUseID]
	if !m.lead() || sub == nil {
		return nil
	}
	var msg claudeMessage
	if json.Unmarshal(l.Message, &msg) != nil {
		return nil
	}
	ev := wslive.Event{Kind: wslive.KindAgent, AgentID: sub.task, Model: msg.Model}
	if u := msg.Usage; u != nil {
		tokens := float64(u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens)
		ev.ContextTokens = &tokens
	}
	if ev.Model == "" && ev.ContextTokens == nil {
		return nil
	}
	return []Event{{Live: &ev}}
}

// alert reads the typed failure a lead's assistant line carries: a login
// needed (authentication_failed), the model unavailable (model_not_found),
// or a usage limit (usage_limit_reached, or a rate_limit refusal with no
// other typed cause), with the line's text. Nil for any other line.
func (m *claudeMapper) alert(l claudeLine) *wslive.Notice {
	if !m.lead() {
		return nil
	}
	errKind, apiErr := rawString(l.Error), rawString(l.APIError)
	var n wslive.Notice
	switch {
	case errKind == "authentication_failed":
		n.Kind = wslive.NoticeLogin
	case errKind == "model_not_found":
		n.Kind = wslive.NoticeModel
	case apiErr == "usage_limit_reached" || (errKind == "rate_limit" && apiErr == ""):
		n.Kind = wslive.NoticeUsageLimit
		if p := l.APIErrorParams; p != nil && p.RateLimit != nil {
			m.rateLimit(*p.RateLimit)
		}
		n.ResetsAt = m.resetsAt()
	default:
		return nil
	}
	n.Text = claudeMessageText(l.Message)
	return &n
}

// rateLimit records which usage limits refuse requests, as rate-limit
// reports tell: rejected (and no overage covering it) adds the limit with
// its reset time, allowed removes it.
func (m *claudeMapper) rateLimit(info claudeRateLimit) {
	kind := info.RateLimitType
	if kind == "" {
		kind = "unknown"
	}
	overage := info.IsUsingOverage || info.OverageStatus == "allowed" || info.OverageStatus == "allowed_warning"
	switch {
	case info.Status == "rejected" && !overage:
		m.rejected[kind] = max(info.ResetsAt, 0) * 1000
	case info.Status == "allowed" || info.Status == "allowed_warning" || overage:
		delete(m.rejected, kind)
	}
}

// resetsAt is when every refusing usage limit has reset (epoch ms), or 0
// when one's reset time is unknown or none refuses.
func (m *claudeMapper) resetsAt() int64 {
	var latest int64
	for _, at := range m.rejected {
		if at == 0 {
			return 0
		}
		latest = max(latest, at)
	}
	return latest
}

// claudeMessageText is a message's text blocks, joined.
func claudeMessageText(raw json.RawMessage) string {
	var msg claudeMessage
	if json.Unmarshal(raw, &msg) != nil {
		return ""
	}
	return claudeResultText(msg.Content)
}

// rawString is a raw JSON value as a string: a string's own text, a
// number's digits, or "" for anything else.
func rawString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}

// rawInt is a raw JSON value as a non-negative int: a number or a numeric
// string; 0 for anything else.
func rawInt(raw json.RawMessage) int {
	n, err := strconv.ParseFloat(rawString(raw), 64)
	if err != nil || n < 0 || n > 1e9 {
		return 0
	}
	return int(n)
}
