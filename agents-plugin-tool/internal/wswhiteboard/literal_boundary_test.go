package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// Every round-3 variant, including cross-group/root and rejected-group skips.
func literalBoundaryRepros() map[string]string {
	topic := func(s string) string { return `<wb:topic name="t">` + s + `</wb:topic>` }
	panel := func(s string) string { return `<wb:panel recap="r">` + s + `</wb:panel>` }
	first := panel("see `<b title='")
	second := `<wb:panel recap="s">it's > two ` + "`" + ` end</wb:panel>`
	out := map[string]string{
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
	return out
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
			p := &problems{}
			got := wsxmlcmd.Parse(p, grammar, input)
			q := &problems{}
			ordered, complete := wsxmlcmd.ParseOrdered(q, grammar, input)
			if !complete {
				t.Fatalf("incomplete: %v", q.List)
			}
			if strings.HasPrefix(name, "CB-") {
				if len(p.List) != 0 || len(q.List) != 0 {
					t.Fatalf("problems: %v / %v", p.List, q.List)
				}
				count := 2
				if strings.HasPrefix(name, "CB-log") {
					count = 3
				}
				if name == "CB-field" {
					count = 1
				}
				if len(got) != count {
					t.Fatalf("commands=%d want %d: %+v", len(got), count, got)
				}
				if name == "CB-field" {
					if got[0].FieldsOf("wb:title")[0] != "a `b" || got[0].FieldsOf("wb:why")[0] != "see ``</wb:why>`` y ` z" {
						t.Fatalf("fields: %+v", got)
					}
				} else {
					first := "a `"
					if name == "CB-log-baseline" {
						first = "a"
					}
					if got[0].Body != first {
						t.Fatalf("first body=%q want=%q", got[0].Body, first)
					}
				}
			} else {
				oldGrammar := grammar
				oldGrammar.LiteralCode = false
				baselineProblems := &problems{}
				baseline := wsxmlcmd.Parse(baselineProblems, oldGrammar, input)
				if !reflect.DeepEqual(canonicalCommands(got), canonicalCommands(baseline)) {
					t.Fatalf("baseline=%+v got=%+v", baseline, got)
				}
				if name != "CA-rejected-group" && (len(p.List) != 0 || len(q.List) != 0) {
					t.Fatalf("problems: %v / %v", p.List, q.List)
				}
			}
			if name == "CA-rejected-group" {
				if len(ordered) != 3 || ordered[2].Body != "it's > keep ` it" {
					t.Fatalf("lost valid group: %+v", ordered)
				}
			} else if !reflect.DeepEqual(canonicalCommands(got), canonicalCommands(ordered)) {
				t.Fatalf("ParseOrdered differs: %+v", ordered)
			}
		})
	}
}

func TestLiteralWalkResumesDelimitersAfterCDATA(t *testing.T) {
	// The first following-body span ends inside CDATA. After consuming the
	// section atomically, its immediately following span quotes its own close.
	body := "`<![CDATA[`]]>``</wb:panel>`` y ``` z"
	input := "<wb:topic name=\"t\"><wb:panel recap=\"r\">```a</wb:panel><wb:panel recap=\"s\">" + body + "</wb:panel></wb:topic>"
	want := "````</wb:panel>`` y ``` z"
	for _, ordered := range []bool{false, true} {
		p := &problems{}
		var cmds []command
		complete := true
		if ordered {
			cmds, complete = wsxmlcmd.ParseOrdered(p, grammar, input)
			cmds = cmds[1:]
		} else {
			cmds = wsxmlcmd.Parse(p, grammar, input)
		}
		if len(p.List) != 0 || !complete || len(cmds) != 2 || cmds[0].Body != "```a" || cmds[1].Body != want {
			t.Fatalf("ordered=%v commands=%+v complete=%v problems=%v", ordered, cmds, complete, p.List)
		}
	}
	st := &State{}
	out, p := Apply(st, input, testEnv())
	if out.Accepted != 2 || st.PanelCounter != 2 || len(p) != 0 || out.Failed != "" || out.Unpartitioned || queryResult(t, st, `{"panel":"p2"}`).Direct.Entry.Text != want {
		t.Fatalf("Apply=%+v panels=%d problems=%v", out, st.PanelCounter, p)
	}
}

func TestLiteralBoundaryRound4ApplyNoSilentMerge(t *testing.T) {
	for _, name := range []string{"CA-panel", "CA-third-panel", "CA-double-quote", "CA-known-name", "CA-fence", "CA-cdata-end", "CA-cdata-baseline", "CB-log", "CB-log-baseline", "CB-panel"} {
		t.Run(name, func(t *testing.T) {
			input := literalBoundaryRepros()[name]
			cmds, parseProblems := parse(input)
			st := &State{}
			result, p := Apply(st, input, testEnv())
			count := 2
			if name == "CA-third-panel" || strings.HasPrefix(name, "CB-log") {
				count = 3
			}
			panels := count
			if strings.HasPrefix(name, "CB-log") {
				panels = 2
			}
			if len(parseProblems) != 0 || len(cmds) != count || result.Accepted != count || st.PanelCounter != panels || len(p) != 0 || result.Failed != "" || result.Unpartitioned {
				t.Fatalf("commands=%d result=%+v panels=%d problems=%v / %v", len(cmds), result, st.PanelCounter, parseProblems, p)
			}
			for i, c := range cmds {
				if c.Tag == "wb:panel" {
					id := fmt.Sprintf("p%d", i+1)
					if strings.HasPrefix(name, "CB-log") && i == 2 {
						id = "p2"
					}
					if body := queryResult(t, st, fmt.Sprintf(`{"panel":%q}`, id)).Direct.Entry.Text; body != c.Body {
						t.Fatalf("%s body=%q want %q", id, body, c.Body)
					}
				} else if log, _ := st.LatestAgent("t"); log.Text != c.Body {
					t.Fatalf("log=%q want %q", log.Text, c.Body)
				}
			}
			if name == "CB-log" && cmds[0].Body != "a `" {
				t.Fatal("changed first body")
			}
		})
	}
	input := literalBoundaryRepros()["CA-rejected-group"]
	st := &State{}
	result, p := Apply(st, input, testEnv())
	failed := strings.Split(input, `<wb:topic name="ok">`)[0]
	if result.Accepted != 1 || st.PanelCounter != 0 || result.Unpartitioned || len(p) != 1 || result.Failed != "<wb:failed>\n"+failed+"\n</wb:failed>" {
		t.Fatalf("rejected group: %+v %v", result, p)
	}
	if log, _ := st.LatestAgent("ok"); log.Text != "it's > keep ` it" {
		t.Fatalf("lost log: %+v", log)
	}
}
