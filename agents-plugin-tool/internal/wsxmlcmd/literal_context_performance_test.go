package wsxmlcmd

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

var literalContextShapes = []string{"quoted-wrapper-openers", "wrapper-old-tilde", "wrapper-new-tilde", "rejected-group-fence", "rejected-group-tilde", "rejected-group-fence-nolog", "panel-foo-desync", "rejected-group-html-fence", "rejected-group-html-inline"}

func literalContextGrammar() Grammar {
	g := literalPerformanceGrammar()
	g.Commands["panel"] = Spec{Kind: Leaf, Attrs: []string{"name", "recap"}, Wrapper: "change", Fields: []string{"old", "new"}, Scoped: true}
	return g
}

func literalContextInput(kind string, size int) (string, int) {
	if kind == "quoted-wrapper-openers" {
		body := strings.Repeat("`<wb:change>` ", max(1, size/13))
		return `<wb:topic name="t"><wb:panel name="p" recap="r">` + body + `</wb:panel></wb:topic>`, 1
	}
	if kind == "panel-foo-desync" {
		unit := "<wb:panel name=\"p\" recap=\"r\">`<wb:change><wb:old>x</wb:panel><wb:foo>y</wb:old><wb:new>z</wb:new></wb:change></wb:panel>"
		n := max(1, size/len(unit))
		return "<wb:topic name=\"t\">" + strings.Repeat(unit, n) + "<wb:log>q </wb:foo>` </wb:panel><wb:log>r</wb:log></wb:topic>", n + 1
	}
	if strings.HasPrefix(kind, "rejected-group-html-") {
		mark := "```x\n"
		last := "\n```\n</b>"
		if kind == "rejected-group-html-inline" {
			mark, last = "`x", "`</b>"
		}
		unit := "<wb:topic name=\"a\"><b>\n" + mark + "</wb:topic>"
		n := max(1, size/len(unit))
		return strings.Repeat(unit, n) + "<wb:topic name=\"z\"><wb:log>" + last + "</wb:log></wb:topic>", n
	}
	if strings.HasPrefix(kind, "wrapper-") {
		field := strings.TrimSuffix(strings.TrimPrefix(kind, "wrapper-"), "-tilde")
		change := func(body string) string {
			if field == "old" {
				return "<wb:change><wb:old>" + body + "</wb:old><wb:new>n</wb:new></wb:change>"
			}
			return "<wb:change><wb:old>o</wb:old><wb:new>" + body + "</wb:new></wb:change>"
		}
		unit := change("~~~x\n")
		n := max(1, size/len(unit))
		return `<wb:topic name="t"><wb:panel name="p">` + strings.Repeat(unit, n) + change("\n~~~\n") + `</wb:panel></wb:topic>`, n + 1
	}
	mark := "```"
	if kind == "rejected-group-tilde" {
		mark = "~~~"
	}
	unit := "<wb:topic name=\"a\">\n" + mark + "x\n</wb:topic>"
	n := max(1, size/len(unit))
	input := strings.Repeat(unit, n)
	if kind != "rejected-group-fence-nolog" {
		input += "<wb:topic name=\"z\"><wb:log>\n" + mark + "\n</wb:log></wb:topic>"
	}
	return input, n
}

func checkLiteralContext(t *testing.T, kind, input string, count int, ordered bool) {
	t.Helper()
	p := &Problems{}
	var cmds []Command
	complete := true
	if ordered {
		cmds, complete = ParseOrdered(p, literalContextGrammar(), input)
	} else {
		cmds = Parse(p, literalContextGrammar(), input)
	}
	if !complete {
		t.Fatalf("incomplete: %v", p.List)
	}
	if kind == "quoted-wrapper-openers" {
		if ordered {
			cmds = cmds[1:]
		}
		if len(p.List) != 0 || len(cmds) != 1 || len(cmds[0].Children) != 0 || !strings.HasPrefix(cmds[0].Body, "`<wb:change>` ") {
			t.Fatalf("quoted openers: commands=%+v problems=%v", cmds, p.List)
		}
		return
	}
	if kind == "panel-foo-desync" {
		if ordered {
			cmds = cmds[1:]
		}
		if len(p.List) != 0 || len(cmds) != count {
			t.Fatalf("desync count=%d want=%d problems=%v", len(cmds), count, p.List)
		}
		for _, c := range cmds[:count-1] {
			if c.Body != "`" || len(c.Children) != 1 || c.Children[0].FieldsOf("wb:old")[0] != "x</wb:panel><wb:foo>y" {
				t.Fatalf("desynchronized panel: %+v", c)
			}
		}
		if cmds[count-1].Body != "q </wb:foo>` </wb:panel><wb:log>r" {
			t.Fatalf("desynchronized log: %+v", cmds[count-1])
		}
		return
	}
	if strings.HasPrefix(kind, "wrapper-") {
		if ordered {
			cmds = cmds[1:]
		}
		if len(p.List) != 0 || len(cmds) != 1 || len(cmds[0].Children) != count || cmds[0].Body != "" {
			t.Fatalf("wrapper commands=%d children=%d want=%d problems=%v", len(cmds), len(cmds[0].Children), count, p.List)
		}
		field := "wb:" + strings.TrimSuffix(strings.TrimPrefix(kind, "wrapper-"), "-tilde")
		for i, child := range cmds[0].Children {
			want := "~~~x"
			if i == count-1 {
				want = "~~~"
			}
			if got := child.FieldsOf(field); len(got) != 1 || got[0] != want {
				t.Fatalf("child %d fields=%v want=%q", i, got, want)
			}
		}
		return
	}
	wantCommands := 1
	if strings.HasSuffix(kind, "-nolog") {
		wantCommands = 0
	}
	if ordered {
		wantCommands += count
		if !strings.HasSuffix(kind, "-nolog") {
			wantCommands++ // final accepted group sentinel
		}
	}
	if len(p.List) != count || len(cmds) != wantCommands {
		t.Fatalf("commands=%d want=%d problems=%d want=%d", len(cmds), wantCommands, len(p.List), count)
	}
	if !strings.HasSuffix(kind, "-nolog") {
		want := "```"
		if kind == "rejected-group-tilde" {
			want = "~~~"
		}
		if kind == "rejected-group-html-fence" {
			want = "```\n</b>"
		}
		if kind == "rejected-group-html-inline" {
			want = "`</b>"
		}
		if cmds[len(cmds)-1].Body != want {
			t.Fatalf("following log=%q want=%q", cmds[len(cmds)-1].Body, want)
		}
	}
}

func TestLiteralGroupEndpointIsNotContainerText(t *testing.T) {
	g := literalContextGrammar()
	input := "<wb:log>`</wb:log><wb:topic name=\"t\">text `"
	g0 := g
	g0.LiteralCode = false
	p, q := &Problems{}, &Problems{}
	got, complete := ParseOrdered(p, g, input)
	baseline, expectedComplete := ParseOrdered(q, g0, input)
	if complete != expectedComplete || len(p.List) != len(q.List) || len(got) != len(baseline) || got[0].Body != baseline[0].Body {
		t.Fatalf("group must retain structural reading: complete=%v/%v problems=%v/%v", complete, expectedComplete, p.List, q.List)
	}
}

func TestLiteralContextPerformanceBoundaries(t *testing.T) {
	for _, kind := range literalContextShapes {
		for _, ordered := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/ordered=%v", kind, ordered), func(t *testing.T) {
				input, count := literalContextInput(kind, 50000)
				checkLiteralContext(t, kind, input, count, ordered)
			})
		}
	}
}

var literalContextSizes = []int{50000, 100000, 200000, 400000, 800000}

const maxLiteralContextExponent = 1.5

// Fit time = constant * bytes^exponent in log space. Five sizes spanning 16x
// distinguish repeated suffix scans (exponent ~2) from linear/indexed walks
// (~1, or slightly higher for binary lookups and GC). The old 50/100/200KB
// adjacent-ratio gate failed on shared CI at 2.67/9.07/22.30ms despite local
// 50..800KB benchmarks fitting ~1.04. Batching alone did not remove that noise.
// Allow exponent <=1.5, halfway between linear and quadratic, while retaining
// the separate 100ms budget at 200KB. A sustained quadratic path grows 256x
// over this range, far beyond the allowed 64x, even if it fits the time budget.
func literalParseGrowthExponent(inputBytes []int, times []time.Duration) float64 {
	var meanX, meanY float64
	for i, size := range inputBytes {
		meanX += math.Log(float64(size))
		meanY += math.Log(float64(times[i]))
	}
	meanX /= float64(len(inputBytes))
	meanY /= float64(len(times))
	var covariance, variance float64
	for i, size := range inputBytes {
		x := math.Log(float64(size)) - meanX
		y := math.Log(float64(times[i])) - meanY
		covariance += x * y
		variance += x * x
	}
	return covariance / variance
}

func TestLiteralParseGrowthExponent(t *testing.T) {
	for _, exponent := range []float64{1, 1.25, 1.75, 2} {
		t.Run(fmt.Sprint(exponent), func(t *testing.T) {
			var times []time.Duration
			for _, size := range literalContextSizes {
				times = append(times, time.Duration(float64(time.Millisecond)*math.Pow(float64(size)/50000, exponent)))
			}
			if got := literalParseGrowthExponent(literalContextSizes, times); math.Abs(got-exponent) > 0.000001 {
				t.Fatalf("exponent=%f want=%f", got, exponent)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		micros []int
		accept bool
	}{
		// Actual CI failures followed by linear doubling: a transient slow
		// size transition must not condemn an otherwise linear larger walk.
		{"ci-tilde-linear-tail", []int{2666, 9073, 22304, 44608, 89216}, true},
		{"ci-fence-linear-tail", []int{3036, 6940, 26682, 53364, 106728}, true},
		{"quadratic-under-budget", []int{250, 1000, 4000, 16000, 64000}, false},
		{"linear-plus-quadratic", []int{1000, 3000, 10000, 36000, 136000}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var times []time.Duration
			for _, micros := range tc.micros {
				times = append(times, time.Duration(micros)*time.Microsecond)
			}
			exponent := literalParseGrowthExponent(literalContextSizes, times)
			if (exponent <= maxLiteralContextExponent) != tc.accept {
				t.Fatalf("exponent=%.3f accept=%v want=%v", exponent, exponent <= maxLiteralContextExponent, tc.accept)
			}
		})
	}
}

func TestLiteralContextNearLinearGrowth(t *testing.T) {
	g := literalContextGrammar()
	for _, kind := range literalContextShapes {
		for _, ordered := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/ordered=%v", kind, ordered), func(t *testing.T) {
				var bests []time.Duration
				var inputBytes []int
				for _, size := range literalContextSizes {
					input, count := literalContextInput(kind, size)
					checkLiteralContext(t, kind, input, count, ordered)
					best := bestLiteralParseTime(t, len(input), func() {
						p := &Problems{}
						if ordered {
							ParseOrdered(p, g, input)
						} else {
							Parse(p, g, input)
						}
					})
					inputBytes = append(inputBytes, len(input))
					bests = append(bests, best)
				}
				exponent := literalParseGrowthExponent(inputBytes, bests)
				t.Logf("50..800KB growth exponent=%.3f", exponent)
				if exponent > maxLiteralContextExponent {
					t.Errorf("superlinear growth: exponent=%.3f > %.1f: %v", exponent, maxLiteralContextExponent, bests)
				}
				if bests[2] >= 100*time.Millisecond {
					t.Errorf("200KB absolute bound: %s >= 100ms", bests[2])
				}
			})
		}
	}
}

func benchmarkLiteralWrapperAndRejectedGroups(b *testing.B) {
	g := literalContextGrammar()
	for _, kind := range literalContextShapes {
		for _, size := range literalContextSizes {
			input, _ := literalContextInput(kind, size)
			for _, ordered := range []bool{false, true} {
				b.Run(fmt.Sprintf("%s/%d/ordered=%v", kind, size, ordered), func(b *testing.B) {
					b.SetBytes(int64(len(input)))
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						p := &Problems{}
						if ordered {
							ParseOrdered(p, g, input)
						} else {
							Parse(p, g, input)
						}
					}
				})
			}
		}
	}
}
