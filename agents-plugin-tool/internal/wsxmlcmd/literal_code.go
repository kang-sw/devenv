package wsxmlcmd

import (
	"sort"
	"strings"
)

type codeRun struct{ start, end int }
type fenceKey struct {
	mark    byte
	closing string
}
type fenceClose struct{ start, end, run int }

// Delimiter positions are indexed once per input. In particular, failed fence
// candidates never rescan every remaining line, and span lookup never rescans
// unmatched backtick runs. The max tree finds the next long-enough fence.
type literalIndex struct {
	spans    map[int][]codeRun
	fences   map[fenceKey]*fenceIndex
	newlines []int
}
type fenceIndex struct {
	closes []fenceClose
	max    []int
	size   int
}

func indexLiterals(s string) *literalIndex {
	out := &literalIndex{spans: map[int][]codeRun{}, fences: map[fenceKey]*fenceIndex{}}
	for pos := 0; pos < len(s); pos++ {
		if s[pos] != '`' {
			continue
		}
		end := pos + 1
		for end < len(s) && s[end] == '`' {
			end++
		}
		out.spans[end-pos] = append(out.spans[end-pos], codeRun{pos, end})
		pos = end - 1
	}
	for start := 0; start < len(s); {
		end := strings.IndexByte(s[start:], '\n')
		if end < 0 {
			end = len(s)
		} else {
			end += start
		}
		pos := start
		for pos < end && pos-start < 3 && s[pos] == ' ' {
			pos++
		}
		if pos < end && (s[pos] == '`' || s[pos] == '~') {
			mark, runEnd := s[pos], pos+1
			for runEnd < end && s[runEnd] == mark {
				runEnd++
			}
			if runEnd-pos >= 3 {
				tail := strings.TrimSpace(s[runEnd:end])
				closing := ""
				if strings.HasPrefix(tail, "</") {
					if close := strings.IndexByte(tail, '>'); close >= 0 {
						closing = tail[:close+1]
					}
				}
				if tail == "" || closing != "" {
					key := fenceKey{mark, closing}
					if out.fences[key] == nil {
						out.fences[key] = &fenceIndex{}
					}
					literalEnd := end
					if closing != "" {
						literalEnd = runEnd
					}
					out.fences[key].closes = append(out.fences[key].closes, fenceClose{pos, literalEnd, runEnd - pos})
				}
			}
		}
		if end == len(s) {
			break
		}
		out.newlines = append(out.newlines, end)
		start = end + 1
	}
	for _, f := range out.fences {
		f.size = 1
		for f.size < len(f.closes) {
			f.size *= 2
		}
		f.max = make([]int, 2*f.size)
		for i, close := range f.closes {
			f.max[f.size+i] = close.run
		}
		for i := f.size - 1; i > 0; i-- {
			f.max[i] = max(f.max[2*i], f.max[2*i+1])
		}
	}
	return out
}

func (f *fenceIndex) next(after, run int) int {
	if f == nil {
		return 0
	}
	first := sort.Search(len(f.closes), func(i int) bool { return f.closes[i].start >= after })
	var find func(node, lo, hi int) int
	find = func(node, lo, hi int) int {
		if hi <= first || f.max[node] < run {
			return -1
		}
		if hi-lo == 1 {
			return lo
		}
		mid := (lo + hi) / 2
		if found := find(node*2, lo, mid); found >= 0 {
			return found
		}
		return find(node*2+1, mid, hi)
	}
	if i := find(1, 0, f.size); i >= 0 {
		return f.closes[i].end
	}
	return 0
}

// firstBodyClose does no tag or quote lexing: only CDATA can hide the exact
// close of a raw body. In particular, an HTML quote cannot cross that close.
func firstBodyClose(s string, at, end int, closing string) int {
	for at < end {
		next := strings.IndexByte(s[at:end], '<')
		if next < 0 {
			break
		}
		at += next
		if strings.HasPrefix(s[at:], closing) {
			return at
		}
		if strings.HasPrefix(s[at:], "<![CDATA[") {
			stop := strings.Index(s[at+9:], "]]>")
			if stop < 0 {
				return -1
			}
			at += 9 + stop + 3
		} else {
			at++
		}
	}
	return -1
}

// A matched candidate crossing its own raw close is not a literal. Structural
// reading wins without adding a refusal; only existing problems gain guidance.
// Cache that raw close to bound repeated probes. Once reading has passed it
// (for example by consuming a wrapper operand), later local quotes start fresh.
// limit also bounds rejected-group lexical probes in readContent.
func (t *tokenizer) codeLiteralEnd(at, bodyStart, lineStart int, closing string, limit *int) int {
	if *limit < at {
		*limit = len(t.s)
	}
	end := t.matchCodeLiteral(at, bodyStart, lineStart, closing, len(t.s))
	if end <= at {
		return at
	}
	first := -1
	if *limit < end {
		first = *limit
	} else {
		first = firstBodyClose(t.s, at, end, closing)
	}
	if first >= 0 {
		*limit = first
		if at >= t.crossingUntil {
			t.crossingFrom = at
		}
		t.crossingUntil = max(t.crossingUntil, end)
		t.crossings++
		return at
	}
	return end
}

// A panel-level quote may contain whole wrapper examples or a quoted opener,
// but it must not end halfway through a structural operand. Otherwise the raw
// payload can silently replace a patch instead of leaving a visible refusal.
// Walk only the candidate's bytes, skipping operand bodies by their own raw
// close (and CDATA), just as structural reading does.
func literalOperandCrossing(s string, at, end int, wrapper *bodyWrapper) string {
	inside := false
	for at < end {
		next := strings.IndexByte(s[at:end], '<')
		if next < 0 {
			break
		}
		at += next
		rest := s[at:end]
		if strings.HasPrefix(rest, "<![CDATA[") {
			close := strings.Index(rest[9:], "]]>")
			if close < 0 {
				break
			}
			at += 9 + close + 3
			continue
		}
		if inside && strings.HasPrefix(rest, "</"+wrapper.tag+">") {
			inside = false
			at += len(wrapper.tag) + 3
			continue
		}
		match := !inside && openingTagPrefix(rest, wrapper.tag)
		if inside {
			for _, field := range wrapper.fields {
				match = match || openingTagPrefix(rest, field)
			}
		}
		if !match {
			at++
			continue
		}
		probe := tokenizer{s: s[:end], i: at, linePos: at, p: &Problems{}}
		tag, ok := probe.readOpen()
		if !ok {
			at++
			continue
		}
		at = probe.i
		if !inside {
			inside = !tag.selfClosing
			continue
		}
		if !tag.selfClosing {
			closing := "</" + tag.name + ">"
			close := firstBodyClose(s, at, end, closing)
			if close < 0 {
				return tag.name
			}
			at = close + len(closing)
		}
	}
	return ""
}

// Matching indexes positions only; no fallback grammar walk licenses crossings.
func (t *tokenizer) matchCodeLiteral(at, bodyStart, lineStart int, closing string, limit int) int {
	mark := t.s[at]
	if mark != '`' && mark != '~' || at > bodyStart && t.s[at-1] == mark {
		return at
	}
	run := 1
	for at+run < len(t.s) && t.s[at+run] == mark {
		run++
	}
	indent := t.s[lineStart:at]
	if run >= 3 && len(indent) <= 3 && strings.Trim(indent, " ") == "" {
		if t.literals == nil {
			t.literals = indexLiterals(t.s)
		}
		nextLine := sort.SearchInts(t.literals.newlines, at+run)
		if nextLine < len(t.literals.newlines) {
			infoEnd := t.literals.newlines[nextLine]
			if mark != '`' || !strings.ContainsRune(t.s[at+run:infoEnd], '`') {
				end := t.literals.fences[fenceKey{mark, ""}].next(infoEnd+1, run)
				framed := t.literals.fences[fenceKey{mark, closing}].next(infoEnd+1, run)
				if end == 0 || framed != 0 && framed < end {
					end = framed
				}
				if end != 0 {
					return end
				}
			}
		}
	}
	if mark != '`' {
		return at
	}
	slashes := 0
	for pos := at - 1; pos >= bodyStart && t.s[pos] == '\\'; pos-- {
		slashes++
	}
	if slashes%2 != 0 {
		return at
	}
	// Short, ordinary code spans need no input-wide index or allocations.
	// Cap this direct probe so many unmatched runs still have bounded cost.
	if t.literals == nil {
		window := min(len(t.s), at+run+256, limit)
		for pos := at + run; pos < window; {
			next := strings.IndexByte(t.s[pos:window], '`')
			if next < 0 {
				break
			}
			pos += next
			end := pos + 1
			for end < len(t.s) && t.s[end] == '`' {
				end++
			}
			if end-pos == run {
				return end
			}
			pos = end
		}
		if window == len(t.s) || window == limit {
			return at
		}
		t.literals = indexLiterals(t.s)
	}
	runs := t.literals.spans[run]
	i := sort.Search(len(runs), func(i int) bool { return runs[i].start > at })
	if i < len(runs) {
		return runs[i].end
	}
	return at
}
