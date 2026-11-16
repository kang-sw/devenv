package wsxmlcmd

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

var literalPerformanceShapes = []string{
	"single-tildes", "single-unmatched-fences", "single-malformed-markup",
	"fence-per-log", "tilde-per-log", "fence-per-panel", "fence-per-field",
	"inline-distinct-runs", "ambiguous-chain", "delimiter-free-walk",
}

func literalPerformanceGrammar() Grammar {
	g := Grammar{Prefix: "wb:", Group: &Group{Name: "topic", Attr: "name"}, Commands: map[string]Spec{
		"panel": {Kind: Leaf, Attrs: []string{"recap"}, Scoped: true},
		"log":   {Kind: Leaf, Scoped: true},
		"add":   {Kind: Container, Attrs: []string{"class"}, Fields: []string{"title"}, Scoped: true},
	}}
	// The identical benchmark source also compiles against the frozen v0.50.0
	// parser, which has no LiteralCode field. No grammar rule otherwise differs.
	if flag := reflect.ValueOf(&g).Elem().FieldByName("LiteralCode"); flag.IsValid() {
		flag.SetBool(true)
	}
	return g
}

func literalPerformanceInput(kind string, size int) (string, int) {
	const panel = `<wb:panel recap="r">`
	const closePanel = `</wb:panel>`
	var b strings.Builder
	b.WriteString(`<wb:topic name="t">`)
	count := 0
	add := func(open, body, close string) {
		b.WriteString(open)
		b.WriteString(body)
		b.WriteString(close)
		count++
	}
	switch kind {
	case "single-tildes":
		add(panel, strings.Repeat("~a", size/2), closePanel)
	case "single-unmatched-fences":
		add(panel, "```\n"+strings.Repeat("```x\n", size/5), closePanel)
	case "single-malformed-markup":
		add(panel, "`<![CDATA[</wb:panel>]]>"+strings.Repeat("<a?", size/3)+"`", closePanel)
	case "fence-per-log", "tilde-per-log", "fence-per-panel", "fence-per-field":
		open, close, mark := "<wb:log>", "</wb:log>", "```"
		if kind == "tilde-per-log" {
			mark = "~~~"
		}
		if kind == "fence-per-panel" {
			open, close = panel, closePanel
		}
		if kind == "fence-per-field" {
			open, close = `<wb:add class="technical"><wb:title>`, `</wb:title></wb:add>`
		}
		for b.Len()+len(open)+len(close)+len(mark)+2 < size {
			add(open, mark+"x\n", close)
		}
		add(open, "\n"+mark+"\n", close)
	case "inline-distinct-runs":
		for run := 1; b.Len() < size; run++ {
			add(panel, strings.Repeat("`", run)+"x", closePanel)
		}
	case "ambiguous-chain":
		for b.Len() < size {
			add(panel, "`", closePanel)
		}
	case "delimiter-free-walk":
		add(panel, "```x\n", closePanel)
		for b.Len() < size {
			add(panel, "ordinary body without delimiters", closePanel)
		}
		add(panel, "\n```\n", closePanel)
	default:
		panic(kind)
	}
	b.WriteString(`</wb:topic>`)
	return b.String(), count
}

// I-A is a correctness regression as well as a benchmark: both entry points
// must preserve every body/field boundary on all multi-body shapes.
func TestLiteralCodePerformanceShapesPreserveBoundaries(t *testing.T) {
	g := literalPerformanceGrammar()
	for _, kind := range literalPerformanceShapes {
		t.Run(kind, func(t *testing.T) {
			input, count := literalPerformanceInput(kind, 50000)
			p := &Problems{}
			cmds := Parse(p, g, input)
			q := &Problems{}
			ordered, complete := ParseOrdered(q, g, input)
			if len(p.List) != 0 || len(q.List) != 0 || !complete || len(cmds) != count || len(ordered) != count+1 {
				t.Fatalf("count=%d/%d want=%d complete=%v problems=%v/%v", len(cmds), len(ordered), count, complete, p.List, q.List)
			}
			for i, c := range cmds {
				other := ordered[i+1]
				if c.Tag != other.Tag || c.Body != other.Body || !reflect.DeepEqual(c.Fields, other.Fields) {
					t.Fatalf("entry points differ at %d", i)
				}
				want := ""
				switch kind {
				case "fence-per-log", "fence-per-panel", "fence-per-field":
					want = "```x"
					if i == count-1 {
						want = "```"
					}
				case "tilde-per-log":
					want = "~~~x"
					if i == count-1 {
						want = "~~~"
					}
				case "inline-distinct-runs":
					want = strings.Repeat("`", i+1) + "x"
				case "ambiguous-chain":
					want = "`"
				case "delimiter-free-walk":
					want = "ordinary body without delimiters"
					if i == 0 {
						want = "```x"
					}
					if i == count-1 {
						want = "```"
					}
				}
				if want != "" {
					body := c.Body
					if kind == "fence-per-field" {
						body = c.Fields[0].Body
					}
					if body != want {
						t.Fatalf("body %d=%q want %q", i, body, want)
					}
				}
			}
		})
	}
}

func bestLiteralParseTime(t *testing.T, inputBytes int, parse func()) time.Duration {
	t.Helper()
	const trials = 7
	const minSample = 20 * time.Millisecond
	iterations := 1
	best := time.Hour
	for trial := 0; trial < trials; {
		runtime.GC()
		start := time.Now()
		for range iterations {
			parse()
		}
		elapsed := time.Since(start)
		// Single sub-millisecond parses are dominated by runner pauses and
		// coarse timers. Require seven batches at the same iteration count,
		// each long enough to amortize that noise, then normalize.
		if elapsed < minSample {
			iterations *= 2
			best, trial = time.Hour, 0
			continue
		}
		best = min(best, elapsed)
		trial++
	}
	best /= time.Duration(iterations)
	t.Logf("%d bytes best-of-%d=%s per parse (%d parses/batch)", inputBytes, trials, best, iterations)
	return best
}

// bestInterleavedLiteralParseTimes fixes batch counts before collecting samples.
// Every round visits every size, so a busy runner cannot isolate one size in a
// separate sampling window. Keep the best per size to discard scheduler pauses.
func bestInterleavedLiteralParseTimes(t *testing.T, inputs []string, parse func(string)) []time.Duration {
	t.Helper()
	const trials = 4
	const minSample = 30 * time.Millisecond
	const minIterations = 8 // Even the largest input gets several parses per batch.
	iterations := make([]int, len(inputs))
	bests := make([]time.Duration, len(inputs))
	batch := func(input string, n int) time.Duration {
		runtime.GC()
		start := time.Now()
		for range n {
			parse(input)
		}
		return time.Since(start)
	}
	// Calibration is not a sample: only the subsequent interleaved rounds
	// contribute to the growth comparison, all with these fixed batch counts.
	for i, input := range inputs {
		iterations[i] = minIterations
		for batch(input, iterations[i]) < minSample {
			iterations[i] *= 2
		}
		bests[i] = time.Hour
	}
	for range trials {
		for i, input := range inputs {
			elapsed := batch(input, iterations[i]) / time.Duration(iterations[i])
			bests[i] = min(bests[i], elapsed)
		}
	}
	for i, input := range inputs {
		t.Logf("%d bytes interleaved best-of-%d=%s per parse (%d parses/batch)", len(input), trials, bests[i], iterations[i])
	}
	return bests
}

func TestLiteralCodeNearLinearGrowth(t *testing.T) {
	g := literalPerformanceGrammar()
	for _, kind := range literalPerformanceShapes {
		t.Run(kind, func(t *testing.T) {
			var inputs []string
			for _, size := range []int{50000, 100000, 200000} {
				input, _ := literalPerformanceInput(kind, size)
				inputs = append(inputs, input)
			}
			parse := func(input string) {
				p := &Problems{}
				Parse(p, g, input)
				if len(p.List) != 0 {
					t.Fatal(p.List)
				}
			}
			const attempts = 3
			for attempt := 1; attempt <= attempts; attempt++ {
				t.Logf("interleaved growth attempt %d/%d", attempt, attempts)
				bests := bestInterleavedLiteralParseTimes(t, inputs, parse)
				if bests[2] >= 100*time.Millisecond {
					t.Fatalf("200KB absolute bound: %s >= 100ms", bests[2])
				}
				// Preserve the existing ratio/slack gates. A transient failure
				// retries the entire set, never cherry-picking sizes across attempts.
				reason := ""
				for i := 1; i < len(bests); i++ {
					if bests[i] > 3*bests[i-1]+5*time.Millisecond {
						reason = "superlinear growth"
					}
				}
				if bests[2] > 6*bests[0]+5*time.Millisecond {
					reason = "fourfold-size growth"
				}
				if reason == "" {
					return
				}
				if attempt == attempts {
					t.Fatalf("%s after %d attempts: %v", reason, attempts, bests)
				}
				t.Logf("%s: %v; retrying whole interleaved set", reason, bests)
			}
		})
	}
}

func BenchmarkLiteralCodeStructuralWalk(b *testing.B) {
	benchmarkLiteralWrapperAndRejectedGroups(b)
	g := literalPerformanceGrammar()
	for _, kind := range literalPerformanceShapes {
		for _, size := range []int{50000, 100000, 200000} {
			input, _ := literalPerformanceInput(kind, size)
			b.Run(fmt.Sprintf("%s/%d", kind, size), func(b *testing.B) {
				b.SetBytes(int64(len(input)))
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					p := &Problems{}
					Parse(p, g, input)
				}
			})
		}
	}
}
