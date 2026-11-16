package wsxmlcmd

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var literalContextShapes = []string{"wrapper-old-tilde", "wrapper-new-tilde", "rejected-group-fence", "rejected-group-tilde", "rejected-group-fence-nolog"}

func literalContextGrammar() Grammar {
	g := literalPerformanceGrammar()
	g.Commands["panel"] = Spec{Kind: Leaf, Attrs: []string{"name"}, Wrapper: "change", Fields: []string{"old", "new"}, Scoped: true}
	return g
}

func literalContextInput(kind string, size int) (string, int) {
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
		if cmds[len(cmds)-1].Body != want {
			t.Fatalf("following log=%q want=%q", cmds[len(cmds)-1].Body, want)
		}
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

func TestLiteralContextNearLinearGrowth(t *testing.T) {
	g := literalContextGrammar()
	for _, kind := range literalContextShapes {
		for _, ordered := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/ordered=%v", kind, ordered), func(t *testing.T) {
				var bests []time.Duration
				for _, size := range []int{50000, 100000, 200000} {
					input, count := literalContextInput(kind, size)
					checkLiteralContext(t, kind, input, count, ordered)
					best := time.Hour
					for trial := 0; trial < 3; trial++ {
						p := &Problems{}
						start := time.Now()
						if ordered {
							ParseOrdered(p, g, input)
						} else {
							Parse(p, g, input)
						}
						best = min(best, time.Since(start))
					}
					bests = append(bests, best)
					t.Logf("%d bytes best-of-3=%s", len(input), best)
				}
				if bests[2] >= 100*time.Millisecond || bests[2] > 6*bests[0]+5*time.Millisecond || bests[1] > 3*bests[0]+5*time.Millisecond || bests[2] > 3*bests[1]+5*time.Millisecond {
					t.Fatalf("superlinear or over 100ms at 200KB: %v", bests)
				}
			})
		}
	}
}

func benchmarkLiteralWrapperAndRejectedGroups(b *testing.B) {
	g := literalContextGrammar()
	for _, kind := range literalContextShapes {
		for _, size := range []int{50000, 100000, 200000} {
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
