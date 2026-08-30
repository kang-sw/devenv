package wsrsrc

import (
	"fmt"
	"regexp"
	"strings"
)

// wsColonPattern and wsSlashPattern anchor the ws: / ws/ namespace-token
// substitution to a left-side word boundary so it only matches "ws:"/"ws/" as
// a standalone token, not as a substring tail of a longer word (e.g. "shows:"
// or "draws/"). \b is zero-width, so it does not consume input and cannot
// cause overlapping-match or non-consumption issues for adjacent tokens like
// "ws:ws:".
//
// wsCliPattern and wsMCPPattern are literal-token substitutions (not
// namespace-prefix rules like the other two): wsCliPattern rewrites the
// standalone token "ws-cli" to "wsflow-cli", and wsMCPPattern rewrites the
// space-separated prose token "ws MCP" to "wsflow MCP". Neither may be
// broadened to match "ws-mcp" or "ws-plugin", and wsMCPPattern in particular
// must never be widened to a bare `\bws\b` rule: the literal space is what
// keeps it off the "ws-mcp" binary name (e.g. ws-mcp-launcher.py), which the
// wsflow mirror preserves verbatim.
var (
	wsColonPattern = regexp.MustCompile(`\bws:`)
	wsSlashPattern = regexp.MustCompile(`\bws/`)
	wsCliPattern   = regexp.MustCompile(`\bws-cli\b`)
	wsMCPPattern   = regexp.MustCompile(`\bws MCP\b`)
)

// disqualifyingTokens are the tokens that make a source SKILL.md ineligible
// for substitution-mirrored generation. Presence of any of these anywhere in
// the pre-substitution text is a hard failure — there is no marker-exception
// path. Eligible sources must contain nothing beyond ws:/ws/ namespace tokens.
var disqualifyingTokens = []string{
	"mercenary",
	"<!-- ws:full-only:",
	"<!-- ws:wsflow-only:",
	"ws.",
	"lead-write-code",
	"lead-write-skeleton",
	"lead-salvage",
	"lead-skill-authoring",
}

// GenerateWsflowSkillBody applies the substitution-mirrored generation
// mechanism to a full-ws SKILL.md source: it validates the source against the
// disqualifying-token guard, then applies literal namespace substitution
// (ws: -> wsflow:, ws/ -> wsflow/) over the full raw text, frontmatter
// included. Returns an error without producing output if the guard rejects
// the source.
func GenerateWsflowSkillBody(source string) (string, error) {
	if err := guardSubstitutionEligible(source); err != nil {
		return "", err
	}
	out := wsColonPattern.ReplaceAllString(source, "wsflow:")
	out = wsSlashPattern.ReplaceAllString(out, "wsflow/")
	out = wsCliPattern.ReplaceAllString(out, "wsflow-cli")
	out = wsMCPPattern.ReplaceAllString(out, "wsflow MCP")
	return out, nil
}

// guardSubstitutionEligible fails loudly if source contains anything beyond
// namespace-only tokens that a blind substitution would mishandle. This is a
// strict, conservative denylist, not a curated exception list: any hit is
// disqualifying, full stop.
func guardSubstitutionEligible(source string) error {
	lower := strings.ToLower(source)
	for _, tok := range disqualifyingTokens {
		if strings.Contains(lower, strings.ToLower(tok)) {
			return fmt.Errorf("source contains disqualifying token %q; substitution-mirrored generation requires namespace-only content", tok)
		}
	}
	return nil
}
