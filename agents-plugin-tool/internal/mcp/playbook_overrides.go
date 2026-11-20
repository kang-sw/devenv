// playbook_overrides.go parses and applies playbook prompt overrides.
package mcp

import (
	"io/fs"
	"sort"
	"strings"
)

// overrideLookupFn is an injectable function for resolving override values for a
// named override-point and harness. It returns the override text and true when a
// value is stored for the (pointId, harness) pair, or ("", false) to signal that
// no override is stored and the inline seed default should be used.
//
// A nil overrideLookupFn means "no overrides available" — every override-point
// renders its inline seed default. This allows printPlaybook and unit tests to
// pass nil without constructing a resolver.
type overrideLookupFn func(pointId, harness string) (value string, found bool)

const (
	// overrideOpenPrefix and overrideClosePrefix are the trimmed-line prefixes used
	// to detect override-point open and close markers. The open marker has the form:
	//   <!-- ws:override:<pointId> desc="..." -->
	// The close marker has the form:
	//   <!-- ws:/override:<pointId> -->
	overrideOpenPrefix  = "<!-- ws:override:"
	overrideClosePrefix = "<!-- ws:/override:"
)

// parseOverrideMarkerPointId extracts the pointId token from a trimmed marker
// line whose prefix has already been verified, returning (pointId, true) when the
// line is a well-formed marker (ends with `-->`). Parsing is symmetric for open
// and close markers and tolerant of spacing: both `<!-- ws:override:Foo -->` and
// `<!-- ws:override:Foo-->` (no space before `-->`) yield pointId "Foo". An
// empty pointId or a missing `-->` terminator returns ("", false).
func parseOverrideMarkerPointId(trimmed, prefix string) (string, bool) {
	if !strings.HasPrefix(trimmed, prefix) || !strings.HasSuffix(trimmed, "-->") {
		return "", false
	}
	// Strip the prefix and the trailing `-->`, then take the first whitespace-
	// delimited token as the pointId. This handles both `Foo -->`, `Foo-->`, and
	// `Foo desc="..." -->` forms identically.
	rest := strings.TrimSuffix(trimmed[len(prefix):], "-->")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", false
	}
	pointId := rest
	if spaceIdx := strings.IndexAny(rest, " \t"); spaceIdx >= 0 {
		pointId = rest[:spaceIdx]
	}
	pointId = strings.TrimSpace(pointId)
	if pointId == "" {
		return "", false
	}
	return pointId, true
}

// parseOverrideOpenMarkerDesc extracts the pointId and optional desc from a
// trimmed open-marker line of the form `<!-- ws:override:<pointId> desc="..." -->`.
// It mirrors parseOverrideMarkerPointId for the pointId token but additionally
// surfaces the `desc="..."` attribute (the render engine discards desc, so this is
// a separate parser dedicated to the config.prompt listing).
//
// Returns (pointId, desc, true) for a well-formed open marker; desc is "" when the
// attribute is absent. A non-marker line or empty pointId returns ("", "", false).
func parseOverrideOpenMarkerDesc(trimmed string) (pointId, desc string, ok bool) {
	if !strings.HasPrefix(trimmed, overrideOpenPrefix) || !strings.HasSuffix(trimmed, "-->") {
		return "", "", false
	}
	rest := strings.TrimSuffix(trimmed[len(overrideOpenPrefix):], "-->")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", false
	}
	pointId = rest
	if spaceIdx := strings.IndexAny(rest, " \t"); spaceIdx >= 0 {
		pointId = rest[:spaceIdx]
	}
	pointId = strings.TrimSpace(pointId)
	if pointId == "" {
		return "", "", false
	}
	// Extract desc="..." from the remainder, if present. Find the opening
	// `desc="` then the next `"` to delimit the value.
	if start := strings.Index(rest, `desc="`); start >= 0 {
		valueStart := start + len(`desc="`)
		if end := strings.IndexByte(rest[valueStart:], '"'); end >= 0 {
			desc = rest[valueStart : valueStart+end]
		}
	}
	return pointId, desc, true
}

// overridePointDecl is a single declared override-point discovered in the rsrc
// playbook tree: the pointId and its short desc (empty when undeclared).
type overridePointDecl struct {
	PointId string
	Desc    string
}

const (
	workflowManualPlaybookName = "lead-workflow-manual"
	preferSubagentEnabledValue = "on"
	// mailboxWaitPlaybookName is the only playbook whose body substitutes
	// {{.MailboxWaitCommand}}, so the render/read dispatch resolves that
	// variable for this stem alone (mirroring the workflowManualPlaybookName
	// special-case): a broader inject would run mailboxOwnerCheck I/O for
	// every keyed read and, on the render path, route the reserved var through
	// the wsflow free-text bridge for render-eligible stems.
	mailboxWaitPlaybookName = "lead-use-mailbox"
	// delegateImplementerPlaybookName is the only stem whose render reserves a
	// write-once plan file, and only for a top-level lead caller: a worker's
	// leaves are governed by its ticket, so a plan per leaf would be noise.
	delegateImplementerPlaybookName = "delegate-implementer"
	// planStemContextKey carries the plan stem in the render context. The
	// render handler consumes it before variable validation, so it is never a
	// declared template variable and a render that gets no plan never fails
	// over it.
	planStemContextKey = "plan_stem"
	// defaultPlanStem names the plan when the caller passed no stem; the
	// timestamped path is already unique, so a missing stem costs no retry.
	defaultPlanStem = "delegate"
	// planPathLinePrefix starts both the plan-path output line (appended after
	// the prompt path, which callers read as the first line) and the line the
	// rendered floor ends with.
	planPathLinePrefix = "plan-path: "
)

// builtinPromptOverrideDefaults supplies defaults for configurable prompt points.
func builtinPromptOverrideDefaults() map[string]string {
	return map[string]string{}
}

// scanOverridePoints walks the rsrc tree rooted at rsrcRoot, scans every `.md`
// file for override open markers, and returns the declared override-points
// deduped by pointId (the first non-empty desc wins) sorted by PointId. It is a
// pure function (root in, data out) so it unit-tests without a Server.
func scanOverridePoints(rsrcRoot fs.FS) ([]overridePointDecl, error) {
	descByPoint := map[string]string{}
	err := fs.WalkDir(rsrcRoot, ".", func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := fs.ReadFile(rsrcRoot, path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			pointId, desc, ok := parseOverrideOpenMarkerDesc(strings.TrimSpace(line))
			if !ok {
				continue
			}
			if existing, seen := descByPoint[pointId]; !seen || (existing == "" && desc != "") {
				descByPoint[pointId] = desc
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	points := make([]overridePointDecl, 0, len(descByPoint))
	for id, desc := range descByPoint {
		points = append(points, overridePointDecl{PointId: id, Desc: desc})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].PointId < points[j].PointId })
	return points, nil
}

// buildOverrideLookup returns an overrideLookupFn backed by the session-anchored
// layered-config resolver (session, project, committed repo, global, builtin).
// When sessionKey is empty, only code-owned builtin prompt defaults
// participate; user/project/repo/global prompt overrides still require a
// session-keyed render. It is the single construction site shared by the
// playbook.read, playbook.render, and workflow_manual paths.
//
// Override values are stored under dynamic keys `prompt.<pointId>.<harness>`; the
// resolver returns empty (not an error) for unset keys, so an absent override
// yields ("", false) and the override pass falls back to the inline seed. The
// closure cannot carry an error, so a config file that fails to load (a
// malformed committed repo file) is reported here, before any render.
func buildOverrideLookup(s *Server, sessionKey string) (overrideLookupFn, error) {
	capturedKey := strings.TrimSpace(sessionKey)
	// The final bucket alone supplies knob fallbacks: an earlier answer would
	// hide a stored prompt override in a later bucket. Resolve lazily so a
	// keyless render without these points gains no config dependency.
	knobFallback := func(pointId, harness string) (string, bool) {
		if harness != "all" {
			return "", false
		}
		switch pointId {
		case "TicketHandlerPersistConfirmation", "TicketHandlerReviewAutoAck":
			enabled, err := s.ticketReviewAutoAckEnabled(capturedKey)
			if err != nil || enabled {
				return "", false
			}
			if pointId == "TicketHandlerPersistConfirmation" {
				return "- The lead's replies are the confirmation record. Persist only what they\n  confirm.", true
			}
			return "- No auto-confirm: every announced or technical item,\n" +
				"  including one from a populator `decision_gaps:` item or a reviewer issue,\n" +
				"  gets a lead round.", true
		case "TicketQueueWhiteboard":
			resolver := s.sessionResolver(capturedKey, builtinConfigDefaults())
			enabled, err := s.whiteboardEnabled(&resolver, capturedKey)
			if err == nil && enabled {
				return s.namespaced(lineTicketQueueBoard), true
			}
		}
		return "", false
	}
	if capturedKey == "" {
		builtins := builtinPromptOverrideDefaults()
		return func(pointId, harness string) (string, bool) {
			if v := builtins[harnessBucketKey("prompt."+pointId, harness)]; v != "" {
				return v, true
			}
			return knobFallback(pointId, harness)
		}, nil
	}
	resolver := s.sessionResolver(capturedKey, builtinPromptOverrideDefaults())
	if err := resolver.Check(); err != nil {
		return nil, err
	}
	preferences, haveSlots, err := preferenceOverride(&resolver, capturedKey, s.playbookHarness())
	if err != nil {
		return nil, err
	}
	return func(pointId, harness string) (string, bool) {
		if pointId == "UserPreferenceSection" && haveSlots {
			return preferences, true
		}
		rv, _ := resolver.Get(capturedKey, harnessBucketKey("prompt."+pointId, harness))
		if rv.Value != "" {
			return rv.Value, true
		}
		return knobFallback(pointId, harness)
	}, nil
}

// applyOverrideMarkers processes override-point block markers in body, resolving
// each (pointId, harness) pair through the lookup closure and substituting the
// inline seed default when no override is stored. Marker lines are always stripped
// from the rendered output — the result contains only resolved content and never
// any override marker syntax.
//
// Resolution order per point: lookup(pointId, bucket) for each bucket in
// harnessBucketReadOrder(harness) (the harness, then all) → inline seed body. An empty seed body (open marker immediately followed by close
// marker) is an extension slot: it renders the stored override or nothing.
//
// Robustness guarantees:
//   - Open/close parsing is symmetric and tolerant of spacing before `-->`.
//   - Nested override blocks inside a seed body are processed recursively, so no
//     inner marker line ever survives in the output.
//   - An unclosed open marker (EOF reached with no matching close) is NOT treated
//     as an override block: its line is emitted unchanged so no playbook content
//     is consumed or truncated.
//
// A nil lookup is treated as "no overrides": every point falls back to its seed.
func applyOverrideMarkers(body, harness string, lookup overrideLookupFn) string {
	lines := strings.Split(body, "\n")
	result := make([]string, 0, len(lines))

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Detect open marker: <!-- ws:override:<pointId> ... -->
		pointId, ok := parseOverrideMarkerPointId(trimmed, overrideOpenPrefix)
		if !ok {
			result = append(result, line)
			i++
			continue
		}

		// Scan forward for the matching close marker, tracking nesting depth so an
		// inner same-or-other override block does not prematurely close the outer
		// one. The close that matches this open is the first close (for any pointId)
		// encountered at depth 0.
		seedLines := make([]string, 0)
		depth := 1
		closeIdx := -1
		for j := i + 1; j < len(lines); j++ {
			jt := strings.TrimSpace(lines[j])
			if _, openOK := parseOverrideMarkerPointId(jt, overrideOpenPrefix); openOK {
				depth++
				seedLines = append(seedLines, lines[j])
				continue
			}
			if _, closeOK := parseOverrideMarkerPointId(jt, overrideClosePrefix); closeOK {
				depth--
				if depth == 0 {
					closeIdx = j
					break
				}
				seedLines = append(seedLines, lines[j])
				continue
			}
			seedLines = append(seedLines, lines[j])
		}

		// Unclosed open marker: do NOT treat as a block. Emit the open line
		// unchanged and continue scanning from the next line so no content is lost.
		if closeIdx < 0 {
			result = append(result, line)
			i++
			continue
		}

		// Recursively process the seed so any nested override block inside it is
		// resolved and its marker lines are stripped (prevents orphaned markers).
		seed := applyOverrideMarkers(strings.Join(seedLines, "\n"), harness, lookup)

		// Resolve: harness-specific → all → seed.
		resolved := seed
		if lookup != nil {
			for _, bucket := range harnessBucketReadOrder(harness) {
				if v, ok := lookup(pointId, bucket); ok {
					resolved = v
					break
				}
			}
		}

		// Append the resolved text (may be empty for empty-seed extension slots
		// with no stored override).
		if resolved != "" {
			result = append(result, resolved)
		}
		// Advance past the close marker.
		i = closeIdx + 1
	}

	return strings.Join(result, "\n")
}
