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

// Walk the fallback interpretation (the enclosing body ended at first).
// Accept only an end in enclosing-level text. Raw frames share readContent's
// matcher: if a frame's candidate crosses its own close, reject immediately
// rather than assuming that close ends the frame, or recursively walking it.
// Thus a long walk stops at the next body that could originate a long walk.
func (t *tokenizer) codeBodyBoundary(first, end int, closing string, fields bodyFieldRules) bool {
	pending := slices.Clone(fields.enclosing)
	pos := first + len(closing)
	// A suspended raw parent can resume after several operand fields. Track
	// source newlines across every frame, not only the currently raw frame.
	// Index once instead of rescanning the parent's prefix for each operand.
	if t.literals == nil {
		t.literals = indexLiterals(t.s)
	}
	lineStart := 0
	if i := sort.SearchInts(t.literals.newlines, pos); i > 0 {
		lineStart = t.literals.newlines[i-1] + 1
	}
	advance := func(next int) {
		if nl := strings.LastIndexByte(t.s[pos:next], '\n'); nl >= 0 {
			lineStart = pos + nl + 1
		}
		pos = next
	}
	for pos < end {
		rest := t.s[pos:]
		if strings.HasPrefix(rest, "<![CDATA[") {
			stop := strings.Index(rest[9:], "]]>")
			if stop < 0 || pos+9+stop+3 > end {
				return false
			}
			advance(pos + 9 + stop + 3)
			continue
		}
		if len(pending) > 0 {
			top := &pending[len(pending)-1]
			endTag := "</" + top.tag + ">"
			if strings.HasPrefix(rest, endTag) {
				advance(pos + len(endTag))
				pending = pending[:len(pending)-1]
				continue
			}
			if top.raw {
				if rest[0] == '`' || rest[0] == '~' {
					candidate := t.matchCodeLiteral(pos, top.bodyStart, max(top.bodyStart, lineStart), endTag, len(t.s))
					if candidate > pos {
						if top.ownClose == 0 {
							top.ownClose = firstBodyClose(t.s, pos, len(t.s), endTag)
						}
						if top.ownClose < 0 || candidate > top.ownClose {
							return false
						}
						// Only scan non-crossing candidates. Expanding CDATA
						// before this check would rescan entire later-body chains.
						candidate = literalConsumedEnd(t.s, pos, candidate)
						if candidate > top.ownClose || candidate > end {
							return false
						}
						advance(candidate)
						continue
					}
				}
				if !top.group && (top.wrapper == "" || !openingTagPrefix(rest, top.wrapper)) {
					advance(pos + 1)
					continue
				}
			}
		}
		if rest[0] != '<' {
			advance(pos + 1)
			continue
		}
		nameEnd := pos + 1
		for nameEnd < len(t.s) && isNameByte(t.s[nameEnd]) {
			nameEnd++
		}
		if nameEnd == pos+1 {
			pos++
			continue
		}
		name := t.s[pos+1 : nameEnd]
		frame, known := literalCommand(t.grammar, name)
		textTag := false
		if len(pending) > 0 {
			top := pending[len(pending)-1]
			if !top.group {
				frame, known = literalFrame{tag: name, raw: !top.raw}, true
			} else if top.raw && !strings.HasPrefix(name, t.grammar.Prefix) {
				// The rejected-group reader treats non-command tags as text,
				// not raw elements whose distant close could rescan a suffix.
				textTag, known = true, false
			} else if !known {
				frame, known = literalFrame{tag: name, raw: true}, true
			}
		} else if slices.Contains(fields.tags, name) {
			frame, known = literalFrame{tag: name, raw: true}, true
		}
		limit := end
		if !known {
			limit = min(limit, pos+256)
		}
		stop, selfClosing, complete := literalTagEnd(t.s, nameEnd, limit)
		if !complete {
			if known {
				return false
			}
			pos = nameEnd
			continue
		}
		if !selfClosing && !textTag {
			frame.bodyStart = stop
			if frame.group {
				// ParseOrdered reads text before a group's first command as
				// a rejected raw body, with the same literal matcher. Such a
				// body can originate a crossing walk just like a leaf/field.
				next := stop
				for next < len(t.s) && isSpace(t.s[next]) {
					next++
				}
				if next < len(t.s) {
					rest := t.s[next:]
					frame.raw = t.ordered && (rest[0] != '<' || !strings.HasPrefix(rest[1:], t.grammar.Prefix) && !strings.HasPrefix(rest, "</"))
					frame.bodyStart = next
				}
			}
			pending = append(pending, frame)
		}
		advance(stop)
	}
	// Text between container fields is invalid in the fallback reading;
	// text in a suspended raw panel body is not. Retain valid operand-local
	// literals without mistaking mixed panel payloads for invalid syntax.
	return len(pending) == 0 || !pending[len(pending)-1].raw && !pending[len(pending)-1].group
}

// Match the body reader's CDATA-before-literal dispatch, including a span
// whose closing delimiter is inside a CDATA section. Never resume inside it.
func literalConsumedEnd(s string, at, end int) int {
	for at < end {
		next := strings.Index(s[at:end], "<![CDATA[")
		if next < 0 {
			break
		}
		at += next + 9
		stop := strings.Index(s[at:], "]]>")
		if stop < 0 {
			return len(s)
		}
		at += stop + 3
		end = max(end, at)
	}
	return end
}

type literalFrame struct {
	tag                 string
	raw, group          bool
	wrapper             string
	bodyStart, ownClose int
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
	// readOpen's syntax without attribute allocation or diagnostic line scans.
	pos := afterName
	for pos < limit {
		for pos < limit && isSpace(s[pos]) {
			pos++
		}
		if pos == limit {
			break
		}
		if s[pos] == '>' {
			return pos + 1, false, true
		}
		if s[pos] == '/' {
			if pos+1 < limit && s[pos+1] == '>' {
				return pos + 2, true, true
			}
			break
		}
		start := pos
		for pos < limit && isNameByte(s[pos]) {
			pos++
		}
		if pos == start {
			break
		}
		for pos < limit && isSpace(s[pos]) {
			pos++
		}
		if pos == limit || s[pos] != '=' {
			break
		}
		pos++
		for pos < limit && isSpace(s[pos]) {
			pos++
		}
		if pos == limit || s[pos] != '\'' && s[pos] != '"' {
			break
		}
		quote := s[pos]
		stop := strings.IndexByte(s[pos+1:limit], quote)
		if stop < 0 {
			break
		}
		pos += stop + 2
	}
	return pos, false, false
}

// Locate the fallback context of a plain enclosing close in a wrapper body.
// A panel close inside an operand does not end the panel in that reading.
// Scan only the candidate prefix, never the panel's already-consumed prefix;
// candidates are disjoint on acceptance and bounded by first on rejection.
func (t *tokenizer) wrapperBoundaryFrames(at, first, bodyStart int, closing, wrapper string) ([]literalFrame, bool) {
	name := strings.TrimSuffix(strings.TrimPrefix(closing, "</"), ">")
	pending := []literalFrame{{tag: name, raw: true, wrapper: wrapper, bodyStart: bodyStart}}
	for pos := at; pos < first; {
		rest := t.s[pos:]
		if strings.HasPrefix(rest, "<![CDATA[") {
			stop := strings.Index(rest[9:], "]]>")
			if stop < 0 || pos+9+stop+3 > first {
				return nil, false
			}
			pos += 9 + stop + 3
			continue
		}
		top := pending[len(pending)-1]
		endTag := "</" + top.tag + ">"
		if strings.HasPrefix(rest, endTag) {
			pos += len(endTag)
			pending = pending[:len(pending)-1]
			continue
		}
		if rest[0] != '<' || top.raw && (top.wrapper == "" || !openingTagPrefix(rest, top.wrapper)) {
			pos++
			continue
		}
		nameEnd := pos + 1
		for nameEnd < first && isNameByte(t.s[nameEnd]) {
			nameEnd++
		}
		if nameEnd == pos+1 {
			pos++
			continue
		}
		stop, selfClosing, complete := literalTagEnd(t.s, nameEnd, first)
		if !complete {
			return nil, false
		}
		if !selfClosing {
			pending = append(pending, literalFrame{tag: t.s[pos+1 : nameEnd], raw: !top.raw, bodyStart: stop})
		}
		pos = stop
	}
	if pending[len(pending)-1].tag == name {
		pending = pending[:len(pending)-1]
	}
	return pending, true
}

func (t *tokenizer) codeLiteralEnd(at, bodyStart, lineStart int, closing string, fields bodyFieldRules, limit *int) int {
	end := t.matchCodeLiteral(at, bodyStart, lineStart, closing, *limit)
	if end <= at || end > *limit {
		return at
	}
	first := firstBodyClose(t.s, at, end, closing)
	if first >= 0 {
		if fields.wrapper != "" {
			enclosing, ok := t.wrapperBoundaryFrames(at, first, bodyStart, closing, fields.wrapper)
			if !ok {
				*limit = min(*limit, first)
				return at
			}
			fields.enclosing = enclosing
		}
		if !t.codeBodyBoundary(first, end, closing, fields) {
			*limit = min(*limit, first)
			return at
		}
	}
	return end
}

// Candidate matching is shared by the body reader and fallback walk. The
// lazily built index contains positions only, never boundary-policy decisions.
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
