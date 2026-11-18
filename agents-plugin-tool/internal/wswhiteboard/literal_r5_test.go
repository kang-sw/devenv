package wswhiteboard

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

func literalR5PanicInputs() []string {
	start := "<wb:topic name=\"t\"><wb:panel name=\"p\">`"
	return []string{
		start + "<wb:change a=\"<![CDATA[\"><wb:old>x</wb:old></wb:change></wb:panel>]]></wb:panel>`</wb:topic>",
		start + "<wb:change><wb:old a=\"<![CDATA[\">x</wb:old></wb:change></wb:panel>]]></wb:panel>`</wb:topic>",
		start + "<wb:change a=\"<![CDATA[\"><wb:old>x</wb:old></wb:change></wb:panel>]]></wb:panel><wb:log>q `</wb:log></wb:topic>",
	}
}

func TestLiteralR5CDATAAttributesRefuseWithoutPanic(t *testing.T) {
	for i, input := range literalR5PanicInputs() {
		for _, entry := range []string{"Parse", "ParseOrdered", "Apply"} {
			t.Run(fmt.Sprintf("%d/%s", i, entry), func(t *testing.T) {
				defer func() {
					if panicValue := recover(); panicValue != nil {
						t.Fatalf("CDATA-in-attribute panic: %v", panicValue)
					}
				}()
				p := &problems{}
				switch entry {
				case "Parse":
					wsxmlcmd.Parse(p, grammar, input)
				case "ParseOrdered":
					wsxmlcmd.ParseOrdered(p, grammar, input)
				default:
					_, p.List = Apply(&State{}, input, testEnv())
				}
				if len(p.List) == 0 {
					t.Fatal("malformed CDATA wrapper was silently accepted")
				}
			})
		}
	}
}

func literalR5Crossings() []string {
	return []string{
		"<wb:topic name=\"t\"><wb:panel name=\"p\" recap=\"r\">`<wb:change><wb:old>x</wb:panel>y</wb:old></wb:change>``<wb:change><wb:old>``z</wb:panel>w</wb:old></wb:change>`</wb:panel></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel name=\"p\" recap=\"r\">`<wb:change><wb:old>x</wb:panel>y</wb:old></wb:change>``<wb:change/>``</wb:panel><wb:log>`</wb:log></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel>``</wb:panel><wb:panel>`<wb:change><wb:old>`</wb:old><wb:new><wb:change>``</wb:new></wb:change></wb:panel></wb:topic>",
	}
}

func TestLiteralR5CrossingsRefuseOrPreserveStructure(t *testing.T) {
	for i, input := range literalR5Crossings() {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			if bad, eligible := literalStructuralSplitFailure(input); bad || !eligible {
				t.Fatalf("R5 crossing: bad=%v eligible=%v", bad, eligible)
			}
			st := &State{}
			out, p := Apply(st, input, testEnv())
			if len(p) == 0 || st.PanelCounter != 0 {
				t.Fatalf("mixed panel silently published: %+v panels=%d problems=%v", out, st.PanelCounter, p)
			}
		})
	}
}

func TestLiteralR5SpanOriginsFollowAcceptedQuotes(t *testing.T) {
	for _, body := range []string{
		"`>`<wb:change/>`",
		"`<wb:change><wb:old></wb:old></wb:change>`<wb:change><wb:old>`</wb:old></wb:change>",
	} {
		input := `<wb:topic name="t"><wb:panel name="p">` + body + `</wb:panel></wb:topic>`
		if bad, eligible := literalStructuralSplitFailure(input); bad || !eligible {
			t.Fatalf("m1 origin artifact: bad=%v eligible=%v input=%s", bad, eligible, input)
		}
	}
}

// Derived from the review's stated shape, not its unavailable generator source.
// Every input is structurally clean: 1-3 panels, 0-3 complete changes per panel,
// raw delimiter runs/fences, CDATA and raw elements, with parent closes/nested wrapper text in
// operands. Attribute-CDATA panic variants are separate exact fixtures, since
// their unknown attributes deliberately violate this generator's precondition.
func literalR5StructuredInput(rng *rand.Rand) string {
	raw := []string{"", "a", "`", "``", "```", "~~~xml\n", "```xml\n", "\n~~~\n", "\n```\n", "\n", " ", "<![CDATA[`</wb:panel>]]>", "<![CDATA[``]]>", "&#96;", "<wb:raw>`</wb:panel>]]></wb:raw>", "<wb:raw>``</wb:raw>"}
	operand := append(append([]string{}, raw...), "</wb:panel>", "<wb:change>", "</wb:change>", "<wb:foo>", "</wb:foo>", "<![CDATA[</wb:old></wb:new>]]>", "<wb:raw></wb:old><![CDATA[</wb:new></wb:raw>")
	body := func(tokens []string) string {
		var b strings.Builder
		for n := 1 + rng.Intn(4); n > 0; n-- {
			b.WriteString(tokens[rng.Intn(len(tokens))])
		}
		return b.String()
	}
	var b strings.Builder
	b.WriteString(`<wb:topic name="t">`)
	for n := 1 + rng.Intn(3); n > 0; n-- {
		b.WriteString(`<wb:panel name="p" recap="r">`)
		b.WriteString(body(raw))
		for m := rng.Intn(4); m > 0; m-- {
			b.WriteString(`<wb:change><wb:old>` + body(operand) + `</wb:old><wb:new>` + body(operand) + `</wb:new></wb:change>`)
			b.WriteString(body(raw))
		}
		b.WriteString(`</wb:panel>`)
	}
	b.WriteString(`</wb:topic>`)
	return b.String()
}

func TestLiteralWrapperStructuredFuzz(t *testing.T) {
	count := 300
	if value := os.Getenv("WS_LITERAL_WRAPPER_FUZZ_CASES"); value != "" {
		var err error
		count, err = strconv.Atoi(value)
		if err != nil || count < 1 {
			t.Fatalf("invalid WS_LITERAL_WRAPPER_FUZZ_CASES=%q", value)
		}
	}
	for _, seed := range []int64{424242, 777, 261116} {
		t.Run(strconv.FormatInt(seed, 10), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			bad, ineligible := 0, 0
			first := ""
			for i := 0; i < count; i++ {
				input := literalR5StructuredInput(rng)
				violates, eligible := literalStructuralSplitFailure(input)
				if !eligible {
					ineligible++
				}
				if violates {
					bad++
					if first == "" {
						first = input
					}
				}
			}
			t.Logf("seed=%d cases=%d bad=%d structurally-ineligible=%d", seed, count, bad, ineligible)
			if ineligible != 0 {
				t.Fatal("generator violated its structural-clean precondition")
			}
			if bad != 0 {
				minimal := minimizeLiteralInput(first, func(s string) bool { bad, eligible := literalStructuralSplitFailure(s); return bad && eligible })
				t.Fatalf("first minimized violation:\n%s", minimal)
			}
		})
	}
}

// D-E: a failed masked structural interpretation is not automatically a bug.
// Keep the original structural result as the comparator, or accept refusal.
func TestLiteralR5InvalidMaskedExpectationUsesStructuralFallback(t *testing.T) {
	for _, input := range literalR5Crossings() {
		p := &problems{}
		g0 := grammar
		g0.LiteralCode = false
		baseline := wsxmlcmd.Parse(p, g0, input)
		want, eligible := literalStructuralExpected(input)
		if len(p.List) != 0 || !eligible || want == nil {
			t.Fatalf("missing eligible comparator: %v", p.List)
		}
		// Cases without a local D-A span must remain entirely structural.
		q := &problems{}
		structural, _ := wsxmlcmd.ParseOrdered(q, g0, input)
		if len(literalWrapperMasks(input, structural)) == 0 && !reflect.DeepEqual(want, canonicalCommands(baseline)) {
			t.Fatal("unmasked structural result changed")
		}
	}
}
