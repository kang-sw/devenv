package wswhiteboard

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// Decision 10 deliberately quotes wrapper openings in panel raw text. I3's
// narrow exception is an INLINE span originating there, crossing a structural
// wrapper opener, and ending before that panel's close. Operand-origin spans,
// fences and spans crossing the panel close are never exempted by this rule.
// Inspect raw source, not decoded Body or the literal parser's chosen result.
func literalInlineWrapperException(input string, structural []command) bool {
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
		bodyStart := literalOpeningEnd(input, start)
		bodyEnd := start + strings.LastIndex(c.Source, "</"+c.Tag+">")
		wrapper := grammar.Prefix + spec.Wrapper
		var raw []literalRawSpan
		var openings []int
		from := bodyStart
		for at := bodyStart; at < bodyEnd; {
			if strings.HasPrefix(input[at:], "<![CDATA[") {
				at += 9 + strings.Index(input[at+9:], "]]>") + 3
				continue
			}
			prefix := "<" + wrapper
			after := at + len(prefix)
			if !strings.HasPrefix(input[at:], prefix) || !strings.ContainsRune(" />\t\r\n", rune(input[after])) {
				at++
				continue
			}
			raw = append(raw, literalRawSpan{from, at})
			openings = append(openings, at)
			at = literalOpeningEnd(input, at)
			if input[at-2] != '/' {
				for {
					for strings.ContainsRune(" \t\r\n", rune(input[at])) {
						at++
					}
					close := "</" + wrapper + ">"
					if strings.HasPrefix(input[at:], close) {
						at += len(close)
						break
					}
					// Structural I3 inputs are clean: here there is a known
					// field opening, followed by its raw CDATA-aware close.
					end := literalOpeningEnd(input, at)
					nameEnd := strings.IndexAny(input[at+1:end], " />\t\r\n") + at + 1
					if input[end-2] != '/' {
						close = "</" + input[at+1:nameEnd] + ">"
						for !strings.HasPrefix(input[end:], close) {
							if strings.HasPrefix(input[end:], "<![CDATA[") {
								end += 9 + strings.Index(input[end+9:], "]]>") + 3
							} else {
								end++
							}
						}
						end += len(close)
					}
					at = end
				}
			}
			from = at
		}
		raw = append(raw, literalRawSpan{from, bodyEnd})
		covered := bodyStart
		for _, region := range raw {
			for at := max(region.start, covered); at < region.end; {
				if strings.HasPrefix(input[at:], "<![CDATA[") {
					at += 9 + strings.Index(input[at+9:], "]]>") + 3
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
				if end := literalOracleFenceEnd(input, at, bodyStart, "</"+c.Tag+">"); end > at {
					covered, at = end, end
					continue // a fence is not the approved inline exception
				}
				slashes := 0
				for i := at - 1; i >= bodyStart && input[i] == '\\'; i-- {
					slashes++
				}
				if mark == '`' && slashes%2 == 0 {
					for next := runEnd; next < len(input); {
						index := strings.IndexByte(input[next:], '`')
						if index < 0 {
							break
						}
						next += index
						end := next + 1
						for end < len(input) && input[end] == '`' {
							end++
						}
						if end-next == runEnd-at {
							if end <= bodyEnd {
								for _, opening := range openings {
									if opening >= runEnd && opening < next {
										return true
									}
								}
							}
							covered = end
							break
						}
						next = end
					}
				}
				at = max(runEnd, covered)
			}
		}
	}
	return false
}

// Independent source-only fence recognition prevents a fence containing ticks
// from being mislabeled as an inline exception. Search the whole input so a
// later fence closer beyond the panel close cannot qualify as inline instead.
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

func TestLiteralWrapperInlineQuotedOpenerIsInert(t *testing.T) {
	for _, input := range []string{literalWrapperInlineCounterexample, "<wb:topic name=\"t\"><wb:panel name=\"p\">`<wb:change/>`</wb:panel></wb:topic>"} {
		g0 := grammar
		g0.LiteralCode = false
		p := &problems{}
		structural, complete := wsxmlcmd.ParseOrdered(p, g0, input)
		if !complete || len(p.List) != 0 || !literalInlineWrapperException(input, structural) {
			t.Fatalf("not a narrow inline exception: %v", p.List)
		}
		for _, ordered := range []bool{false, true} {
			p := &problems{}
			var got []command
			if ordered {
				got, complete = wsxmlcmd.ParseOrdered(p, grammar, input)
				got = got[1:]
			} else {
				got = wsxmlcmd.Parse(p, grammar, input)
			}
			body := input[strings.Index(input, ">`")+1 : strings.LastIndex(input, "</wb:panel>")]
			if !complete || len(p.List) != 0 || len(got) != 1 || len(got[0].Children) != 0 || got[0].Body != body {
				t.Fatalf("Decision-10 literal changed: ordered=%v commands=%+v problems=%v", ordered, got, p.List)
			}
		}
	}
}

func TestLiteralWrapperOracleKeepsCloseCrossings(t *testing.T) {
	inputs := literalWrapperContextInputs() // C-1 and mixed-payload variant
	inputs = append(inputs,
		"<wb:topic name=\"t\"><wb:panel name=\"p\">`<wb:change/></wb:panel><wb:log>`</wb:log></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel name=\"p\">~~~xml\n<wb:change/> ` x `\n~~~\n</wb:panel></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel name=\"p\">```xml\n<wb:change/> ` x `\n```\n</wb:panel></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel name=\"p\"><![CDATA[`<wb:change/>`]]></wb:panel></wb:topic>",
		"<wb:topic name=\"t\"><wb:panel name=\"p\">\\`<wb:change/>`</wb:panel></wb:topic>",
	)
	g0 := grammar
	g0.LiteralCode = false
	for _, input := range inputs {
		p := &problems{}
		structural, complete := wsxmlcmd.ParseOrdered(p, g0, input)
		if !complete || len(p.List) != 0 || literalInlineWrapperException(input, structural) {
			t.Fatalf("improperly exempted or malformed fixture: %q %v", input, p.List)
		}
	}
}
