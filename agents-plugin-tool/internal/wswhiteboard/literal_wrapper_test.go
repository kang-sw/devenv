package wswhiteboard

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

func literalWrapperContextInputs() []string {
	return []string{
		"<wb:topic name=\"t\"><wb:panel name=\"p\"><wb:change><wb:old>a ` b</wb:old><wb:new>c</wb:new></wb:change><wb:change><wb:old>x </wb:change> y ` z</wb:old><wb:new>w</wb:new></wb:change></wb:panel></wb:topic>",
		"<wb:topic name=\"0\"><wb:panel name=\"\"><wb:change><wb:old>a ` b</wb:old><wb:new>c</wb:new></wb:change> d ` e</wb:old><wb:new>f</wb:new></wb:change></wb:panel></wb:topic>",
	}
}

func literalPanelOriginInputs() []string {
	return []string{
		"<wb:topic name=\"t\"><wb:panel name=\"p\" recap=\"r\">`<wb:change><wb:old>x</wb:panel><wb:foo>y</wb:old><wb:new>z</wb:new></wb:change></wb:panel><wb:log>q </wb:foo>` </wb:panel><wb:log>r</wb:log></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel name=\"p\">`<wb:change><wb:old>x</wb:panel>y`</wb:old><wb:new>z</wb:new></wb:change></wb:panel></wb:topic>",
	}
}

func TestLiteralPanelOriginCloseInsideOperand(t *testing.T) {
	g0 := grammar
	g0.LiteralCode = false
	for _, input := range literalPanelOriginInputs() {
		p := &problems{}
		baseline := wsxmlcmd.Parse(p, g0, input)
		if len(p.List) != 0 {
			t.Fatal(p.List)
		}
		for _, ordered := range []bool{false, true} {
			p := &problems{}
			var got []command
			complete := true
			if ordered {
				got, complete = wsxmlcmd.ParseOrdered(p, grammar, input)
			} else {
				got = wsxmlcmd.Parse(p, grammar, input)
			}
			if !complete || len(p.List) != 0 || !reflect.DeepEqual(canonicalCommands(got), canonicalCommands(baseline)) {
				t.Fatalf("ordered=%v commands=%+v problems=%v", ordered, canonicalCommands(got), p.List)
			}
		}
		st := &State{}
		out, pApply := Apply(st, input, testEnv())
		if len(pApply) == 0 || out.Failed == "" || out.Unpartitioned || st.PanelCounter != 0 {
			t.Fatalf("mixed panel silently accepted: %+v panels=%d problems=%v", out, st.PanelCounter, pApply)
		}
	}
}

func TestLiteralWrapperContextPreservesStructuralReading(t *testing.T) {
	g0 := grammar
	g0.LiteralCode = false
	for _, input := range literalWrapperContextInputs() {
		p := &problems{}
		baseline := wsxmlcmd.Parse(p, g0, input)
		if len(p.List) != 0 {
			t.Fatal(p.List)
		}
		for _, ordered := range []bool{false, true} {
			p := &problems{}
			var got []command
			complete := true
			if ordered {
				got, complete = wsxmlcmd.ParseOrdered(p, grammar, input)
			} else {
				got = wsxmlcmd.Parse(p, grammar, input)
			}
			if !complete || len(p.List) != 0 || !reflect.DeepEqual(canonicalCommands(got), canonicalCommands(baseline)) {
				t.Fatalf("ordered=%v complete=%v problems=%v\nbaseline=%+v\ngot=%+v", ordered, complete, p.List, canonicalCommands(baseline), canonicalCommands(got))
			}
		}
	}
}

func TestLiteralWrapperContextApplyNoSilentMerge(t *testing.T) {
	const seedBody = "S[a ` b</wb:old><wb:new>c</wb:new></wb:change><wb:change><wb:old>x </wb:change> y ` z]"
	const want = "S[c</wb:old><wb:new>c</wb:new></wb:change><wb:change><wb:old>w]"
	st := &State{}
	seed := `<wb:topic name="t"><wb:panel name="p" recap="r"><![CDATA[` + seedBody + `]]></wb:panel></wb:topic>`
	if out, p := Apply(st, seed, testEnv()); len(p) != 0 || out.Accepted != 1 {
		t.Fatalf("seed: %+v %v", out, p)
	}
	out, p := Apply(st, literalWrapperContextInputs()[0], testEnv())
	if len(p) != 0 || out.Accepted != 1 || out.Unpartitioned || out.Failed != "" || st.PanelCounter != 1 {
		t.Fatalf("patch: %+v %v", out, p)
	}
	if got := queryResult(t, st, `{"panel":"p1"}`).Direct.Entry.Text; got != want {
		t.Fatalf("stored=%q want=%q", got, want)
	}
	if got := queryResult(t, st, `{"panel":"e1"}`).Direct.Entry.Text; got != seedBody {
		t.Fatalf("original immutable entry=%q", got)
	}
	// Panel-level raw text is Parse-clean but must remain visible to Apply's
	// mixed-payload refusal, not disappear into a preceding operand's code.
	mixed := strings.Replace(literalWrapperContextInputs()[1], `name="0"`, `name="t"`, 1)
	mixed = strings.Replace(mixed, `name=""`, `name="p"`, 1)
	out, p = Apply(st, mixed, testEnv())
	if len(p) == 0 || out.Accepted != 0 || out.Unpartitioned || out.Failed == "" {
		t.Fatalf("mixed payload was not refused: %+v %v", out, p)
	}
	if got := queryResult(t, st, `{"panel":"p1"}`).Direct.Entry.Text; got != want {
		t.Fatalf("refused mixed payload changed stored text: %q", got)
	}
}

func TestLiteralRejectedGroupFenceRetainsFollowingLog(t *testing.T) {
	for _, mark := range []string{"```", "~~~"} {
		bad := "<wb:topic name=\"a\">\n" + mark + "x\n</wb:topic>"
		fenceLog := "<wb:log>\n" + mark + "\n</wb:log>"
		input := strings.Repeat(bad, 4) + "<wb:topic name=\"z\">" + fenceLog + "<wb:log>keep</wb:log></wb:topic>"
		st := &State{}
		out, p := Apply(st, input, testEnv())
		// A fence-only log is correctly refused by inline-only validation.
		// Its delimiter must still stop earlier rejected-group walks, and
		// the subsequent valid log must be applied exactly once.
		if out.Accepted != 1 || out.Unpartitioned || len(p) != 5 || strings.Count(out.Failed, bad) != 4 || !strings.Contains(out.Failed, fenceLog) || strings.Contains(out.Failed, "keep") {
			t.Fatalf("mark=%q result=%+v problems=%v", mark, out, p)
		}
		if log, ok := st.LatestAgent("z"); !ok || log.Text != "keep" {
			t.Fatalf("following log=%+v ok=%v", log, ok)
		}
	}
}
