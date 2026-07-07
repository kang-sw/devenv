package mcp

import (
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdoc"
)

// docCoverageWarning computes the session-bootstrap doc-coverage banner, or ""
// when no warning should be surfaced. Silent cases (by design, not a bug):
//   - ItemDocCoverageAlarm resolves to "off".
//   - Both ai-docs/spec/ and ai-docs/mental-model/ already contain at least
//     one .md file with a parsed frontmatter block.
//
// The check is purely a project-local ai-docs/ scan (no skillsRoot/package
// template comparison), unlike bootstrapStalenessWarning.
func docCoverageWarning(root string, resolver *wsconfig.Resolver, sessionKey string) string {
	rv, err := resolver.Get(sessionKey, wsconfig.ItemDocCoverageAlarm)
	if err == nil && rv.Value == "off" {
		return ""
	}

	hasSpec := wsdoc.SpecAreaHasFrontmatterFile(root)
	hasMentalModel := wsdoc.MentalModelAreaHasFrontmatterFile(root)
	if hasSpec && hasMentalModel {
		return ""
	}

	var missing string
	switch {
	case !hasSpec && !hasMentalModel:
		missing = "ai-docs/spec/ and ai-docs/mental-model/ have no"
	case !hasSpec:
		missing = "ai-docs/spec/ has no"
	default:
		missing = "ai-docs/mental-model/ has no"
	}

	return "> **Doc coverage is missing.** This project's " + missing +
		" .md file carrying a frontmatter block. Run lead-forge-spec/lead-forge-mental-model to populate it, or run " +
		"`config.doc_coverage_alarm(value: \"off\")` to silence this permanently."
}

// injectDocCoverageWarning prepends warning to body, delegating to the
// existing generic prepend-if-nonempty helper rather than duplicating it.
func injectDocCoverageWarning(body, warning string) string {
	return injectBootstrapStalenessWarning(body, warning)
}
