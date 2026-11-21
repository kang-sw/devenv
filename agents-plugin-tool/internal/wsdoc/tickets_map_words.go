package wsdoc

import (
	"bytes"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Word tokenizers and stoplists of the ticket map text engine
// (tickets_map_text.go): Latin, Hangul and CJK runs, and the generic
// function-word lists.

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
