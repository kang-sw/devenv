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
func codeBodyBoundary(code, closing string, grammar *Grammar, fields []string) int {
	first := -1
	var pending []literalFrame
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
		if len(pending) > 0 {
			top := pending[len(pending)-1]
			endTag := "</" + top.tag + ">"
			if strings.HasPrefix(rest, endTag) {
				pending = pending[:len(pending)-1]
				pos += len(endTag)
				continue
			}
			// A field/leaf body ends only at its own close. In particular,
			// literal parent closes in raw fields cannot pop a container.
			if top.raw && (top.wrapper == "" || !openingTagPrefix(rest, top.wrapper)) {
				pos++
				continue
			}
		}
		nameEnd := pos + 1
		for nameEnd < len(code) && isNameByte(code[nameEnd]) {
			nameEnd++
		}
		if nameEnd == pos+1 {
			pos++
			continue
		}
		name := code[pos+1 : nameEnd]
		frame, known := literalCommand(grammar, name)
		if len(pending) > 0 {
			top := pending[len(pending)-1]
			if !top.group {
				// Container children (including rejected unknown fields) are
				// raw fields. A wrapper itself is the one nested container.
				frame, known = literalFrame{tag: name, raw: !top.raw}, true
			} else if !known {
				frame, known = literalFrame{tag: name, raw: true}, true
			}
		} else if slices.Contains(fields, name) {
			frame, known = literalFrame{tag: name, raw: true}, true
		}
		limit := len(code)
		if !known {
			limit = min(limit, pos+256)
		}
		end, selfClosing, complete := literalTagEnd(code, nameEnd, limit)
		if first >= 0 && known {
			if !complete {
				return first
			}
			if !selfClosing {
				pending = append(pending, frame)
			}
		}
		if complete {
			pos = end
		} else {
			pos = nameEnd
		}
	}
	if len(pending) > 0 {
		return first
	}
	return -1
}

type literalFrame struct {
	tag        string
	raw, group bool
	wrapper    string
}

func literalCommand(g *Grammar, name string) (literalFrame, bool) {
	if g.isGroup(name) {
		return literalFrame{tag: name, group: true}, true
	}
	spec, known := g.lookup(name)
	frame := literalFrame{tag: name, raw: spec.Kind != Container}
	if spec.Wrapper != "" {
		frame.wrapper = g.Prefix + spec.Wrapper
	}
	return frame, known
}

// Boundary recognition does not validate attributes or emit diagnostics.
// Unknown literal names get only a bounded lexical probe, so malformed markup
// cannot trigger diagnostic lineAt prefix scans or unbounded repeated scans.
func literalTagEnd(s string, afterName, limit int) (end int, selfClosing, complete bool) {
	if afterName >= limit || !isSpace(s[afterName]) && s[afterName] != '>' && s[afterName] != '/' {
		return afterName, false, false
	}
	var quote byte
	for pos := afterName; pos < limit; pos++ {
		c := s[pos]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
		} else {
			switch c {
			case '\'', '"':
				quote = c
			case '<':
				return pos, false, false
			case '>':
				return pos + 1, pos > afterName && s[pos-1] == '/', true
			}
		}
	}
	return limit, false, false
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
		if boundary := codeBodyBoundary(t.s[at:end], closing, t.grammar, fields); boundary >= 0 {
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
