package mcp

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// handlerOwnedArguments lists, per tool, top-level argument names a handler
// reads although the advertised inputSchema does not declare them. When one
// is present the precheck defers the whole call to the handler, so the
// handler's own response stays the answer:
//
//   - relogin_session_key is the bootstrap tool's deliberately unadvertised
//     re-login input for harness adapters (see handleLeadRelogin); declaring
//     it would advertise it, refusing it would break adapter re-entry.
//   - sparse_paths and spec_stem are retired names whose handlers answer with
//     a specific migration hint (profile, ticket_stem) that a generic
//     unknown-argument refusal would lose.
//
// Add a name here only for one of those two reasons; a real caller-facing
// input belongs in the tool's inputSchema instead.
var handlerOwnedArguments = map[string][]string{
	bootstrapToolName:      {"relogin_session_key"},
	"worktree.acquire":     {"sparse_paths"},
	"tickets.query":        {"spec_stem"},
	"tickets.close":        {"spec_stem"},
	"tickets.move":         {"spec_stem"},
	"tickets.create_empty": {"spec_stem"},
	"tickets.sage_gate":    {"spec_stem"},
	"tickets.sage_stamp":   {"spec_stem"},
	"tickets.verify":       {"spec_stem"},
}

// handlerGuidedRequired lists, per tool, required properties whose absence
// the handler answers with runtime guidance a static schema cannot carry, so
// the precheck defers a call missing one to the handler. session_key is
// implicitly on every tool's list: its handler error names the route to
// obtain a key. worktree.acquire's profile error lists the profiles resolved
// for this project, including project-declared ones the schema never names.
var handlerGuidedRequired = map[string][]string{
	"worktree.acquire": {"profile"},
}

// toolInputSchema returns the raw inputSchema of the named tool from the same
// definitions tools/list serves, or nil when the tool has no definition (a
// removed tool whose handler only answers with a migration error).
func toolInputSchema(name string) map[string]any {
	for _, tool := range tools() {
		if toolName, _ := tool["name"].(string); toolName == name {
			schema, _ := tool["inputSchema"].(map[string]any)
			return schema
		}
	}
	return nil
}

// precheckToolArguments validates the top level of a tools/call arguments
// object against the tool's inputSchema before its handler runs: every
// `required` property must be present and every argument name must be a
// declared property. It checks names only - no types, no nested objects - and
// returns ok=false with the refusal text when the call must not run.
func precheckToolArguments(name string, arguments map[string]any) (string, bool) {
	schema := toolInputSchema(name)
	if schema == nil {
		return "", true
	}
	required := schemaStringList(schema["required"])
	// A missing handler-guided required property defers the whole call to
	// the handler: its error carries guidance a static schema cannot (see
	// handlerGuidedRequired). The retry then meets this check.
	for _, property := range append([]string{"session_key"}, handlerGuidedRequired[name]...) {
		if _, present := arguments[property]; !present && slices.Contains(required, property) {
			return "", true
		}
	}
	// A handler-owned argument likewise defers the call, so the handler's
	// re-login or migration answer is the one the caller sees.
	for _, argument := range handlerOwnedArguments[name] {
		if _, present := arguments[argument]; present {
			return "", true
		}
	}
	properties, _ := schema["properties"].(map[string]any)

	var missing []string
	for _, property := range required {
		if _, present := arguments[property]; !present {
			missing = append(missing, property)
		}
	}

	var unknown []string
	if schemaClosed(schema) {
		for argument := range arguments {
			if _, declared := properties[argument]; declared {
				continue
			}
			// callTool itself reads session_key on every call (the role
			// gate and the unread-mail badge), so it is a valid argument
			// for every tool whether or not that tool's schema declares it.
			if argument == "session_key" {
				continue
			}
			unknown = append(unknown, argument)
		}
		sort.Strings(unknown)
	}

	if len(missing) == 0 && len(unknown) == 0 {
		return "", true
	}
	// One generic event per refusal keeps dropped-field diagnosis visible in
	// runtime.debug_events for every tool, since the handler never runs.
	appendDebugEvent("tools.call.precheck_refused", map[string]any{
		"tool":    name,
		"missing": nonNilStrings(missing),
		"unknown": nonNilStrings(unknown),
	})
	return toolArgumentsRefusal(name, schema, missing, unknown), false
}

// schemaClosed reports whether the unknown-argument check applies. It does
// when the top level leaves additionalProperties absent (a declared
// properties map is then read as the complete argument set) or sets it to
// false. Any other value - true or a schema for extra properties - declares
// open-ended top-level arguments, so names outside properties are the
// handler's to judge.
func schemaClosed(schema map[string]any) bool {
	additional, present := schema["additionalProperties"]
	if !present {
		return true
	}
	closed, isBool := additional.(bool)
	return isBool && !closed
}

func schemaStringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// toolArgumentsRefusal names every missing required property and every
// unknown argument, then appends the called tool's own advertised inputSchema
// (never the full tool list) so the caller can correct the call in one retry.
func toolArgumentsRefusal(name string, schema map[string]any, missing, unknown []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: invalid arguments; the tool did not run.\n", name)
	if len(missing) > 0 {
		fmt.Fprintf(&b, "missing required: %s\n", strings.Join(missing, ", "))
	}
	if len(unknown) > 0 {
		fmt.Fprintf(&b, "unknown arguments: %s\n", strings.Join(unknown, ", "))
	}
	b.WriteString("Retry with arguments matching this inputSchema:\n")
	raw, err := json.Marshal(namespaceValue(schema))
	if err != nil {
		fmt.Fprintf(&b, "(inputSchema unavailable: %v)\n", err)
		return b.String()
	}
	b.Write(raw)
	b.WriteString("\n")
	return b.String()
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
