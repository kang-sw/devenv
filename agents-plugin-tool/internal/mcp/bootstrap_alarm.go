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

// templateVersionMarker detects the presence of a Template Version marker
// comment independent of whether its value parses as vNNNN. It exists only
// to let bootstrapStalenessWarning distinguish "no marker at all" (silent —
// the project never opted into the bootstrap contract) from "marker present
// but the value is unparseable" (must fire). It is not used for the running
// package's own template head, which still requires a strict parse via
// readTemplateVersion/latestKnownTemplateVersion.
var templateVersionMarker = regexp.MustCompile(`<!--\s*Template Version:`)

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

// readInstalledVersionState reads path and reports its Template Version
// marker state for bootstrapStalenessWarning's installed-tag side: version
// and parsed mirror parseTemplateVersionTag, and markerPresent additionally
// reports whether a Template Version marker comment exists at all, even when
// its value does not parse as vNNNN. A missing file reports
// markerPresent=false, matching the "never opted in" treatment.
func readInstalledVersionState(path string) (version int, parsed, markerPresent bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false, false
	}
	content := string(data)
	version, parsed = parseTemplateVersionTag(content)
	markerPresent = parsed || templateVersionMarker.MatchString(content)
	return version, parsed, markerPresent
}

// latestKnownTemplateVersion resolves the "latest known version" for the
// package whose MCP binary is currently running, by reading the shipped
// lead-bootstrap AGENTS.template.md under skillsRoot. skillsRoot is already
// package-scoped (ws vs wsflow) via wsrsrc.ResolveSkillsRoot, so no explicit
// package branching is needed here.
func latestKnownTemplateVersion(skillsRoot string) (int, bool) {
	return readTemplateVersion(filepath.Join(skillsRoot, "lead-bootstrap", "AGENTS.template.md"))
}

// bootstrapStalenessWarning computes the session-bootstrap staleness/skew
// banner, or "" when no warning should be surfaced. This function is a
// detector only: it never modifies AGENTS.md/the version tag, and the
// warnings it returns must say so — enforcement of the above-head/unknown-tag
// refuse is a lead-bootstrap skill instruction, not code. Silent cases (by
// design, not a bug):
//   - ItemBootstrapAlarm resolves to "off".
//   - The downstream root has no AGENTS.md, or it has no Template Version
//     marker at all (an untagged project never opted into the ws bootstrap
//     contract).
//   - The shipped template's own tag is unreadable/malformed (fail-safe: never
//     warn off of an unreadable "latest").
//   - The installed version equals the latest known version (current).
//
// Firing cases, compared against the running package's own template head:
//   - installed < latest: existing "stale" warning (upgrade path, text
//     unchanged).
//   - installed > latest: new "above-head" warning.
//   - Template Version marker present but its value does not parse as
//     `vNNNN`: new "unrecognized tag" warning.
func bootstrapStalenessWarning(root, skillsRoot string, resolver *wsconfig.Resolver, sessionKey string) string {
	rv, err := resolver.Get(sessionKey, wsconfig.ItemBootstrapAlarm)
	if err == nil && rv.Value == "off" {
		return ""
	}

	installed, parsed, markerPresent := readInstalledVersionState(filepath.Join(root, "AGENTS.md"))
	if !markerPresent {
		return ""
	}

	latest, ok := latestKnownTemplateVersion(skillsRoot)
	if !ok {
		return ""
	}

	if !parsed {
		return fmt.Sprintf(
			"> **Bootstrap template tag is unrecognized.** This project's AGENTS.md has a Template Version marker whose value does not parse as `vNNNN`; this package's shipped lead-bootstrap template head is v%04d. Do not reconcile or restamp — this check is a code-level detector only, not a mechanical block; lead-bootstrap must stop and report rather than auto-fix. Leave the artifact and tag unchanged, or run `config.tune(key: \"bootstrap_alarm\", value: \"off\")` to silence this permanently.",
			latest,
		)
	}

	if installed > latest {
		return fmt.Sprintf(
			"> **Bootstrap template tag is ahead of this package's own template head.** This project's AGENTS.md is at v%04d; this package's shipped lead-bootstrap template head is v%04d. Do not reconcile or restamp — this check is a code-level detector only, not a mechanical block; lead-bootstrap must stop and report rather than auto-fix. Leave the artifact and tag unchanged, or run `config.tune(key: \"bootstrap_alarm\", value: \"off\")` to silence this permanently.",
			installed, latest,
		)
	}

	if installed == latest {
		return ""
	}

	return fmt.Sprintf(
		"> **Bootstrap template is stale.** This project's AGENTS.md is at v%04d; the shipped lead-bootstrap template is at v%04d. Re-run lead-bootstrap to pick up the latest workflow template, or run `config.tune(key: \"bootstrap_alarm\", value: \"off\")` to silence this permanently.",
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
