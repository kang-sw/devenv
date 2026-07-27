package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdoc"
)

// legacyMarkerAdvisoryPrefix mirrors wsdoc's note prefix, including the marker's
// 1-based line number, so a render-side change to the note is caught here too.
const legacyMarkerAdvisoryPrefix = "legacy planned marker (contract-first planned-entry mechanism being retired by " +
	"260726-refactor-retire-spec-planned-marker-mechanism): 1 marker(s) at line 10"

func legacyMarkerRenderRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Demo\n---\n# Demo\n\n"+
		"## Sibling Behavior {#260101-sibling}\n\nDeterministic workspace root pruning is already implemented.\n\n"+
		"> [!note] Planned 🚧 {#260101-anchor}\n> The registry will prune stale roots.\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260101-feat-unrelated.md",
		"---\ntitle: Unrelated\nspec:\n  - 260101-sibling\n---\n# Unrelated\n")
	return root
}

// formatSpecFind delegates wholly to formatDocumentFind, which knows nothing of
// SpecInfo, so the query path needs its own advisory append. This exercises the
// query path end-to-end rather than the no-query fallback that formatSpecs
// serves.
func TestFormatSpecSurfacesRenderLegacyMarkerAdvisory(t *testing.T) {
	root := legacyMarkerRenderRoot(t)

	list, err := wsdoc.SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	listText := formatSpecs(list)
	if !strings.Contains(listText, "  legacy-marker: "+legacyMarkerAdvisoryPrefix+"; no live ticket references this spec") {
		t.Fatalf("formatSpecs text = %q", listText)
	}

	found, err := wsdoc.SpecsFind(root, wsdoc.SpecFindOptions{Query: "deterministic workspace root pruning"})
	if err != nil {
		t.Fatalf("SpecsFind returned error: %v", err)
	}
	findText := formatSpecFind("deterministic workspace root pruning", found)
	if !strings.Contains(findText, "candidate spec for query=") {
		t.Fatalf("formatSpecFind lost its delegated body: %q", findText)
	}
	// A blank line separates the advisory from the last hit line: document
	// blocks are emitted with a leading "\n", so without it the note runs flush
	// against the preceding hit.
	if !strings.Contains(findText, "\n\nlegacy-marker: ai-docs/spec/demo.md: "+legacyMarkerAdvisoryPrefix+"; no live ticket references this spec") {
		t.Fatalf("formatSpecFind text = %q", findText)
	}

	status, err := wsdoc.SpecsStatus(root, wsdoc.SpecStatusOptions{SpecStem: "260101-anchor"})
	if err != nil {
		t.Fatalf("SpecsStatus returned error: %v", err)
	}
	statusText := formatSpecStatus(status)
	if !strings.Contains(statusText, "legacy-marker: ai-docs/spec/demo.md: "+legacyMarkerAdvisoryPrefix) {
		t.Fatalf("formatSpecStatus text = %q", statusText)
	}
}

// formatSpecFind's advisory loop is bounded by the same maxFindTextDocuments cut
// the delegated body applies, so the note can never name a spec that was
// truncated out of the listing above it.
func TestFormatSpecFindDropsAdvisoriesForTruncatedDocuments(t *testing.T) {
	specs := []wsdoc.SpecInfo{}
	for i := 0; i < maxFindTextDocuments; i++ {
		specs = append(specs, wsdoc.SpecInfo{Path: fmt.Sprintf("ai-docs/spec/rank%02d.md", i), MatchScore: 100 - i})
	}
	specs = append(specs, wsdoc.SpecInfo{
		Path:                 "ai-docs/spec/truncated.md",
		MatchScore:           1,
		LegacyMarkerAdvisory: "legacy planned marker: 1 marker(s) at line 3; no live ticket references this spec",
	})

	got := formatSpecFind("prune policy", specs)
	if strings.Contains(got, "ai-docs/spec/truncated.md") {
		t.Fatalf("formatSpecFind named a truncated document: %q", got)
	}
	if strings.Contains(got, "legacy-marker") {
		t.Fatalf("formatSpecFind emitted an advisory for a truncated document: %q", got)
	}
}

// normalizeJSONKey folds a wire key to the one form both a snake_case json tag
// and Go's tagless field-name fallback share. Separators are dropped rather
// than inserted, which keeps "marker_context" and "marker_contexts" distinct:
// they normalize to "markercontext" and "markercontexts", still compared for
// equality, so neither can stand in for the other.
func normalizeJSONKey(key string) string {
	return strings.ToLower(strings.ReplaceAll(key, "_", ""))
}

// jsonObjectKeys collects every object key anywhere in a decoded JSON value, at
// any nesting depth. Keys are compared for equality rather than scanned as
// substrings: the two retired fields are "marker_context" and
// "marker_contexts", and a substring test for the former also matches the
// latter, so one would silently stand in for the other.
//
// Keys are normalized by normalizeJSONKey so the guard also covers deletion of
// the `json:"-"` tag with no replacement, not only its reversion to the
// historical snake_case tag. Go falls back to the Go field name when no tag is
// present, so a bare deletion puts the retired data back on the wire as
// "MarkerContexts"/"MarkerContext" - a shape a case-exact comparison misses
// entirely. Lowercasing alone is not enough: "MarkerContexts" lowercases to
// "markercontexts", which is not "marker_contexts". Measured, not assumed.
func jsonObjectKeys(value any, out map[string]bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			out[normalizeJSONKey(key)] = true
			jsonObjectKeys(child, out)
		}
	case []any:
		for _, child := range typed {
			jsonObjectKeys(child, out)
		}
	}
}

// marshalledKeys serializes a tool result exactly as toolJSONResponse does and
// returns every key the wire payload carries.
func marshalledKeys(t *testing.T, value any) map[string]bool {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	keys := map[string]bool{}
	jsonObjectKeys(decoded, keys)
	return keys
}

// Phase 2 of 260726-refactor-retire-spec-planned-marker-mechanism retired three
// outputs: formatSpecs' "  marker: " line, formatSpecStatus' trailing
// " # <marker context>" suffix on a location, and the JSON serialization of
// SpecInfo.MarkerContexts / SpecAnchorInfo.MarkerContext, silenced by `json:"-"`.
// Both feeding values are still populated on every call, because SpecsFind match
// scoring reads them, so each retired output is exactly one restored statement or
// tag away. Each negative assertion is paired with a check that its feeding value
// is non-empty, so the guard cannot pass vacuously if population ever stops.
func TestFormatSpecSurfacesOmitRetiredMarkerRender(t *testing.T) {
	root := legacyMarkerRenderRoot(t)

	list, err := wsdoc.SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	fedList := false
	for _, spec := range list {
		if len(spec.MarkerContexts) > 0 {
			fedList = true
		}
	}
	if !fedList {
		t.Fatal("fixture no longer populates SpecInfo.MarkerContexts; the marker-line assertion would pass vacuously")
	}
	// "legacy-marker: " is Phase 1's retained advisory and must survive; only a
	// bare "marker: " label is the retired render, at any indentation.
	for _, line := range strings.Split(formatSpecs(list), "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "marker: ") {
			t.Fatalf("formatSpecs re-emitted the retired marker line: %q", line)
		}
	}
	// specs.list and specs.find at format=json hand this value straight to
	// toolJSONResponse, so the struct tags are the wire contract. They are the
	// whole contract for every JSON path, not only the specs.* handlers:
	// references.trace embeds []SpecInfo as well, and SpecAnchorStatus nests both
	// leaf structs, so only the leaf tags keep those payloads silent too.
	listKeys := marshalledKeys(t, list)
	if listKeys["markercontexts"] {
		t.Fatal(`serialized specs.list result carries the retired "marker_contexts" key`)
	}
	if listKeys["markercontext"] {
		t.Fatal(`serialized specs.list result carries the retired "marker_context" key under anchors[]`)
	}

	status, err := wsdoc.SpecsStatus(root, wsdoc.SpecStatusOptions{SpecStem: "260101-anchor"})
	if err != nil {
		t.Fatalf("SpecsStatus returned error: %v", err)
	}
	fedStatus := false
	for _, loc := range status.Locations {
		if loc.MarkerContext != "" {
			fedStatus = true
		}
	}
	if !fedStatus {
		t.Fatal("fixture no longer populates SpecAnchorInfo.MarkerContext; the suffix assertion would pass vacuously")
	}
	// Scoped to location lines so an anchor or a "#"-bearing path elsewhere in
	// the render cannot make this brittle.
	for _, line := range strings.Split(formatSpecStatus(status), "\n") {
		if !strings.HasPrefix(line, "  - line ") {
			continue
		}
		if strings.Contains(line, " # ") {
			t.Fatalf("formatSpecStatus re-emitted the retired marker-context suffix: %q", line)
		}
	}
	statusKeys := marshalledKeys(t, status)
	if statusKeys["markercontext"] {
		t.Fatal(`serialized specs.status result carries the retired "marker_context" key under locations[]`)
	}
	if statusKeys["markercontexts"] {
		t.Fatal(`serialized specs.status result carries the retired "marker_contexts" key under files[]`)
	}
}

func TestFormatSpecSurfacesOmitAdvisoryWhenNoMarker(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/clean.md", "---\ntitle: Clean\n---\n# Clean\n\n## Clean {#260101-clean}\n\nDeterministic workspace root pruning is implemented.\n")

	list, err := wsdoc.SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	if got := formatSpecs(list); strings.Contains(got, "legacy-marker") {
		t.Fatalf("formatSpecs emitted advisory without a marker: %q", got)
	}

	found, err := wsdoc.SpecsFind(root, wsdoc.SpecFindOptions{Query: "deterministic workspace root pruning"})
	if err != nil {
		t.Fatalf("SpecsFind returned error: %v", err)
	}
	if got := formatSpecFind("deterministic workspace root pruning", found); strings.Contains(got, "legacy-marker") {
		t.Fatalf("formatSpecFind emitted advisory without a marker: %q", got)
	}

	status, err := wsdoc.SpecsStatus(root, wsdoc.SpecStatusOptions{SpecStem: "260101-clean"})
	if err != nil {
		t.Fatalf("SpecsStatus returned error: %v", err)
	}
	if got := formatSpecStatus(status); strings.Contains(got, "legacy-marker") {
		t.Fatalf("formatSpecStatus emitted advisory without a marker: %q", got)
	}
}
