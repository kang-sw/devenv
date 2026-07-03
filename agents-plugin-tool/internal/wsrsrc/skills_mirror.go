package wsrsrc

import (
	"fmt"
	"strings"
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
	out := strings.ReplaceAll(source, "ws:", "wsflow:")
	out = strings.ReplaceAll(out, "ws/", "wsflow/")
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
