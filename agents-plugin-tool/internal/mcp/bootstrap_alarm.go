package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// templateVersionTag matches the downstream/template marker comment
// `<!-- Template Version: vNNNN -->` used both by a downstream project's root
// AGENTS.md and by each package's shipped agents-plugin-wsflow/agents-plugin
// AGENTS.template.md. The same regex parses both sides of the staleness
// comparison.
var templateVersionTag = regexp.MustCompile(`<!--\s*Template Version:\s*v(\d+)\s*-->`)

// parseTemplateVersionTag extracts the numeric version from a Template
// Version marker comment. It returns (0, false) when the tag is absent or
// malformed — callers must treat this as "no signal", not "version 0".
func parseTemplateVersionTag(content string) (int, bool) {
	m := templateVersionTag.FindStringSubmatch(content)
	if m == nil {
		return 0, false
	}
	var version int
	if _, err := fmt.Sscanf(m[1], "%d", &version); err != nil {
		return 0, false
	}
	return version, true
}

// readTemplateVersion reads path and extracts its Template Version tag. A
// missing file is treated the same as a missing tag: (0, false).
func readTemplateVersion(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	return parseTemplateVersionTag(string(data))
}

// latestKnownTemplateVersion resolves the "latest known version" for the
// package whose MCP binary is currently running, by reading the shipped
// lead-bootstrap AGENTS.template.md under skillsRoot. skillsRoot is already
// package-scoped (ws vs wsflow) via wsrsrc.ResolveSkillsRoot, so no explicit
// package branching is needed here.
func latestKnownTemplateVersion(skillsRoot string) (int, bool) {
	return readTemplateVersion(filepath.Join(skillsRoot, "lead-bootstrap", "AGENTS.template.md"))
}

// bootstrapStalenessWarning computes the session-bootstrap staleness banner,
// or "" when no warning should be surfaced. Silent cases (by design, not a
// bug):
//   - ItemBootstrapAlarm resolves to "off".
//   - The downstream root has no AGENTS.md, or it has no Template Version tag
//     (an untagged project never opted into the ws bootstrap contract).
//   - The shipped template's own tag is unreadable/malformed (fail-safe: never
//     warn off of an unreadable "latest").
//   - The installed version is already at or above the latest known version.
func bootstrapStalenessWarning(root, skillsRoot string, resolver *wsconfig.Resolver, sessionKey string) string {
	rv, err := resolver.Get(sessionKey, wsconfig.ItemBootstrapAlarm)
	if err == nil && rv.Value == "off" {
		return ""
	}

	installed, ok := readTemplateVersion(filepath.Join(root, "AGENTS.md"))
	if !ok {
		return ""
	}

	latest, ok := latestKnownTemplateVersion(skillsRoot)
	if !ok {
		return ""
	}

	if installed >= latest {
		return ""
	}

	return fmt.Sprintf(
		"> **Bootstrap template is stale.** This project's AGENTS.md is at v%04d; the shipped lead-bootstrap template is at v%04d. Re-run lead-bootstrap to pick up the latest workflow template, or run `config.bootstrap_alarm(value: \"off\")` to silence this permanently.",
		installed, latest,
	)
}

// injectBootstrapStalenessWarning prepends warning to body, followed by a
// blank line separator. It is a no-op passthrough when warning is empty,
// mirroring injectSkepticalPosture's shape in workflow_manual.go.
func injectBootstrapStalenessWarning(body, warning string) string {
	if warning == "" {
		return body
	}
	return warning + "\n\n" + body
}
