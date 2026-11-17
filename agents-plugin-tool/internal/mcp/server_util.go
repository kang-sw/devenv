package mcp

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func rawMessageString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return string(raw)
}

func wantsJSON(arguments map[string]any) bool {
	format, _ := arguments["format"].(string)
	return strings.EqualFold(strings.TrimSpace(format), "json")
}

func formatStringLines(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, "\n") + "\n"
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// capitalizeFirst upper-cases only the first byte, matching the capitalized-
// sentence convention formatSageGate's sibling fields use. It intentionally
// leaves the shared sageReviewNonWaivableAdvisory constant itself lowercase
// (tickets_mutate.go) so its other embedding — mid-sentence inside
// readySagePostureWarning's Tip-channel text — stays grammatically correct;
// only this display site capitalizes.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func writeIndentedLines(b *strings.Builder, prefix string, lines []string) {
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fmt.Fprintf(b, "%s%s\n", prefix, strings.TrimSpace(line))
	}
}

func displayString(value any) string {
	text, _ := value.(string)
	return displayOrDash(text)
}

func displayOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func boolValue(value any) bool {
	got, _ := value.(bool)
	return got
}

func shortHash(value string) string {
	if len(value) > 7 && isHexString(value) {
		return value[:7]
	}
	return value
}

func isHexString(value string) bool {
	for _, ch := range value {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return false
	}
	return value != ""
}

func toolJSONResponse(id json.RawMessage, value any, err error) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	return toolTextResponse(id, string(raw)+"\n", nil)
}

func toolTextResponse(id json.RawMessage, text string, err error) response {
	if err != nil {
		return toolErrorTextResponse(id, err.Error())
	}
	return response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []map[string]string{{
			"type": "text",
			"text": text,
		}},
	}}
}

func toolErrorTextResponse(id json.RawMessage, text string) response {
	return response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"isError": true,
		"content": []map[string]string{{
			"type": "text",
			"text": text,
		}},
	}}
}

func errorResponse(id json.RawMessage, code int, message string) response {
	return response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

func stringMapArgument(value any) map[string]string {
	items, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(items))
	for key, value := range items {
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}
	return out
}

func int64FromArgument(value any, fallback int64) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func optString(value any) string {
	text, _ := value.(string)
	return text
}

// typedArrayRejection returns a type-accurate error when a caller supplied a
// present, non-null value for an array-typed argument that is not a JSON
// array — for example a dash-bulleted prose string, the shape a commit
// message's `## AI Context` block takes everywhere else in the workflow,
// passed where ai_context expects one bullet per array element. It returns
// nil when the value is absent, explicit null, or already an array; those
// cases fall through unchanged to stringList/stringListKeepBlank's existing
// empty-vs-absent classification. Applied per call site (git.commit's and
// git.merge's ai_context) rather than inside stringList/stringListKeepBlank
// themselves: those helpers are also shared by other array-typed args
// (updated_tickets, stems, note/todo arrays) whose mismatch shape this fix's
// evidence does not cover, so a helper-level reject would widen the blast
// radius beyond what was verified.
func typedArrayRejection(argName string, value any, present bool) error {
	if !present || value == nil {
		return nil
	}
	if _, ok := value.([]any); ok {
		return nil
	}
	return fmt.Errorf("%s must be an array of strings, one bullet per element; got %s", argName, jsonValueTypeName(value))
}

// jsonValueTypeName names a decoded JSON-RPC argument's dynamic type for a
// type-accurate rejection message. Go's encoding/json decodes into exactly
// these dynamic types for a map[string]any argument value.
func jsonValueTypeName(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func stringList(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if ok && text != "" {
			out = append(out, text)
		}
	}
	return out
}

// stringListKeepBlank mirrors stringList's nil-vs-empty-array and
// type-checking behavior but keeps blank ("" and whitespace-only) string
// entries instead of dropping them. Callers that need to classify a present
// array by blankness themselves (e.g. wsgit.normalizeCommitOptions's
// TrimSpace-based emptiness check) must receive the untouched entries, or an
// exact-empty-string entry becomes indistinguishable from an entry that was
// never present at all.
func stringListKeepBlank(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func boolArgument(value any) bool {
	result, _ := value.(bool)
	return result
}

func exactIntegerArgument(arguments map[string]any, name string, fallback int) (int, error) {
	raw, ok := arguments[name]
	if !ok {
		return fallback, nil
	}
	var value float64
	switch typed := raw.(type) {
	case float64:
		value = typed
	case int:
		value = float64(typed)
	default:
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	maxInt := int(^uint(0) >> 1)
	if value >= float64(maxInt) {
		return maxInt, nil
	}
	return int(value), nil
}

func hasSpecStemArgument(arguments map[string]any) bool {
	_, ok := arguments["spec_stem"]
	return ok
}

func durationFromSeconds(value any) time.Duration {
	switch v := value.(type) {
	case float64:
		if v <= 0 {
			return 0
		}
		return time.Duration(v * float64(time.Second))
	case string:
		if v == "" {
			return 0
		}
		duration, err := time.ParseDuration(v)
		if err == nil {
			return duration
		}
		seconds, err := strconv.ParseFloat(v, 64)
		if err != nil || seconds <= 0 {
			return 0
		}
		return time.Duration(seconds * float64(time.Second))
	default:
		return 0
	}
}

func intFromArgument(value any, fallback int) int {
	switch v := value.(type) {
	case float64:
		if v <= 0 {
			return fallback
		}
		return int(v)
	case string:
		if v == "" {
			return fallback
		}
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}
