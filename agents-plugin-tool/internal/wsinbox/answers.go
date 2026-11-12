package wsinbox

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The answer block is how a message carries its board answers and topic
// notes, ahead of the typed text:
//
//	<wb:answers>
//	341=a
//	123,567,691=b
//	124..129,181=ok
//	130=ok^user note
//	131=^multi-line note\nsecond line
//	[ship-gate]^topic note
//	</wb:answers>
//
// One line per entry, "<ids>=<value>[^<note>]": the items sharing a value
// and carrying no note are one line, their IDs ascending and comma-joined,
// a run of three or more consecutive IDs written "a..b"; an item with a note
// has its own line, the note after the first "^" ("id=^note" for a note
// alone). A topic note is "[<topic>]^<note>". In a note, newline is written
// "\n" and backslash "\\", so each entry stays one line. Lines follow the
// order of the first item each covers, topic notes last. The static page's
// copy-answers and the dashboard composer build the same block in
// wswhiteboard/assets/answers.js; a test holds the two to the same cases.
const (
	answersOpen  = "<wb:answers>"
	answersClose = "</wb:answers>"
)

var (
	answerLineIDs  = regexp.MustCompile(`^[0-9]{1,6}(\.\.[0-9]{1,6})?(,[0-9]{1,6}(\.\.[0-9]{1,6})?)*$`)
	answerLineItem = regexp.MustCompile(`^([0-9.,]+)=([A-Za-z0-9]{0,16})(\^(.*))?$`)
)

// TopicNote is the user's note on a board topic as a whole, from its note
// pen. Topic is the topic's key: its stem, or its name when it has none.
type TopicNote struct {
	Topic string `json:"topic"`
	Note  string `json:"note"`
}

// AnswerBlock renders chips and topic notes as the answer block; "" when
// neither carries anything.
func AnswerBlock(chips []Chip, topics []TopicNote) string {
	type line struct {
		ids  []string // a grouped line's IDs; nil for a noted item
		text string
	}
	var lines []*line
	groups := map[string]*line{}
	var values []string
	for _, c := range chips {
		note := strings.TrimSpace(c.Note)
		switch {
		case note != "":
			lines = append(lines, &line{text: c.ID + "=" + c.Answer + "^" + escapeAnswerNote(note)})
		case c.Answer != "":
			g := groups[c.Answer]
			if g == nil {
				g = &line{}
				groups[c.Answer] = g
				lines = append(lines, g)
				values = append(values, c.Answer)
			}
			g.ids = append(g.ids, c.ID)
		}
	}
	for _, v := range values {
		groups[v].text = compressIDs(groups[v].ids) + "=" + v
	}
	var out []string
	for _, l := range lines {
		out = append(out, l.text)
	}
	for _, t := range topics {
		if note := strings.TrimSpace(t.Note); note != "" {
			out = append(out, "["+t.Topic+"]^"+escapeAnswerNote(note))
		}
	}
	if len(out) == 0 {
		return ""
	}
	return answersOpen + "\n" + strings.Join(out, "\n") + "\n" + answersClose
}

// compressIDs joins ids ascending by number with ",", writing a run of three
// or more consecutive numbers as "<first>..<last>".
func compressIDs(ids []string) string {
	type id struct {
		s string
		n int
	}
	list := make([]id, len(ids))
	for i, s := range ids {
		n, _ := strconv.Atoi(s)
		list[i] = id{s, n}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].n < list[j].n })
	var parts []string
	for i := 0; i < len(list); {
		j := i
		for j+1 < len(list) && list[j+1].n == list[j].n+1 {
			j++
		}
		if j-i >= 2 {
			parts = append(parts, list[i].s+".."+list[j].s)
		} else {
			for k := i; k <= j; k++ {
				parts = append(parts, list[k].s)
			}
		}
		i = j + 1
	}
	return strings.Join(parts, ",")
}

// escapeAnswerNote writes a note on one line: CR LF and CR read as newline,
// backslash becomes "\\" and newline "\n".
func escapeAnswerNote(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, "\n", `\n`)
}

// unescapeAnswerNote reverses escapeAnswerNote; any other backslash pair
// stays as written.
func unescapeAnswerNote(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// AnswerLine is one parsed line of an answer block: an item line (IDs as
// written, such as "124..129,181", Value possibly empty for a note alone)
// or a topic note (Topic set).
type AnswerLine struct {
	IDs, Value, Note string
	Topic            string
}

// ParseAnswerBlock splits a message that starts with an answer block into
// its lines and the text after it. ok is false, and nothing is split, when
// text does not start with a well-formed block: the caller shows it whole.
func ParseAnswerBlock(text string) (lines []AnswerLine, rest string, ok bool) {
	body := strings.TrimLeft(text, " \t\r\n")
	if !strings.HasPrefix(body, answersOpen+"\n") {
		return nil, text, false
	}
	body = body[len(answersOpen)+1:]
	end := strings.Index(body, "\n"+answersClose)
	if end < 0 {
		return nil, text, false
	}
	after := body[end+1+len(answersClose):]
	if after != "" && after[0] != '\n' && !strings.HasPrefix(after, "\r\n") {
		return nil, text, false
	}
	for _, raw := range strings.Split(body[:end], "\n") {
		l, good := parseAnswerLine(strings.TrimSuffix(raw, "\r"))
		if !good {
			return nil, text, false
		}
		lines = append(lines, l)
	}
	return lines, strings.TrimLeft(after, "\r\n"), true
}

func parseAnswerLine(s string) (AnswerLine, bool) {
	if strings.HasPrefix(s, "[") {
		i := strings.Index(s, "]^")
		if i < 2 {
			return AnswerLine{}, false
		}
		note := unescapeAnswerNote(s[i+2:])
		return AnswerLine{Topic: s[1:i], Note: note}, strings.TrimSpace(note) != ""
	}
	m := answerLineItem.FindStringSubmatch(s)
	if m == nil || !answerLineIDs.MatchString(m[1]) {
		return AnswerLine{}, false
	}
	l := AnswerLine{IDs: m[1], Value: m[2], Note: unescapeAnswerNote(m[4])}
	return l, l.Value != "" || strings.TrimSpace(l.Note) != ""
}
