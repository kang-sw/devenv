package mcp

import (
	"sort"
	"strings"
)

// wsflowRenderEligibleStems is the exact set of prompt stems that are
// eligible for the wsflow playbook.render legacy context bridge.
// Add entries here as the spec expands the set.
var wsflowRenderEligibleStems = map[string]bool{
	"reference-discovery": true,
	"code-reviewer":       true,
}

func appendRenderContext(body string, context map[string]string) string {
	if len(context) == 0 {
		return body
	}
	keys := make([]string, 0, len(context))
	for k := range context {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString("\n\n## Render Context\n")
	for _, k := range keys {
		sb.WriteString("- ")
		sb.WriteString(k)
		sb.WriteString(": ")
		sb.WriteString(context[k])
		sb.WriteString("\n")
	}
	return body + sb.String()
}
