// Package claudeagents generates the Claude effort-carrier plugin agents.
//
// Claude Code's Agent tool takes a per-call model but no reasoning-effort
// parameter; effort comes only from a subagent definition's `effort`
// frontmatter. Each plugin package therefore ships one agent per Claude effort
// level, rendered from effort-agent.md.tmpl into <package>/claude-agents/, and
// the spawn passes the model on the call. The generated files are committed
// (marketplace installs take committed files only); a drift test keeps them
// equal to the template output.
package claudeagents

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

// Levels are the Claude effort levels, one generated agent each, in ascending
// order. The Claude SpawnIdiom names exactly this list as its mapping rule.
var Levels = []string{"low", "medium", "high", "xhigh", "max"}

//go:embed effort-agent.md.tmpl
var templateText string

var effortTemplate = template.Must(template.New("effort-agent").Option("missingkey=error").Parse(templateText))

// FileName returns the generated file name for level.
func FileName(level string) string {
	return "effort-" + level + ".md"
}

// Render returns the agent definition for level, which must be one of Levels.
func Render(level string) (string, error) {
	known := false
	for _, l := range Levels {
		if l == level {
			known = true
			break
		}
	}
	if !known {
		return "", fmt.Errorf("claudeagents: unknown effort level %q", level)
	}
	var buf bytes.Buffer
	if err := effortTemplate.Execute(&buf, struct{ Level string }{level}); err != nil {
		return "", fmt.Errorf("claudeagents: render %s: %w", level, err)
	}
	return buf.String(), nil
}
