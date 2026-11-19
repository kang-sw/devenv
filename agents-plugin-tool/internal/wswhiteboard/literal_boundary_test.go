package wswhiteboard

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// Every prior variant remains a regression; owner (1208)=b now requires the
// structural interpretation rather than licensing any raw-close crossing.
func literalBoundaryRepros() map[string]string {
	topic := func(s string) string { return `<wb:topic name="t">` + s + `</wb:topic>` }
	panel := func(s string) string { return `<wb:panel recap="r">` + s + `</wb:panel>` }
	first := panel("see `<b title='")
	second := `<wb:panel recap="s">it's > two ` + "`" + ` end</wb:panel>`
	return map[string]string{
		"CA-cdata-end":      topic(panel("``") + panel("`<![CDATA[`</wb:panel>]]>``")),
		"CA-cdata-baseline": topic(panel("``") + panel("`<![CDATA[`</wb:panel>]]>")),
		"CA-panel":          topic(first + second),
		"CA-third-panel":    topic(first + second + panel("three")),
		"CA-double-quote":   topic(panel("see `<div class=\"") + `<wb:panel recap="s">say " > two ` + "`" + ` end</wb:panel>`),
		"CA-known-name":     topic(panel("see `<wb:log x='") + second),
		"CA-fence":          topic(panel("```\n<b title='\n") + `<wb:panel recap="s">it's > two` + "\n```\nend</wb:panel>"),
		"CA-panel-log":      topic(first + "<wb:log>it's > two ` end</wb:log>"),
		"CA-body-panel":     topic("<wb:body>x `<a href='</wb:body>" + second),
		"CA-why-link":       topic("<wb:add class=\"technical\"><wb:title>x</wb:title><wb:why>see `<b c='</wb:why><wb:link>it's > l ` m</wb:link></wb:add>"),
		"CA-old-new":        topic("<wb:edit><wb:old>a `<i x='</wb:old><wb:new>it's > b ` c</wb:new></wb:edit>"),
		"CA-new-topic":      topic(first) + `<wb:topic name="next"><wb:panel recap="s">it's > two ` + "`" + ` end</wb:panel></wb:topic>`,
		"CA-root-answer":    topic(first) + "<wb:answer id=\"1\" value=\"ok\">it's > two ` end</wb:answer>",
		"CA-rejected-group": "<wb:topic name=\"bad\">junk `<i a='</wb:topic><wb:topic name=\"ok\"><wb:log>it's > keep ` it</wb:log></wb:topic>",
		"CB-log":            topic(panel("a `") + "<wb:log>``</wb:log>`` y ` z</wb:log>" + panel("two")),
		"CB-log-baseline":   topic(panel("a") + "<wb:log>``</wb:log>`` y ` z</wb:log>" + panel("two")),
		"CB-panel":          topic(panel("a `") + panel("``</wb:panel>`` y ` z")),
		"CB-field":          topic("<wb:add class=\"technical\"><wb:title>a `b</wb:title><wb:why>see ``</wb:why>`` y ` z</wb:why></wb:add>"),
	}
}

type literalAmbiguityCase struct{ name, input string }

func literalAmbiguityTransitions() []literalAmbiguityCase {
	prefix := `<wb:topic name="t"><wb:panel recap="r">`
	next := `</wb:panel><wb:panel recap="s">`
	suffix := `</wb:panel></wb:topic>`
	return []literalAmbiguityCase{
		{"two-to-one-before", prefix + "``" + next + "`</wb:panel>`" + suffix},
		{"two-to-one-after", prefix + "``" + next + "`</wb:panel>``" + suffix},
		{"one-to-two-before", prefix + "```a" + next + "``b</wb:panel>```c" + suffix},
		{"one-to-two-after", prefix + "```a" + next + "``b</wb:panel>```c``" + suffix},
	}
}

func TestLiteralCodeActiveSpanMutationChangesCleanReading(t *testing.T) {
	// Retain all four historical inputs, but none can license a raw-close
	// crossing under owner-b. Existing structural problems and Apply policy win.
	for _, tc := range literalAmbiguityTransitions() {
		t.Run(tc.name, func(t *testing.T) {
			assertLiteralOwnerBReading(t, tc.input)
			assertLiteralOwnerBApply(t, tc.input)
		})
	}
}

// Compare semantic parse output, not ordered-mode rejection/source metadata.
// Empty ordered group sentinels are not commands in Parse's public contract.
func canonicalCommands(cmds []command) []map[string]any {
	var out []map[string]any
	for _, c := range cmds {
		if c.IsGroup() && !c.Scope.HasExtra() {
			continue
		}
		raw, _ := json.Marshal(c)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		delete(m, "Source")
		delete(m, "Problems")
		// Literal is newer than the v0.50 parser the differential compares
		// against; it differs from Body only when an entity decoded.
		if fields, ok := m["Fields"].([]any); ok {
			for _, f := range fields {
				if field, ok := f.(map[string]any); ok {
					delete(field, "Literal")
				}
			}
		}
		if scope, ok := m["Scope"].(map[string]any); ok {
			delete(scope, "Source")
			delete(scope, "Problems")
		}
		m["Children"] = nil
		if len(c.Children) != 0 {
			m["Children"] = canonicalCommands(c.Children)
		}
		out = append(out, m)
	}
	return out
}

func TestLiteralBoundaryRound4Repros(t *testing.T) {
	for name, input := range literalBoundaryRepros() {
		t.Run(name, func(t *testing.T) {
			if name == "CA-rejected-group" {
				// Preserve the original regression, not a code-disabled
				// ordered reference that lacks the required probe limit.
				p := &problems{}
				got, complete := wsxmlcmd.ParseOrdered(p, grammar, input)
				if !complete || len(p.List) != 1 || len(got) != 3 || got[2].Body != "it's > keep ` it" {
					t.Fatalf("lost following valid group: %+v %v complete=%v", got, p.List, complete)
				}
				return
			}
			assertLiteralOwnerBReading(t, input)
		})
	}
}

func TestLiteralWalkResumesDelimitersAfterCDATA(t *testing.T) {
	// CDATA remains atomic; the later quote of a raw panel close is now
	// structural, so compare the complete result and Apply outcome to fallback.
	body := "`<![CDATA[`]]>``</wb:panel>`` y ``` z"
	input := "<wb:topic name=\"t\"><wb:panel recap=\"r\">```a</wb:panel><wb:panel recap=\"s\">" + body + "</wb:panel></wb:topic>"
	assertLiteralOwnerBReading(t, input)
	assertLiteralOwnerBApply(t, input)
}

func TestLiteralBoundaryRound4ApplyNoSilentMerge(t *testing.T) {
	for name, input := range literalBoundaryRepros() {
		t.Run(name, func(t *testing.T) {
			if name == "CA-rejected-group" {
				st := &State{}
				out, p := Apply(st, input, testEnv())
				failed := strings.Split(input, `<wb:topic name="ok">`)[0]
				log, ok := st.LatestAgent("ok")
				if out.Accepted != 1 || out.Unpartitioned || len(p) != 1 || out.Failed != "<wb:failed>\n"+failed+"\n</wb:failed>" || !ok || log.Text != "it's > keep ` it" {
					t.Fatalf("lost accepted log or changed retry group: %+v %v", out, p)
				}
				return
			}
			assertLiteralOwnerBApply(t, input)
		})
	}
}
