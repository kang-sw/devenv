package wswhiteboard

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// These tests are serial (the package has no t.Parallel calls). The disabled
// grammar is the structural reference, not a literal-mode implementation copy.
func assertLiteralOwnerBReading(t *testing.T, input string) {
	t.Helper()
	g0 := grammar
	g0.LiteralCode = false
	for _, ordered := range []bool{false, true} {
		p, q := &problems{}, &problems{}
		var baseline, got []command
		wantComplete, complete := true, true
		if ordered {
			baseline, wantComplete = wsxmlcmd.ParseOrdered(p, g0, input)
			got, complete = wsxmlcmd.ParseOrdered(q, grammar, input)
		} else {
			baseline = wsxmlcmd.Parse(p, g0, input)
			got = wsxmlcmd.Parse(q, grammar, input)
		}
		if complete != wantComplete || len(p.List) != len(q.List) || !reflect.DeepEqual(canonicalCommands(baseline), canonicalCommands(got)) {
			t.Fatalf("owner-b structural mismatch: ordered=%v complete=%v/%v problems=%v/%v baseline=%+v got=%+v", ordered, wantComplete, complete, p.List, q.List, baseline, got)
		}
	}
}

func assertLiteralOwnerBApply(t *testing.T, input string) {
	t.Helper()
	baseline := &State{}
	original := grammar.LiteralCode
	var want Outcome
	var p []string
	func() {
		grammar.LiteralCode = false
		defer func() { grammar.LiteralCode = original }()
		want, p = Apply(baseline, input, testEnv())
	}()
	gotState := &State{}
	got, q := Apply(gotState, input, testEnv())
	if len(p) != len(q) || !reflect.DeepEqual(want, got) || !reflect.DeepEqual(baseline, gotState) {
		t.Fatalf("owner-b Apply mismatch: expected=%+v actual=%+v problems=%v/%v", want, got, p, q)
	}
}

func TestLiteralOwnerBStructuralReadingAndGuidance(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		guidance    bool
	}{
		{"clean-legacy-split", "<wb:topic name=\"t\"><wb:panel recap=\"a\">a `</wb:panel><wb:panel recap=\"b\">b `</wb:panel></wb:topic>", false},
		{"own-close-example", "<wb:topic name=\"t\"><wb:panel recap=\"r\">`</wb:panel>` tail</wb:panel></wb:topic>", true},
		{"fatal-structure", "<wb:topic name=\"t\"><wb:panel recap=\"r\">`</wb:panel>`<wb:panel recap='", true},
		{"unrelated-later-problem", "<wb:topic name=\"t\"><wb:panel recap=\"a\">a `</wb:panel><wb:panel recap=\"b\">b `</wb:panel></wb:topic><wb:unknown/>", false},
		{"CDATA-own-close", "<wb:topic name=\"t\"><wb:panel recap=\"r\">`<![CDATA[</wb:panel>]]>`</wb:panel></wb:topic>", false},
		{"raw-own-close", "<wb:topic name=\"t\"><wb:panel recap=\"r\">`<wb:raw></wb:panel></wb:raw>`</wb:panel></wb:topic>", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g0 := grammar
			g0.LiteralCode = false
			for _, ordered := range []bool{false, true} {
				p, q := &problems{}, &problems{}
				var baseline, got []command
				wantComplete, complete := true, true
				if ordered {
					baseline, wantComplete = wsxmlcmd.ParseOrdered(p, g0, tc.input)
					got, complete = wsxmlcmd.ParseOrdered(q, grammar, tc.input)
				} else {
					baseline = wsxmlcmd.Parse(p, g0, tc.input)
					got = wsxmlcmd.Parse(q, grammar, tc.input)
				}
				if complete != wantComplete || len(p.List) != len(q.List) || !reflect.DeepEqual(canonicalCommands(baseline), canonicalCommands(got)) {
					t.Fatalf("ordered=%v structural=%+v literal=%+v problems=%v/%v complete=%v/%v", ordered, baseline, got, p.List, q.List, wantComplete, complete)
				}
				text := strings.Join(q.List, "\n")
				if strings.Contains(text, "<wb:raw>...</wb:raw>") != tc.guidance || strings.Contains(text, "CDATA") {
					t.Fatalf("guidance=%v problems=%v", tc.guidance, q.List)
				}
			}
		})
	}
}

func TestLiteralOwnerBCDATASplitAndLocalWrapperStayInert(t *testing.T) {
	input := "<wb:topic name=\"t\"><wb:panel name=\"p\" recap=\"r\">`<wb:change/>` <![CDATA[A]]]]><![CDATA[>B </wb:panel>]]></wb:panel></wb:topic>"
	for _, ordered := range []bool{false, true} {
		p := &problems{}
		var got []command
		complete := true
		if ordered {
			got, complete = wsxmlcmd.ParseOrdered(p, grammar, input)
			got = got[1:]
		} else {
			got = wsxmlcmd.Parse(p, grammar, input)
		}
		if !complete || len(p.List) != 0 || len(got) != 1 || len(got[0].Children) != 0 || got[0].Body != "`<wb:change/>` A]]>B </wb:panel>" {
			t.Fatalf("ordered=%v commands=%+v complete=%v problems=%v", ordered, got, complete, p.List)
		}
	}
}
