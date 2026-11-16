package wswhiteboard

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// Owner (1208)=b: crossing candidates are structural, without exceptions.
// Normalize only local D-A wrapper-opening quotes in the reference reading;
// inline/fenced spans must contain no own raw close outside CDATA. All other
// commands, operands and bytes remain structural comparisons, never skipped.
func literalStructuralExpected(input string) ([]map[string]any, bool) {
	g0 := grammar
	g0.LiteralCode = false
	p, q := &problems{}, &problems{}
	baseline := wsxmlcmd.Parse(p, g0, input)
	structural, complete := wsxmlcmd.ParseOrdered(q, g0, input)
	if !complete || len(p.List) != 0 || len(q.List) != 0 {
		return nil, false
	}
	masks := literalWrapperMasks(input, structural)
	// A single-byte marker preserves every line/source coordinate. Choose one
	// absent from the input, and restore it in body/field values before comparing.
	marker := byte(1)
	for strings.IndexByte(input, marker) >= 0 {
		marker++
		if marker == 0 {
			panic("no oracle marker available")
		}
	}
	masked := []byte(input)
	for at := range masks {
		masked[at] = marker
	}
	p = &problems{}
	expected := wsxmlcmd.Parse(p, g0, string(masked))
	if len(p.List) != 0 {
		// D-E: an invalid masked reading permits refusal or the original
		// structural result, not an automatically failing nil comparator.
		return canonicalCommands(baseline), true
	}
	var restore func([]command)
	restore = func(cmds []command) {
		for i := range cmds {
			cmds[i].Body = strings.ReplaceAll(cmds[i].Body, string(marker), "<")
			for j := range cmds[i].Fields {
				cmds[i].Fields[j].Body = strings.ReplaceAll(cmds[i].Fields[j].Body, string(marker), "<")
			}
			restore(cmds[i].Children)
		}
	}
	restore(expected)
	return canonicalCommands(expected), true
}

func literalSourceOpening(s string, at int, name string) bool {
	prefix := "<" + name
	after := at + len(prefix)
	return after < len(s) && strings.HasPrefix(s[at:], prefix) && strings.ContainsRune(" />\t\r\n", rune(s[after]))
}

// Skip only a wrapper not already consumed by an accepted panel-level quote.
// Thus a quoted opener's remaining text stays panel raw text, not a stale
// structural operand region. Clean structural input supplies complete tags.
func literalSkipWrapper(input string, at int, wrapper string) int {
	at = literalOpeningEnd(input, at)
	if input[at-2] == '/' {
		return at
	}
	for {
		for at < len(input) && strings.ContainsRune(" \t\r\n", rune(input[at])) {
			at++
		}
		close := "</" + wrapper + ">"
		if strings.HasPrefix(input[at:], close) {
			return at + len(close)
		}
		if !literalSourceOpening(input, at, "wb:old") && !literalSourceOpening(input, at, "wb:new") {
			return -1 // a quoted earlier opener exposed invalid container text
		}
		end := literalOpeningEnd(input, at)
		nameEnd := strings.IndexAny(input[at+1:end], " />\t\r\n") + at + 1
		if input[end-2] != '/' {
			close = "</" + input[at+1:nameEnd] + ">"
			for end < len(input) && !strings.HasPrefix(input[end:], close) {
				if strings.HasPrefix(input[end:], "<![CDATA[") {
					stop := strings.Index(input[end+9:], "]]>")
					if stop < 0 {
						return -1
					}
					end += 9 + stop + 3
				} else {
					end++
				}
			}
			if end == len(input) {
				return -1
			}
			end += len(close)
		}
		at = end
	}
}

func literalOracleRawClose(s string, start, end int, closing string) bool {
	for at := start; at < end; at++ {
		if strings.HasPrefix(s[at:], "<![CDATA[") {
			at += 9 + strings.Index(s[at+9:], "]]>") + 2
		} else if strings.HasPrefix(s[at:], closing) {
			return true
		}
	}
	return false
}

func literalWrapperMasks(input string, structural []command) map[int]bool {
	masks := map[int]bool{}
	sourceAt := 0
	for _, c := range structural {
		if c.IsGroup() {
			continue
		}
		start := strings.Index(input[sourceAt:], c.Source) + sourceAt
		sourceAt = start + len(c.Source)
		spec := grammar.Commands[strings.TrimPrefix(c.Tag, grammar.Prefix)]
		if spec.Wrapper == "" || strings.HasSuffix(c.Source, "/>") {
			continue
		}
		wrapper, closing := grammar.Prefix+spec.Wrapper, "</"+c.Tag+">"
		bodyStart := literalOpeningEnd(input, start)
		for at := bodyStart; at < sourceAt && !strings.HasPrefix(input[at:], closing); {
			if strings.HasPrefix(input[at:], "<![CDATA[") {
				at += 9 + strings.Index(input[at+9:], "]]>") + 3
				continue
			}
			if literalSourceOpening(input, at, wrapper) {
				next := literalSkipWrapper(input, at, wrapper)
				if next < 0 {
					break // masked structural parse handles this via D-E
				}
				at = next
				continue
			}
			if input[at] != '`' && input[at] != '~' {
				at++
				continue
			}
			mark, runEnd := input[at], at+1
			for runEnd < len(input) && input[runEnd] == mark {
				runEnd++
			}
			end := literalOracleFenceEnd(input, at, bodyStart, closing)
			if end <= at && mark == '`' {
				slashes := 0
				for i := at - 1; i >= bodyStart && input[i] == '\\'; i-- {
					slashes++
				}
				if slashes%2 == 0 {
					for next := runEnd; next < len(input); {
						index := strings.IndexByte(input[next:], '`')
						if index < 0 {
							break
						}
						next += index
						stop := next + 1
						for stop < len(input) && input[stop] == '`' {
							stop++
						}
						if stop-next == runEnd-at {
							end = stop
							break
						}
						next = stop
					}
				}
			}
			if end > at && !literalOracleRawClose(input, at, end, closing) {
				// CDATA is consumed atomically even when a span ends in it.
				for pos := at; pos < end; pos++ {
					if strings.HasPrefix(input[pos:], "<![CDATA[") {
						pos += 9 + strings.Index(input[pos+9:], "]]>") + 2
						end = max(end, pos+1)
					} else if literalSourceOpening(input, pos, wrapper) {
						masks[pos] = true
					}
				}
				at = end // accepted closer/inner runs are not new origins (m1)
			} else {
				// Rejected crossing/unmatched candidates cannot hide a later
				// valid local quote (M-7). Resume at the next source run.
				at = runEnd
			}
		}
	}
	return masks
}

func literalOracleFenceEnd(s string, at, bodyStart int, closing string) int {
	mark, runEnd := s[at], at+1
	for runEnd < len(s) && s[runEnd] == mark {
		runEnd++
	}
	lineStart := max(bodyStart, strings.LastIndexByte(s[:at], '\n')+1)
	if runEnd-at < 3 || at-lineStart > 3 || strings.Trim(s[lineStart:at], " ") != "" {
		return at
	}
	infoEnd := strings.IndexByte(s[runEnd:], '\n')
	if infoEnd < 0 || mark == '`' && strings.ContainsRune(s[runEnd:runEnd+infoEnd], '`') {
		return at
	}
	for line := runEnd + infoEnd + 1; line < len(s); {
		end := strings.IndexByte(s[line:], '\n')
		if end < 0 {
			end = len(s)
		} else {
			end += line
		}
		pos := line
		for pos < end && pos-line < 3 && s[pos] == ' ' {
			pos++
		}
		stop := pos
		for stop < end && s[stop] == mark {
			stop++
		}
		tail := strings.TrimSpace(s[stop:end])
		if stop-pos >= runEnd-at && (tail == "" || strings.HasPrefix(tail, closing)) {
			if tail != "" {
				return stop
			}
			return end
		}
		line = end + 1
	}
	return at
}

const literalWrapperInlineCounterexample = "<wb:topic name=\"t\"><wb:panel name=\"\">`<wb:change><wb:old>`</wb:old><wb:new></wb:new></wb:change></wb:panel></wb:topic>"

func literalWrapperPositiveBodies() []string {
	return []string{
		"`<wb:change><wb:old>`</wb:old><wb:new></wb:new></wb:change>",
		"`<wb:change/>`",
		"~~~xml\n<wb:change/> ` x `\n~~~",
		"```xml\n<wb:change/> ` x `\n```",
		"~~~xml\n<wb:change><wb:old>a</wb:old><wb:new>b</wb:new></wb:change>\n~~~",
		// M-7: the earlier run has a far closer but is not an active quote
		// across the panel close. It must not suppress the later fence mask.
		"`\n~~~xml\n<wb:change/>\n~~~\n``",
	}
}

func TestLiteralWrapperInlineQuotedOpenerIsInert(t *testing.T) {
	for _, body := range literalWrapperPositiveBodies() {
		input := "<wb:topic name=\"t\"><wb:panel name=\"p\" recap=\"r\">" + body + "</wb:panel></wb:topic>"
		g0 := grammar
		g0.LiteralCode = false
		p := &problems{}
		structural, complete := wsxmlcmd.ParseOrdered(p, g0, input)
		if !complete || len(p.List) != 0 || len(literalWrapperMasks(input, structural)) == 0 {
			t.Fatalf("missing Decision-10 span mask: %v", p.List)
		}
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
			if !complete || len(p.List) != 0 || len(got) != 1 || len(got[0].Children) != 0 || got[0].Body != body {
				t.Fatalf("Decision-10 literal changed: ordered=%v commands=%+v problems=%v", ordered, got, p.List)
			}
		}
		if bad, eligible := literalStructuralSplitFailure(input); bad || !eligible {
			t.Fatalf("Decision-10 oracle disagrees: bad=%v eligible=%v", bad, eligible)
		}
	}
}

func TestLiteralWrapperFenceAfterRejectedInlineRun(t *testing.T) {
	body := "`\n~~~xml\n<wb:change/>\n~~~"
	input := "<wb:topic name=\"t\"><wb:panel name=\"p\">" + body + "</wb:panel><wb:log>later `</wb:log></wb:topic>"
	p := &problems{}
	got := wsxmlcmd.Parse(p, grammar, input)
	if len(p.List) != 0 || len(got) != 2 || got[0].Body != body || len(got[0].Children) != 0 || got[1].Body != "later `" {
		t.Fatalf("later fence changed: %+v %v", got, p.List)
	}
	if bad, eligible := literalStructuralSplitFailure(input); bad || !eligible {
		t.Fatalf("earlier far inline closer hid fence: bad=%v eligible=%v", bad, eligible)
	}
}

func TestLiteralWrapperOracleKeepsCloseCrossings(t *testing.T) {
	inputs := append(literalWrapperContextInputs(), literalPanelOriginInputs()...)
	inputs = append(inputs,
		"<wb:topic name=\"t\"><wb:panel name=\"p\">`<wb:change/></wb:panel><wb:log>`</wb:log></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel name=\"p\"><![CDATA[`<wb:change/>`]]></wb:panel></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel name=\"p\">\\`<wb:change/>`</wb:panel></wb:topic>",
	)
	g0 := grammar
	g0.LiteralCode = false
	for _, input := range inputs {
		p := &problems{}
		structural, complete := wsxmlcmd.ParseOrdered(p, g0, input)
		if !complete || len(p.List) != 0 || len(literalWrapperMasks(input, structural)) != 0 {
			t.Fatalf("improperly masked or malformed fixture: %q %v", input, p.List)
		}
		if bad, eligible := literalStructuralSplitFailure(input); bad || !eligible {
			t.Fatalf("crossing oracle: bad=%v eligible=%v input=%q", bad, eligible, input)
		}
	}
}

func TestLiteralWrapperOracleCannotMaskAnotherSpanOrCommand(t *testing.T) {
	quote := "<wb:panel name=\"q\">`<wb:change/>`</wb:panel>"
	// Check both another command (C-1) and another span in this same command
	// (I-A inside-panel variant); neither may inherit the local quote's mask.
	inputs := []string{
		strings.Replace(literalWrapperContextInputs()[0], "<wb:panel", quote+"<wb:panel", 1),
		strings.Replace(literalPanelOriginInputs()[1], ">`<wb:change>", ">`<wb:change/>` ` <wb:change>", 1),
	}
	for _, input := range inputs {
		want, eligible := literalStructuralExpected(input)
		if !eligible || want == nil {
			t.Fatal("local quote masked eligibility of the remaining input")
		}
		p := &problems{}
		got := wsxmlcmd.Parse(p, grammar, input)
		if len(p.List) != 0 || !reflect.DeepEqual(want, canonicalCommands(got)) {
			t.Fatalf("unmutated parse differs: %v", p.List)
		}
		// Simulate the C-1/I-A loss of a child after the unrelated quote.
		// An input-wide or command-wide skip would hide this mutation.
		last := len(got) - 1
		got[last].Children = nil
		if reflect.DeepEqual(want, canonicalCommands(got)) {
			t.Fatal("oracle masked a non-exempt child's loss")
		}
	}
}
