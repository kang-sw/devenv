package wsrsrc

import (
	"fmt"
	"html"
	"strings"
)

// SkillSplice declares one build-time skill-body composition: Source's
// SKILL.md body is spliced into Target's committed SKILL.md, wrapped in the
// same <playbook> boundary the serve-time concatenation hooks emit, anchored
// immediately before AnchorHeading.
//
// This is composition, not the namespace substitution GenerateWsflowSkillBody
// performs, and the two run in a fixed order: compose the full-ws source
// first, then mirror the composed result to wsflow. The reverse order would
// leak ws-namespace text into the wsflow package.
type SkillSplice struct {
	// Target is the skill whose committed SKILL.md carries the region.
	Target string
	// Source is the skill whose body is spliced in, verbatim and
	// frontmatter-stripped (see LoadSkillBody).
	Source string
	// Title is the human-readable title in the <playbook> boundary. It must
	// match the serve-time title for the same body so the two paths emit one
	// boundary shape.
	Title string
	// AnchorHeading is the markdown heading line the region is inserted
	// before on first generation. It is consulted only when no region exists
	// yet; later generations locate the region by its delimiter pair.
	AnchorHeading string
}

const spliceCloseTag = "</playbook>"

// WrapForConcatenation wraps an already-loaded body in the visible
// <playbook name=... title=...> boundary used for code-side pragmatic
// concatenation. It does not load or parse source text.
//
// Both the serve-time path (internal/mcp printPlaybook) and the build-time
// path (ComposeSkillBody) call this, so one boundary shape has exactly one
// implementation.
func WrapForConcatenation(name, title, body string) string {
	trimmedBody := strings.TrimRight(body, "\n")
	return fmt.Sprintf(
		"<playbook name=\"%s\" title=\"%s\">\n%s\n</playbook>",
		html.EscapeString(name),
		html.EscapeString(title),
		trimmedBody,
	)
}

// ComposeSkillBody returns target carrying exactly one spliced region for
// splice, built from sourceBody.
//
// Generation is idempotent: when target already contains a region for
// splice.Source it is located by its <playbook>/</playbook> delimiter pair and
// replaced in place; only when no region exists is AnchorHeading consulted and
// the region inserted before it. Composing an already-composed target is
// therefore a no-op.
//
// target is the full raw SKILL.md text, frontmatter included — the frontmatter
// is never touched because insertion is anchored to a body heading.
func ComposeSkillBody(target string, splice SkillSplice, sourceBody string) (string, error) {
	region := WrapForConcatenation(splice.Source, splice.Title, sourceBody)
	openTag := fmt.Sprintf("<playbook name=%q", html.EscapeString(splice.Source))

	var composed string
	switch n := strings.Count(target, openTag); {
	case n > 1:
		return "", fmt.Errorf("compose %s: found %d spliced regions for %q, expected at most 1", splice.Target, n, splice.Source)
	case n == 1:
		start := strings.Index(target, openTag)
		rel := strings.Index(target[start:], spliceCloseTag)
		if rel < 0 {
			return "", fmt.Errorf("compose %s: spliced region for %q has no closing %s", splice.Target, splice.Source, spliceCloseTag)
		}
		end := start + rel + len(spliceCloseTag)
		composed = target[:start] + region + target[end:]
	default:
		anchor := "\n" + splice.AnchorHeading + "\n"
		if got := strings.Count(target, anchor); got != 1 {
			return "", fmt.Errorf("compose %s: anchor heading %q appears %d times in the target, expected exactly 1", splice.Target, splice.AnchorHeading, got)
		}
		at := strings.Index(target, anchor) + 1
		composed = target[:at] + region + "\n\n" + target[at:]
	}

	// Every splice target is also a substitution-mirrored skill, so content the
	// wsflow mirror would reject must fail here, at composition, rather than one
	// generation step later where the error would name the composed artifact
	// instead of the source that introduced the token.
	if err := guardSubstitutionEligible(composed); err != nil {
		return "", fmt.Errorf("compose %s: %w", splice.Target, err)
	}
	return composed, nil
}
