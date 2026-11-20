package wsdoc

import (
	"bytes"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// termStat is one term's lossless per-channel aggregate in a ticket: w holds
// the weight sums and n the occurrence counts over the channels 0 title,
// 1 heading, 2 code, 3 prose.
type termStat struct {
	w [4]float64
	n [4]int32
}

// mapHeading is one H2-or-deeper body heading, outside fenced code.
type mapHeading struct {
	Level int
	Text  string
}

// ticketDoc is a ticket's feature document for the ticket map. It is
// immutable after buildTicketDoc returns and shared by pointer between the
// cache and every build.
type ticketDoc struct {
	Title    string
	Lines    int
	Headings []mapHeading
	Preview  string
	Mentions []string
	Terms    map[string]*termStat
	// ODQ reports an `## Open Decision Queue` heading outside fenced code
	// (containsOpenDecisionQueue).
	ODQ bool
}

// Term channels, indexing termStat.w and termStat.n.
const (
	tmChTitle   = 0
	tmChHeading = 1
	tmChCode    = 2
	tmChProse   = 3
)

// Channel weights and section scales. Headings carry no weight of their own:
// the pipeline names three weighted channels (title, code, prose), so a
// heading weighs as prose, scaled by the section it opens.
const (
	tmWTitle     = 2.4
	tmWCode      = 3.0
	tmWProse     = 1.0
	tmWHeading   = tmWProse
	tmWRelNote   = 0.6
	tmScaleBoost = 1.4
	tmScaleDamp  = 0.5
)

const (
	tmFeatureCap    = 256 << 10 // body bytes read for features
	tmPreviewOpen   = 1500      // preview runes for an open ticket
	tmPreviewClosed = 700       // preview runes for a closed ref
	tmCodeSpanMax   = 160       // longest backtick span read as code
	tmCodePieceMax  = 34        // longest code token kept
	tmSlabChunk     = 128       // termStats allocated per slab chunk
)

// buildTicketDoc parses one ticket's full text into its feature document.
// closed selects the closed-ref preview budget (700 runes instead of 1500).
func buildTicketDoc(stem, text string, closed bool) *ticketDoc {
	doc := &ticketDoc{
		Lines: strings.Count(text, "\n") + 1,
		ODQ:   containsOpenDecisionQueue(text),
	}
	// Invalid UTF-8 bytes are skipped (dropped) once here, so every feature,
	// the title and the preview see valid text only.
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "")
	}
	fmEnd := tmFrontmatterEnd(text)
	var fm map[string]any
	if fmEnd > 0 {
		fm = frontmatterFromText(text[:fmEnd])
	}
	body := text[fmEnd:]
	if len(body) > tmFeatureCap {
		cut := tmFeatureCap
		for cut > 0 && !utf8.RuneStart(body[cut]) {
			cut--
		}
		body = body[:cut]
	}

	b := &tmBuilder{stem: stem, terms: make(map[string]*termStat, 256)}
	b.pv.limit = tmPreviewOpen
	if closed {
		b.pv.limit = tmPreviewClosed
	}

	title := ""
	if s, ok := fm["title"].(string); ok {
		title = strings.TrimSpace(s)
	}
	b.parseBody(body)
	if title == "" {
		title = b.h1
	}
	if title == "" {
		title = stem
	}
	doc.Title = strings.Clone(title)

	// Channel 0: the stem core plus the title.
	core := stem
	if n := tmStemCategoryPrefix(stem); n > 0 {
		core = stem[n:]
	}
	b.addText(core, tmChTitle, tmWTitle)
	b.addText(title, tmChTitle, tmWTitle)

	// Frontmatter relation notes, in stem order so float sums are stable.
	if related := relatedEntries(fm["related"]); len(related) > 0 {
		keys := make([]string, 0, len(related))
		for key := range related {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if note := related[key]; note != "" {
				b.addText(note, tmChProse, tmWRelNote)
			}
		}
	}

	doc.Headings = b.headings
	doc.Preview = b.pv.result()
	sort.Strings(b.mentions)
	doc.Mentions = b.mentions
	doc.Terms = b.terms
	return doc
}

// tmFrontmatterEnd returns the byte offset just past the closing fence line
// of a leading frontmatter block, or 0 when there is none. It finds the same
// closing line frontmatterFromText does, so parsing text[:end] matches parsing
// the whole text without splitting the whole body.
func tmFrontmatterEnd(text string) int {
	if !strings.HasPrefix(text, "---") {
		return 0
	}
	nl := strings.IndexByte(text, '\n')
	if nl < 0 {
		return 0
	}
	for pos := nl + 1; pos < len(text); {
		line, next := text[pos:], len(text)
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line, next = line[:i], pos+i+1
		}
		if strings.TrimSpace(line) == "---" {
			return next
		}
		pos = next
	}
	return 0
}

// tmStemCategoryPrefix returns the length of a leading `\d{6}-[a-z]+-`, or 0.
func tmStemCategoryPrefix(stem string) int {
	if len(stem) < 9 {
		return 0
	}
	for i := 0; i < 6; i++ {
		if !tmDigit(stem[i]) {
			return 0
		}
	}
	if stem[6] != '-' {
		return 0
	}
	j := 7
	for j < len(stem) && stem[j] >= 'a' && stem[j] <= 'z' {
		j++
	}
	if j == 7 || j >= len(stem) || stem[j] != '-' {
		return 0
	}
	return j + 1
}

// tmBuilder accumulates one ticket's features. Its scratch buffers are reused
// across lines so tokenizing allocates only for new term keys.
type tmBuilder struct {
	stem     string
	terms    map[string]*termStat
	slab     []termStat
	headings []mapHeading
	mentions []string
	h1       string
	pv       tmPreview

	lower    []byte // lowercased token scratch
	textBuf  []byte // prose scratch
	outBuf   []byte // line with backtick spans blanked, for bare paths
	spanBuf  []byte // backtick span with call arguments dropped
	pathBuf  []byte // lowercased path scratch
	runeOffs []int  // CJK run rune offsets
}

type tmStackEntry struct {
	level int
	scale float64
}

// parseBody walks the body line by line: fence-aware headings, sections,
// mentions, the preview, and the heading, code and prose channels.
func (b *tmBuilder) parseBody(body string) {
	var fenceChar byte
	fenceLen := 0
	scale := 1.0
	var stack [8]tmStackEntry
	depth := 0
	secOpen, secName, secStart := false, "", 0

	for pos := 0; pos < len(body); {
		lineStart := pos
		raw, next := body[pos:], len(body)
		if i := strings.IndexByte(raw, '\n'); i >= 0 {
			raw, next = raw[:i], pos+i+1
		}
		pos = next
		b.scanMentions(raw)
		line := strings.TrimRight(raw, " \t\r")
		if char, n, info := markdownFence(line); n > 0 {
			if fenceLen == 0 {
				fenceChar, fenceLen = char, n
			} else if char == fenceChar && n >= fenceLen && info == "" {
				fenceLen = 0
			}
			continue
		}
		if fenceLen > 0 {
			// Fenced content feeds only the path extractors: a path named in a
			// code block is a strong code signal, but its keywords and syntax
			// are not prose.
			out := append(b.outBuf[:0], raw...)
			tmBlankURLs(out)
			b.pathsIn(out, tmWCode*scale)
			b.outBuf = out
			continue
		}
		if level, htext := tmATXHeading(line); level > 0 {
			b.flushSection(body, secOpen, secName, secStart, lineStart)
			if level == 1 {
				// The H1 is the title's fallback, not a section: it is neither
				// listed, previewed nor tokenized (the title channel carries it).
				if b.h1 == "" {
					b.h1 = htext
				}
				depth, scale, secOpen = 0, 1.0, false
				continue
			}
			for depth > 0 && stack[depth-1].level >= level {
				depth--
			}
			// A section that matches neither scale regex inherits its nearest
			// enclosing section's scale, so subsections of a damped or boosted
			// section stay damped or boosted.
			scale = tmSectionScale(htext)
			if scale == 1.0 && depth > 0 {
				scale = stack[depth-1].scale
			}
			if depth < len(stack) {
				stack[depth] = tmStackEntry{level: level, scale: scale}
				depth++
			}
			b.headings = append(b.headings, mapHeading{Level: level, Text: strings.Clone(htext)})
			b.addText(tmStripHeading(htext), tmChHeading, tmWHeading*scale)
			secOpen, secName, secStart = true, htext, next
			continue
		}
		b.codeLine(raw, tmWCode*scale)
		b.addText(raw, tmChProse, tmWProse*scale)
	}
	b.flushSection(body, secOpen, secName, secStart, len(body))
}

// tmATXHeading reports an ATX heading line's level (1-6) and its text with
// any closing `#` sequence removed; level is 0 for a non-heading line.
func tmATXHeading(line string) (int, string) {
	indent := 0
	for indent < len(line) && indent < 4 && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return 0, ""
	}
	rest := line[indent:]
	level := 0
	for level < len(rest) && rest[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, ""
	}
	if level < len(rest) && rest[level] != ' ' && rest[level] != '\t' {
		return 0, ""
	}
	text := strings.TrimSpace(rest[level:])
	if t := strings.TrimRight(text, "#"); t == "" || t[len(t)-1] == ' ' || t[len(t)-1] == '\t' {
		text = strings.TrimSpace(t)
	}
	return level, text
}

var (
	tmBoostSections = tmWordSet(`context goal goals scope problem background motivation summary overview why`)
	tmDampSections  = tmWordSet(`verification constraint constraints result results resolution note notes
		acceptance test tests validation history log`)
)

// tmSectionScale applies the section regexes
// `^(context|goal|…|why)\b` (×1.4) and `^(verification|…|log)\b` (×0.5),
// case-insensitively, to a raw heading text.
func tmSectionScale(heading string) float64 {
	n := 0
	for n < len(heading) && tmWordByte(heading[n]) {
		n++
	}
	if n == 0 {
		return 1.0
	}
	word := strings.ToLower(heading[:n])
	if _, ok := tmBoostSections[word]; ok {
		return tmScaleBoost
	}
	if _, ok := tmDampSections[word]; ok {
		return tmScaleDamp
	}
	return 1.0
}

// tmStripHeading removes a leading `Phase N`, `Result`, `(<hash>)`,
// `- YYYY-MM-DD` and `:`, repeatedly, from a heading before tokenizing it.
func tmStripHeading(h string) string {
	for {
		h = strings.TrimLeft(h, " \t")
		next := h
		switch {
		case strings.HasPrefix(h, ":"):
			next = h[1:]
		case strings.HasPrefix(h, "Phase "):
			r := strings.TrimLeft(h[len("Phase "):], " ")
			d := 0
			for d < len(r) && tmDigit(r[d]) {
				d++
			}
			if d > 0 {
				next = r[d:]
			}
		case strings.HasPrefix(h, "Result") && (len(h) == 6 || !tmWordByte(h[6])):
			next = h[6:]
		case strings.HasPrefix(h, "("):
			if c := strings.IndexByte(h, ')'); c > 1 && tmIsHex(h[1:c]) {
				next = h[c+1:]
			}
		case strings.HasPrefix(h, "-"):
			r := strings.TrimLeft(h[1:], " ")
			if tmIsDate(r) {
				next = r[10:]
			}
		}
		if next == h {
			return h
		}
		h = next
	}
}

func tmIsHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !tmDigit(c) && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// tmIsDate reports a leading YYYY-MM-DD.
func tmIsDate(s string) bool {
	if len(s) < 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for _, i := range [...]int{0, 1, 2, 3, 5, 6, 8, 9} {
		if !tmDigit(s[i]) {
			return false
		}
	}
	return true
}

// flushSection renders a finished H2-or-deeper section into the preview.
func (b *tmBuilder) flushSection(body string, open bool, name string, start, end int) {
	if !open || b.pv.full {
		return
	}
	if start > end {
		start = end
	}
	text := tmTrimBlankLines(body[start:end])
	if b.pv.runes > 0 {
		b.pv.write("\n\n")
	}
	b.pv.write("## ")
	b.pv.write(name)
	if text != "" {
		b.pv.write("\n")
		b.pv.write(text)
	}
}

// tmTrimBlankLines drops leading blank lines and trailing whitespace.
func tmTrimBlankLines(s string) string {
	s = strings.TrimRight(s, " \t\r\n")
	for s != "" {
		i := strings.IndexByte(s, '\n')
		if i < 0 || strings.TrimSpace(s[:i]) != "" {
			break
		}
		s = s[i+1:]
	}
	return s
}

// tmPreview accumulates at most limit+1 runes: enough to know whether the
// preview needs truncating without copying a long body.
type tmPreview struct {
	sb    strings.Builder
	runes int
	limit int
	full  bool
}

func (p *tmPreview) write(s string) {
	if p.full {
		return
	}
	need := p.limit + 1 - p.runes
	i, n := 0, 0
	for i < len(s) && n < need {
		if s[i] < utf8.RuneSelf {
			i++
		} else {
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
		n++
	}
	p.sb.WriteString(s[:i])
	p.runes += n
	if p.runes > p.limit {
		p.full = true
	}
}

// result returns the preview, truncated to limit runes in total (the `\n…`
// marker included) when it overflowed.
func (p *tmPreview) result() string {
	s := p.sb.String()
	if !p.full {
		return s
	}
	keep := p.limit - 2
	i := 0
	for n := 0; n < keep && i < len(s); n++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return strings.TrimRight(s[:i], " \t\r\n") + "\n…"
}

// scanMentions records every ticket stem in a body line except the ticket's
// own, deduplicated; the caller sorts.
func (b *tmBuilder) scanMentions(line string) {
	for i := 0; i < len(line); i++ {
		if !tmDigit(line[i]) {
			continue
		}
		e := tmStemEnd(line, i)
		if e < 0 {
			continue
		}
		if s := line[i:e]; s != b.stem && !tmContains(b.mentions, s) {
			b.mentions = append(b.mentions, strings.Clone(s))
		}
		i = e - 1
	}
}

func tmContains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// tmStemEnd returns the end of a ticket stem
// `\d{6}-[a-z]+-[a-z0-9][a-z0-9-]*` (trailing dashes trimmed) starting at i
// and not preceded by a word byte, or -1.
func tmStemEnd[T string | []byte](s T, i int) int {
	if i > 0 && tmWordByte(s[i-1]) {
		return -1
	}
	if i+9 > len(s) {
		return -1
	}
	for k := 0; k < 6; k++ {
		if !tmDigit(s[i+k]) {
			return -1
		}
	}
	j := i + 6
	if s[j] != '-' {
		return -1
	}
	j++
	a := j
	for j < len(s) && s[j] >= 'a' && s[j] <= 'z' {
		j++
	}
	if j == a || j >= len(s) || s[j] != '-' {
		return -1
	}
	j++
	if j >= len(s) || !tmLowerOrDigit(s[j]) {
		return -1
	}
	for j < len(s) && (tmLowerOrDigit(s[j]) || s[j] == '-') {
		j++
	}
	for s[j-1] == '-' {
		j--
	}
	return j
}

func tmHasStem[T string | []byte](s T) bool {
	for i := 0; i < len(s); i++ {
		if tmDigit(s[i]) && tmStemEnd(s, i) >= 0 {
			return true
		}
	}
	return false
}

// tmBlankStems overwrites every ticket stem in p with spaces.
func tmBlankStems(p []byte) {
	for i := 0; i < len(p); i++ {
		if !tmDigit(p[i]) {
			continue
		}
		if e := tmStemEnd(p, i); e >= 0 {
			tmBlank(p[i:e])
			i = e - 1
		}
	}
}

// tmBlankURLs overwrites `scheme://…` and `www.…` runs in p with spaces.
func tmBlankURLs(p []byte) {
	for i := 0; i < len(p); i++ {
		start := -1
		switch {
		case p[i] == ':' && i+2 < len(p) && p[i+1] == '/' && p[i+2] == '/':
			start = i
			for start > 0 && tmSchemeByte(p[start-1]) {
				start--
			}
		case (p[i] == 'w' || p[i] == 'W') && i+4 <= len(p) && (i == 0 || !tmWordByte(p[i-1])) &&
			bytes.EqualFold(p[i:i+4], []byte("www.")):
			start = i
		default:
			continue
		}
		end := i
		for end < len(p) && !tmURLStop(p[end]) {
			end++
		}
		tmBlank(p[start:end])
		i = end
	}
}

func tmURLStop(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', '<', '>', '"', '\'', '`', ')', ']':
		return true
	}
	return false
}

func tmSchemeByte(c byte) bool {
	return tmAlnum(c) || c == '+' || c == '.' || c == '-'
}

func tmBlank(p []byte) {
	for i := range p {
		p[i] = ' '
	}
}

// addText runs the prose cleanup (stems and URLs removed; backticks and other
// punctuation separate words) and the word tokenizer over s.
func (b *tmBuilder) addText(s string, ch int, w float64) {
	if s == "" {
		return
	}
	p := append(b.textBuf[:0], s...)
	tmBlankStems(p)
	tmBlankURLs(p)
	b.words(p, ch, w)
	b.textBuf = p
}

// codeLine extracts the code channel from one section line: backtick spans,
// then bare file paths and slash paths in the text outside them.
func (b *tmBuilder) codeLine(line string, w float64) {
	out := append(b.outBuf[:0], line...)
	for i := 0; i < len(line); {
		o := strings.IndexByte(line[i:], '`')
		if o < 0 {
			break
		}
		o += i
		c := strings.IndexByte(line[o+1:], '`')
		if c < 0 {
			break
		}
		c += o + 1
		if span := line[o+1 : c]; span != "" && len(span) <= tmCodeSpanMax {
			b.codeSpan(span, w)
		}
		tmBlank(out[o : c+1])
		i = c + 1
	}
	tmBlankURLs(out)
	b.pathsIn(out, w)
	b.outBuf = out
}

// codeSpan reads one backtick span: a stem or URL span is rejected whole;
// call arguments are dropped; the rest splits into pieces.
func (b *tmBuilder) codeSpan(span string, w float64) {
	if strings.Contains(span, "://") || tmHasPrefixFold(span, "www.") || tmHasStem(span) {
		return
	}
	sb := b.spanBuf[:0]
	depth := 0
	for i := 0; i < len(span); i++ {
		c := span[i]
		if depth > 0 {
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
			}
			continue
		}
		if c == '(' && i > 0 && tmWordByte(span[i-1]) {
			depth = 1
			sb = append(sb, ' ')
			continue
		}
		sb = append(sb, c)
	}
	for i := 0; i < len(sb); {
		if !tmCodePieceByte(sb[i]) {
			i++
			continue
		}
		j := i
		for j < len(sb) && tmCodePieceByte(sb[j]) {
			j++
		}
		prev := byte(' ')
		if i > 0 {
			prev = sb[i-1]
		}
		b.codePiece(sb[i:j], prev, w)
		i = j
	}
	b.spanBuf = sb
}

func tmHasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func tmCodePieceByte(c byte) bool {
	return tmWordByte(c) || c == '.' || c == '/' || c == '#' || c == ':' || c == '-'
}

// codePiece classifies one span piece: a path, an `#id` or `.class` marker, a
// file name, a dotted identifier, or a plain identifier.
func (b *tmBuilder) codePiece(p []byte, prev byte, w float64) {
	// A `#` after the first byte starts a fragment (`file.go#L10`), not a
	// marker; `:<line>` suffixes go too.
	if len(p) > 1 {
		if k := bytes.IndexByte(p[1:], '#'); k >= 0 {
			p = p[:k+1]
		}
	}
	p = tmStripLineSuffix(p)
	p = bytes.TrimRight(p, ".-")
	p = bytes.TrimLeft(p, "-")
	if prev == ')' || prev == ']' {
		// `(*T).Method`: the dot continues an expression, not a class.
		p = bytes.TrimLeft(p, ".")
	}
	if len(p) == 0 || bytes.IndexByte(p, ':') >= 0 {
		return
	}
	tmLowerInPlace(p)
	switch {
	case bytes.IndexByte(p, '/') >= 0:
		b.emitPath(p, w)
	case p[0] == '#':
		if len(p) > 1 && tmLetter(p[1]) {
			b.addCode(p, w)
		}
	case p[0] == '.':
		if len(p) > 1 && tmLetter(p[1]) {
			b.addCode(p, w)
		}
	default:
		if dot := bytes.LastIndexByte(p, '.'); dot > 0 && tmIsPathExt(p[dot+1:]) {
			b.addCode(p, w)
			b.addCode(p[:dot], w)
			return
		}
		b.addCode(p, w) // a dotted identifier stays whole
	}
}

// tmStripLineSuffix strips trailing `:<n>`, `:<n>-<m>` suffixes.
func tmStripLineSuffix(p []byte) []byte {
	for {
		k := len(p)
		for k > 0 && tmDigit(p[k-1]) {
			k--
		}
		if k == len(p) || k == 0 {
			return p
		}
		if p[k-1] == ':' {
			p = p[:k-1]
			continue
		}
		if p[k-1] == '-' {
			m := k - 1
			for m > 0 && tmDigit(p[m-1]) {
				m--
			}
			if m < k-1 && m > 0 && p[m-1] == ':' {
				p = p[:m-1]
				continue
			}
		}
		return p
	}
}

// pathsIn scans text outside backticks for path-ish runs ([\w./-]) and reads
// bare file paths and slash paths from them.
func (b *tmBuilder) pathsIn(out []byte, w float64) {
	for i := 0; i < len(out); {
		if !tmPathByte(out[i]) {
			i++
			continue
		}
		j := i
		interesting := false
		for j < len(out) && tmPathByte(out[j]) {
			if out[j] == '/' || (out[j] == '.' && j+1 < len(out) && tmAlnum(out[j+1])) {
				interesting = true
			}
			j++
		}
		if interesting {
			b.pathRun(out[i:j], w)
		}
		i = j
	}
}

func tmPathByte(c byte) bool {
	return tmWordByte(c) || c == '.' || c == '/' || c == '-'
}

// pathRun applies, within one maximal [\w./-] run, the bare file path rule
//
//	(?<![\w/.])((?:[\w.-]+/)*[\w-]+\.(go|js|…|png))(?!\w)
//
// with both guards emulated by hand (the run's own boundaries satisfy them,
// so inside a run a match may start only at the run start or after `-`), and
// then the slash path rule `[a-z][\w-]*(?:/[\w.-]+)+` for matches that carry
// an extension or are not a bare word/word and that overlap no bare match.
func (b *tmBuilder) pathRun(run []byte, w float64) {
	var bareBuf [8][2]int
	bare := bareBuf[:0]
	for s := 0; s < len(run); s++ {
		if s > 0 && run[s-1] != '-' {
			continue
		}
		if e := tmMatchBarePath(run, s); e > 0 {
			bare = append(bare, [2]int{s, e})
			b.emitPath(run[s:e], w)
			s = e - 1
		}
	}
	// The slash rule has no lookbehind in the ticket; a start inside a word
	// (`Foo/bar/baz` matching `oo/bar/baz`) is refused here as noise.
	for s := 0; s < len(run); s++ {
		if (s > 0 && tmWordByte(run[s-1])) || run[s] < 'a' || run[s] > 'z' {
			continue
		}
		e, comps := tmMatchSlashPath(run, s)
		if e == 0 {
			continue
		}
		overlaps := false
		for _, m := range bare {
			if s < m[1] && m[0] < e {
				overlaps = true
				break
			}
		}
		if !overlaps {
			end := e
			for end > s && run[end-1] == '.' {
				end--
			}
			last := bytes.LastIndexByte(run[s:end], '/') + s + 1
			ext := false
			if dot := bytes.LastIndexByte(run[last:end], '.'); dot >= 0 && last+dot+1 < end {
				ext = true
			}
			if ext || comps > 2 {
				b.emitPath(run[s:end], w)
			}
		}
		s = e - 1
	}
}

// tmMatchBarePath returns the end of a bare file path match anchored at s, or
// 0. It prefers the most directory components, like the regex's greedy
// `(?:[\w.-]+/)*`, falling back to fewer when the basename does not match.
func tmMatchBarePath(run []byte, s int) int {
	var starts [32]int
	n := 1
	starts[0] = s
	for i := s; i < len(run) && n < len(starts); i++ {
		if run[i] == '/' {
			if i == starts[n-1] {
				break // an empty component ends the directory chain
			}
			starts[n] = i + 1
			n++
		}
	}
	for k := n - 1; k >= 0; k-- {
		p := starts[k]
		q := p
		for q < len(run) && (tmWordByte(run[q]) || run[q] == '-') {
			q++
		}
		if q == p || q >= len(run) || run[q] != '.' {
			continue
		}
		for _, ext := range tmPathExts {
			e := q + 1 + len(ext)
			if e <= len(run) && string(run[q+1:e]) == ext && (e == len(run) || !tmWordByte(run[e])) {
				return e
			}
		}
	}
	return 0
}

// tmMatchSlashPath returns the end and component count of a slash path match
// anchored at s, or 0.
func tmMatchSlashPath(run []byte, s int) (int, int) {
	i := s + 1
	for i < len(run) && (tmWordByte(run[i]) || run[i] == '-') {
		i++
	}
	end, comps := 0, 1
	for i < len(run) && run[i] == '/' {
		j := i + 1
		for j < len(run) && run[j] != '/' {
			j++
		}
		if j == i+1 {
			break
		}
		end, i = j, j
		comps++
	}
	return end, comps
}

var tmPathExts = []string{
	"go", "js", "mjs", "ts", "tsx", "jsx", "py", "md", "css", "scss", "html", "json", "rs",
	"toml", "yaml", "yml", "sh", "ps1", "sql", "txt", "lock", "mod", "sum", "svg", "png",
}

func tmIsPathExt(ext []byte) bool {
	for _, e := range tmPathExts {
		if string(ext) == e {
			return true
		}
	}
	return false
}

// emitPath emits a path's lowercased segments plus its basename with and
// without the extension. A path naming a ticket stem is rejected whole.
func (b *tmBuilder) emitPath(p []byte, w float64) {
	q := append(b.pathBuf[:0], p...)
	b.pathBuf = q
	tmLowerInPlace(q)
	if tmHasStem(q) {
		return
	}
	last := len(q)
	for last > 0 && q[last-1] == '/' {
		last--
	}
	for i := 0; i < last; {
		j := i
		for j < last && q[j] != '/' {
			j++
		}
		seg := bytes.TrimLeft(q[i:j], ".") // `.hidden` dirs, `./`, `../`
		i = j + 1
		if len(seg) == 0 {
			continue
		}
		b.addCode(seg, w)
		if j == last {
			if dot := bytes.LastIndexByte(seg, '.'); dot > 0 {
				b.addCode(seg[:dot], w)
			}
		}
	}
}

// addCode filters and records one code token: 2-34 bytes, no `:`, not a hex
// hash, digits under half its alphanumerics, and a pure word not a stopword.
func (b *tmBuilder) addCode(tok []byte, w float64) {
	if len(tok) < 2 || len(tok) > tmCodePieceMax || bytes.IndexByte(tok, ':') >= 0 {
		return
	}
	core := bytes.TrimLeft(tok, "#.")
	digits, alnum := 0, 0
	hex, alpha := true, true
	for _, c := range core {
		switch {
		case tmDigit(c):
			digits++
			alnum++
			alpha = false
		case c >= 'a' && c <= 'z':
			alnum++
			if c > 'f' {
				hex = false
			}
		default:
			hex, alpha = false, false
		}
	}
	if alnum == 0 || digits*2 >= alnum || (hex && digits > 0) {
		return
	}
	if alpha && len(core) == len(tok) {
		if _, stop := enStop[string(tok)]; stop {
			return
		}
	}
	b.addTerm(tok, tmChCode, w)
}

// words is the word tokenizer: ASCII alphanumeric runs (camelCase split,
// lowercased, noise and stopwords dropped), Hangul runs (particle stripped),
// and Han, Hiragana and Katakana runs (overlapping rune bigrams). Every other
// rune, and any invalid byte, separates tokens.
func (b *tmBuilder) words(p []byte, ch int, w float64) {
	for i := 0; i < len(p); {
		c := p[i]
		if c < utf8.RuneSelf {
			if tmAlnum(c) {
				j := i + 1
				for j < len(p) && p[j] < utf8.RuneSelf && tmAlnum(p[j]) {
					j++
				}
				b.latinRun(p[i:j], ch, w)
				i = j
				continue
			}
			i++
			continue
		}
		r, size := utf8.DecodeRune(p[i:])
		cls := tmScriptOf(r)
		if cls == tmScriptOther {
			i += size
			continue
		}
		j := i + size
		for j < len(p) {
			r2, s2 := utf8.DecodeRune(p[j:])
			if tmScriptOf(r2) != cls {
				break
			}
			j += s2
		}
		if cls == tmScriptHangul {
			b.hangulRun(p[i:j], ch, w)
		} else {
			b.cjkRun(p[i:j], ch, w)
		}
		i = j
	}
}

const (
	tmScriptOther = iota
	tmScriptHangul
	tmScriptHan
	tmScriptHiragana
	tmScriptKatakana
)

func tmScriptOf(r rune) int {
	switch {
	case r >= 0xAC00 && r <= 0xD7A3:
		return tmScriptHangul
	case r < 0x2E80 || r == utf8.RuneError:
		return tmScriptOther
	case r == 0x30FC: // the prolonged sound mark is Common, but continues Katakana words
		return tmScriptKatakana
	case unicode.Is(unicode.Han, r):
		return tmScriptHan
	case unicode.Is(unicode.Hiragana, r):
		return tmScriptHiragana
	case unicode.Is(unicode.Katakana, r):
		return tmScriptKatakana
	}
	return tmScriptOther
}

// latinRun splits an ASCII alphanumeric run at camelCase boundaries
// (`fooBar`, `utf8Decode`, `HTTPRequest`) and records each piece.
func (b *tmBuilder) latinRun(run []byte, ch int, w float64) {
	start := 0
	for i := 1; i < len(run); i++ {
		if !tmUpper(run[i]) {
			continue
		}
		prev := run[i-1]
		if tmLowerOrDigit(prev) || (tmUpper(prev) && i+1 < len(run) && run[i+1] >= 'a' && run[i+1] <= 'z') {
			b.latinPiece(run[start:i], ch, w)
			start = i
		}
	}
	b.latinPiece(run[start:], ch, w)
}

// latinPiece reads `[A-Za-z][A-Za-z0-9]*` from a piece, lowercases it, and
// drops noise (under 2 bytes, digits at least half, a hex hash with a digit)
// and stopwords.
func (b *tmBuilder) latinPiece(piece []byte, ch int, w float64) {
	k := 0
	for k < len(piece) && !tmLetter(piece[k]) {
		k++
	}
	piece = piece[k:]
	if len(piece) < 2 {
		return
	}
	buf := b.lower[:0]
	digits := 0
	hex := true
	for _, c := range piece {
		if tmUpper(c) {
			c += 'a' - 'A'
		}
		if tmDigit(c) {
			digits++
		} else if c > 'f' {
			hex = false
		}
		buf = append(buf, c)
	}
	b.lower = buf
	if digits*2 >= len(buf) || (hex && digits > 0) {
		return
	}
	if _, stop := enStop[string(buf)]; stop {
		return
	}
	b.addTerm(buf, ch, w)
}

// hangulRun strips the longest matching koSuffixes entry from a Hangul run
// when at least 2 syllables remain, then drops short runs and koStop words.
func (b *tmBuilder) hangulRun(run []byte, ch int, w float64) {
	const syl = 3 // every Hangul syllable is 3 bytes of UTF-8
	if len(run) < 2*syl {
		return
	}
	for _, sf := range koSuffixes {
		if len(sf.s) >= len(run) || !bytes.HasSuffix(run, []byte(sf.s)) {
			continue
		}
		stem := run[:len(run)-len(sf.s)]
		if !sf.fits(stem) {
			continue
		}
		if len(stem) >= 2*syl {
			run = stem
		}
		break
	}
	if _, stop := koStop[string(run)]; stop {
		return
	}
	b.addTerm(run, ch, w)
}

// cjkRun emits a Han, Hiragana or Katakana run's overlapping rune bigrams;
// a single-rune run emits nothing.
func (b *tmBuilder) cjkRun(run []byte, ch int, w float64) {
	offs := b.runeOffs[:0]
	for i := 0; i < len(run); {
		offs = append(offs, i)
		_, size := utf8.DecodeRune(run[i:])
		i += size
	}
	offs = append(offs, len(run))
	b.runeOffs = offs
	for k := 0; k+2 < len(offs); k++ {
		b.addTerm(run[offs[k]:offs[k+2]], ch, w)
	}
}

// addTerm adds one occurrence of tok on channel ch. Term stats come from
// fixed-size slab chunks so pointers stay stable without one allocation each.
func (b *tmBuilder) addTerm(tok []byte, ch int, w float64) {
	if st, ok := b.terms[string(tok)]; ok {
		st.w[ch] += w
		st.n[ch]++
		return
	}
	if len(b.slab) == cap(b.slab) {
		b.slab = make([]termStat, 0, tmSlabChunk)
	}
	b.slab = append(b.slab, termStat{})
	st := &b.slab[len(b.slab)-1]
	st.w[ch] = w
	st.n[ch] = 1
	b.terms[string(tok)] = st
}

func tmDigit(c byte) bool        { return c >= '0' && c <= '9' }
func tmUpper(c byte) bool        { return c >= 'A' && c <= 'Z' }
func tmLetter(c byte) bool       { return (c|0x20) >= 'a' && (c|0x20) <= 'z' }
func tmAlnum(c byte) bool        { return tmLetter(c) || tmDigit(c) }
func tmWordByte(c byte) bool     { return tmAlnum(c) || c == '_' }
func tmLowerOrDigit(c byte) bool { return (c >= 'a' && c <= 'z') || tmDigit(c) }

func tmLowerInPlace(p []byte) {
	for i, c := range p {
		if tmUpper(c) {
			p[i] = c + 'a' - 'A'
		}
	}
}

func tmWordSet(words string) map[string]struct{} {
	fields := strings.Fields(words)
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[f] = struct{}{}
	}
	return set
}

// enStop holds English function words plus ws ticket-convention vocabulary:
// status names, ticket categories, frontmatter keys, and the standard
// template section headings. It names no domain term.
var enStop = tmWordSet(`
	a about above across after afterwards again against all almost alone along already also
	although always am among amongst an and another any anyhow anyone anything anyway anywhere
	are aren around as at be became because become becomes been before beforehand behind being
	below beside besides between beyond both but by can cannot could couldn did didn do does
	doesn doing don done down during each eg either else elsewhere enough etc even ever every
	everyone everything everywhere except few for former formerly from further furthermore had
	hadn has hasn have haven having he hence her here hereby herein hers herself him himself his
	how however ie if in indeed instead into is isn it its itself just last latter least less
	let like ll may maybe me meanwhile might mine more moreover most mostly much must mustn my
	myself namely neither never nevertheless next no nobody none nor not nothing now nowhere of
	off often on once one only onto or other others otherwise ought our ours ourselves out over
	own per perhaps please quite rather re really same several shall shan she should shouldn
	since so some somehow someone something sometime sometimes somewhere still such than that
	the their theirs them themselves then thence there thereafter thereby therefore therein
	thereupon these they this those though through throughout thru thus to together too toward
	towards under unless until up upon us ve very via vs was wasn we well were weren what
	whatever when whence whenever where whereas whereby wherein whether which while whither who
	whoever whole whom whose why will with within without won would wouldn yet you your yours
	yourself yourselves

	ready todo idea ideas done dropped wip
	feat bug refactor chore research epic
	title related completed
	background decision decisions constraint constraints phase phases result results edition
	verification resolution prior art outcome ledger verified findings confirmed proposals
	questions rejected alternatives non child cross completion criteria deferred planned
	route facts blocked
`)

// koStop holds Korean function words (conjunctions, pronouns, adverbs, and
// two-syllable particle forms too short to strip) plus Korean renderings of
// the ticket-convention section headings. It names no domain term.
var koStop = tmWordSet(`
	그리고 그러나 하지만 그런데 그래서 따라서 그러면 그러므로 또는 또한 혹은 및 즉 단지 오직 바로
	이것 그것 저것 이거 그거 저거 여기 거기 저기 우리 저희 당신 이런 그런 저런 어떤 무엇 어떻게 언제
	어디 누구 모든 각각 해당 같은 다른 위해 위한 통해 대해 대한 관련 경우 때문 이후 이전 현재 이번 다음
	정도 가장 매우 아주 너무 조금 다시 계속 이미 아직 항상 만약 만일 먼저 나중 모두 전부 일부 그냥
	있다 없다 있는 없는 있고 없고 있어 없어 있음 없음 된다 한다 했다 하는 되는 하고 되고 하면 되면
	것은 것이 것을 것도 것과 것의 것에 수도 수가 수는 수를 때는 때도 때에 이를 이는 그는 그를 저는
	등을 등의 등이 등은 등도
	배경 결정 결정사항 제약 제약사항 단계 결과 검증 비고 참고
`)

// tmKoSuffix is one Korean particle or ending. after constrains the syllable
// it attaches to, the way the allomorph pairs (이/가, 을/를, 은/는, 과/와,
// 으로/로) split by whether that syllable ends in a final consonant: a
// vowel-final loanword ending in 이 is not stripped as if 이 were a particle.
type tmKoSuffix struct {
	s     string
	after int8
}

const (
	koAfterAny        = iota
	koAfterConsonant  // the preceding syllable has a final consonant
	koAfterVowel      // the preceding syllable has none
	koAfterVowelOrRie // none, or ㄹ (the 로 forms)
)

// fits reports whether the suffix may attach to stem's last syllable.
func (sf tmKoSuffix) fits(stem []byte) bool {
	if sf.after == koAfterAny {
		return true
	}
	r, _ := utf8.DecodeLastRune(stem)
	if r < 0xAC00 || r > 0xD7A3 {
		return false
	}
	final := (r - 0xAC00) % 28
	switch sf.after {
	case koAfterConsonant:
		return final != 0
	case koAfterVowel:
		return final == 0
	default:
		return final == 0 || final == 8 // 8 is ㄹ
	}
}

// koSuffixes lists common josa and verb endings, longest first (ties by
// text), so hangulRun strips the longest match.
var koSuffixes = tmSortKoSuffixes([]tmKoSuffix{
	{"에게서", koAfterAny}, {"에서는", koAfterAny}, {"에서도", koAfterAny}, {"에서의", koAfterAny},
	{"에게는", koAfterAny}, {"까지는", koAfterAny}, {"부터는", koAfterAny}, {"했습니다", koAfterAny},
	{"되었다", koAfterAny}, {"합니다", koAfterAny}, {"됩니다", koAfterAny}, {"입니다", koAfterAny},
	{"시킨다", koAfterAny}, {"시키는", koAfterAny}, {"적으로", koAfterAny}, {"들에게", koAfterAny},
	{"들까지", koAfterAny}, {"이라는", koAfterConsonant}, {"이라고", koAfterConsonant},
	{"이었다", koAfterConsonant}, {"으로서", koAfterConsonant}, {"으로써", koAfterConsonant},
	{"으로는", koAfterConsonant}, {"으로의", koAfterConsonant},
	{"에서", koAfterAny}, {"에게", koAfterAny}, {"께서", koAfterAny}, {"한테", koAfterAny},
	{"까지", koAfterAny}, {"부터", koAfterAny}, {"보다", koAfterAny}, {"처럼", koAfterAny},
	{"마다", koAfterAny}, {"만큼", koAfterAny}, {"에는", koAfterAny}, {"에도", koAfterAny},
	{"에의", koAfterAny}, {"하고", koAfterAny}, {"하다", koAfterAny}, {"한다", koAfterAny},
	{"하는", koAfterAny}, {"하며", koAfterAny}, {"하여", koAfterAny}, {"해서", koAfterAny},
	{"했다", koAfterAny}, {"하면", koAfterAny}, {"하지", koAfterAny}, {"하게", koAfterAny},
	{"된다", koAfterAny}, {"되는", koAfterAny}, {"되고", koAfterAny}, {"되어", koAfterAny},
	{"되며", koAfterAny}, {"되면", koAfterAny}, {"됐다", koAfterAny}, {"적인", koAfterAny},
	{"들이", koAfterAny}, {"들을", koAfterAny}, {"들은", koAfterAny}, {"들의", koAfterAny},
	{"들에", koAfterAny}, {"들도", koAfterAny}, {"들과", koAfterAny}, {"들로", koAfterAny},
	{"으로", koAfterConsonant}, {"이나", koAfterConsonant}, {"이랑", koAfterConsonant},
	{"이며", koAfterConsonant}, {"이고", koAfterConsonant}, {"이란", koAfterConsonant},
	{"이다", koAfterConsonant}, {"과는", koAfterConsonant}, {"과의", koAfterConsonant},
	{"라는", koAfterVowel}, {"라고", koAfterVowel}, {"와는", koAfterVowel}, {"와의", koAfterVowel},
	{"였다", koAfterVowel}, {"로서", koAfterVowelOrRie}, {"로써", koAfterVowelOrRie},
	{"로는", koAfterVowelOrRie}, {"로의", koAfterVowelOrRie},
	{"의", koAfterAny}, {"에", koAfterAny}, {"도", koAfterAny}, {"만", koAfterAny},
	{"들", koAfterAny}, {"함", koAfterAny}, {"할", koAfterAny},
	{"이", koAfterConsonant}, {"을", koAfterConsonant}, {"은", koAfterConsonant},
	{"과", koAfterConsonant}, {"가", koAfterVowel}, {"를", koAfterVowel}, {"는", koAfterVowel},
	{"와", koAfterVowel}, {"나", koAfterVowel}, {"랑", koAfterVowel}, {"며", koAfterVowel},
	{"란", koAfterVowel}, {"로", koAfterVowelOrRie},
})

func tmSortKoSuffixes(list []tmKoSuffix) []tmKoSuffix {
	sort.SliceStable(list, func(i, j int) bool {
		if len(list[i].s) != len(list[j].s) {
			return len(list[i].s) > len(list[j].s)
		}
		return list[i].s < list[j].s
	})
	return list
}
