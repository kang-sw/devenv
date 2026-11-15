package wsxmlcmd

import (
	"slices"
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
	spans  map[int][]codeRun
	fences map[fenceKey]*fenceIndex
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

// A candidate may quote enclosing closing tags, but cannot end partway through
// a following command/group/operand. Complete balanced examples remain inert.
// Return the first enclosing close on rejection, so subsequent candidates in
// this body can be bounded without rescanning the same following commands.
func codeBodyBoundary(code, closing string, commands, fields []string) int {
	first := -1
	var pending []string
	for pos := 0; pos < len(code); {
		next := strings.IndexByte(code[pos:], '<')
		if next < 0 {
			break
		}
		pos += next
		rest := code[pos:]
		if strings.HasPrefix(rest, "<![CDATA[") {
			if end := strings.Index(rest[9:], "]]>"); end >= 0 {
				pos += 9 + end + 3
				continue
			}
		}
		if first < 0 && strings.HasPrefix(rest, closing) {
			first = pos
			pos += len(closing)
			continue
		}
		if first >= 0 && strings.HasPrefix(rest, "</") {
			if end := strings.IndexByte(rest, '>'); end >= 0 {
				name := rest[2:end]
				if len(pending) > 0 && pending[len(pending)-1] == name {
					pending = pending[:len(pending)-1]
				}
				pos += end + 1
				continue
			}
		}
		if first >= 0 && len(rest) > 1 && isNameByte(rest[1]) {
			nameEnd := 1
			for nameEnd < len(rest) && isNameByte(rest[nameEnd]) {
				nameEnd++
			}
			name := rest[1:nameEnd]
			probe := tokenizer{s: code, i: pos, p: &Problems{}}
			tag, ok := probe.readOpen()
			if slices.Contains(commands, name) || slices.Contains(fields, name) {
				if !ok {
					return first
				}
				if !tag.selfClosing {
					pending = append(pending, name)
				}
			}
			if ok {
				pos = probe.i
				continue
			}
		}
		pos++
	}
	if len(pending) > 0 {
		return first
	}
	return -1
}

func (t *tokenizer) codeLiteralEnd(at, bodyStart, lineStart int, closing string, fields []string, limit *int) int {
	mark := t.s[at]
	if mark != '`' && mark != '~' || at > bodyStart && t.s[at-1] == mark {
		return at
	}
	run := 1
	for at+run < len(t.s) && t.s[at+run] == mark {
		run++
	}
	withinBody := func(end int) int {
		if end <= at || end > *limit {
			return at
		}
		if boundary := codeBodyBoundary(t.s[at:end], closing, t.commandTags, fields); boundary >= 0 {
			*limit = min(*limit, at+boundary)
			return at
		}
		return end
	}
	indent := t.s[lineStart:at]
	if run >= 3 && len(indent) <= 3 && strings.Trim(indent, " ") == "" {
		if newline := strings.IndexByte(t.s[at+run:], '\n'); newline >= 0 {
			infoEnd := at + run + newline
			if mark != '`' || !strings.ContainsRune(t.s[at+run:infoEnd], '`') {
				if t.literals == nil {
					t.literals = indexLiterals(t.s)
				}
				end := t.literals.fences[fenceKey{mark, ""}].next(infoEnd+1, run)
				framed := t.literals.fences[fenceKey{mark, closing}].next(infoEnd+1, run)
				if end == 0 || framed != 0 && framed < end {
					end = framed
				}
				if end != 0 {
					return withinBody(end)
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
		window := min(len(t.s), at+run+256, *limit)
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
				return withinBody(end)
			}
			pos = end
		}
		if window == len(t.s) || window == *limit {
			return at
		}
		t.literals = indexLiterals(t.s)
	}
	runs := t.literals.spans[run]
	i := sort.Search(len(runs), func(i int) bool { return runs[i].start > at })
	if i < len(runs) {
		return withinBody(runs[i].end)
	}
	return at
}
