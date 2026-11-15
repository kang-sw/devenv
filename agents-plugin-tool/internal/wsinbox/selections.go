package wsinbox

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	selectionOpen  = "<wb:selection>"
	selectionClose = "</wb:selection>"
	commentOpen    = "<wb:comment>"
	commentClose   = "</wb:comment>"
)

// Selection is one composer push: ordered source excerpts, exactly one user
// memo (possibly empty), and the numbers of its attached images. It is a
// transient view of existing Parts text, never an additional stored field.
// Only Images are references: marker-looking text in either source or memo
// is literal content, not an instruction to renumber an attachment.
type Selection struct {
	Mentions []Mention
	Comment  string
	Images   []int
}

// SelectionPart is a lossless segment of message text. Text always holds
// the original bytes; Selection is non-nil only for a complete valid group.
// Ordinary text and malformed groups have no Selection.
type SelectionPart struct {
	Text      string
	Selection *Selection
}

// SelectionBlock serializes a push into the existing text transport. Source
// and memo are escaped independently, so neither can introduce a child or
// close the group. The image tail uses only the existing ImageMarker form.
func SelectionBlock(s Selection) (string, error) {
	if len(s.Mentions) == 0 || len(s.Mentions) > maxMentions {
		return "", fmt.Errorf("%w: a selection needs 1..%d mentions", ErrInvalid, maxMentions)
	}
	for _, m := range s.Mentions {
		if strings.TrimSpace(m.Topic) == "" || len(m.Topic) > maxTopicLen || strings.ContainsAny(m.Topic, "\r\n") ||
			(m.ID != "" && !chipIDPattern.MatchString(m.ID)) ||
			(m.Entry != "" && !mentionEntryPattern.MatchString(m.Entry)) || strings.TrimSpace(m.Text) == "" {
			return "", fmt.Errorf("%w: malformed selection mention", ErrInvalid)
		}
	}
	var seen []int
	for _, n := range s.Images {
		if n < 1 || n > 999999 || containsInt(seen, n) {
			return "", fmt.Errorf("%w: selection image number %d", ErrInvalid, n)
		}
		seen = append(seen, n)
	}
	lines := []string{selectionOpen, protectSelectionMarkers(MentionBlock(s.Mentions)), commentOpen + protectSelectionMarkers(mentionTextEscaper.Replace(s.Comment)) + commentClose}
	for _, n := range s.Images {
		lines = append(lines, ImageMarker(n))
	}
	return strings.Join(append(lines, selectionClose), "\n"), nil
}

// Numeric-escape the opening bracket of literal markers after normal XML
// escaping. An older writer's flat Parts marker remapper then sees only the
// real references in the tail. Decoding the numeric escape before &amp;
// keeps a user's literal "&#91;" distinct from this escape (one decode only).
func protectSelectionMarkers(text string) string {
	return imageMarkerPattern.ReplaceAllStringFunc(text, func(marker string) string { return "&#91;" + marker[1:] })
}

// ParseSelections splits text at complete, line-delimited selection groups,
// including groups between ordinary paragraphs. A malformed envelope stays
// wholly ordinary text: in particular, a nested valid-looking group is not
// recovered from it. Legacy leading mention parsing is unchanged.
func ParseSelections(text string) []SelectionPart {
	var parts []SelectionPart
	appendPart := func(raw string, s *Selection) {
		if raw == "" {
			return
		}
		if s == nil && len(parts) > 0 && parts[len(parts)-1].Selection == nil {
			parts[len(parts)-1].Text += raw
			return
		}
		parts = append(parts, SelectionPart{Text: raw, Selection: s})
	}
	walkSelections(text, func(raw string, group bool, s *Selection) { appendPart(raw, s) })
	return parts
}

// Prefixes include malformed attribute-bearing and unterminated openers so
// their nested contents cannot escape the whole-group fallback. Escaped
// source/comment content cannot match these prefixes.
var selectionTagPrefix = regexp.MustCompile(`^</?wb:selection(?:[^A-Za-z0-9_.:-]|$)`)

// selectionTags locates envelope boundaries, not valid children. Comments,
// CDATA, processing instructions and quoted attributes cannot contribute a
// boundary. They remain forbidden by parseSelection; skipping their contents
// here prevents a false close from exposing a nested group in failed text.
// Inside an envelope, an ambiguous/unterminated tag keeps the candidate
// opaque through the end. Outside one, an ordinary non-tag '<' must not
// prevent discovery of later independent envelopes.
func selectionTags(text string) [][2]int {
	var tags [][2]int
	depth := 0
	for pos := 0; pos < len(text); {
		n := strings.IndexByte(text[pos:], '<')
		if n < 0 {
			break
		}
		start := pos + n
		body := text[start:]
		terminator := ""
		switch {
		case strings.HasPrefix(body, "<!--"):
			terminator = "-->"
		case strings.HasPrefix(body, "<![CDATA["):
			terminator = "]]>"
		case strings.HasPrefix(body, "<?"):
			terminator = "?>"
		case strings.HasPrefix(body, "<!"):
			// Other declarations are outside the grammar; their internal
			// syntax cannot safely supply an envelope close.
			return tags
		}
		if terminator != "" {
			end := strings.Index(body, terminator)
			if end < 0 {
				return tags
			}
			pos = start + end + len(terminator)
			continue
		}
		candidate := selectionTagPrefix.MatchString(body)
		quote := byte(0)
		end := start + 1
		for ; end < len(text); end++ {
			c := text[end]
			if quote != 0 {
				if c == quote {
					quote = 0
				}
				continue
			}
			if c == '\'' || c == '"' {
				quote = c
			} else if c == '>' {
				break
			} else if c == '<' {
				break
			}
		}
		if end == len(text) || text[end] != '>' {
			if candidate {
				tags = append(tags, [2]int{start, len(text)})
				return tags
			}
			if depth > 0 {
				return tags
			}
			// No enclosing selection owns this non-tag text. Resume after
			// its '<', without imposing its incomplete quoting on siblings.
			pos = start + 1
			continue
		}
		pos = end + 1
		if candidate {
			tags = append(tags, [2]int{start, pos})
			if text[start+1] != '/' {
				depth++
			} else if depth > 0 {
				depth--
			}
		}
	}
	return tags
}

// walkSelections keeps failed envelopes opaque instead of recovering nested
// children. Boundary lexing and strict child parsing are separate.
func walkSelections(text string, visit func(raw string, group bool, s *Selection)) {
	tags := selectionTags(text)
	last := 0
	for i := 0; i < len(tags); i++ {
		start := tags[i][0]
		if text[start+1] == '/' {
			continue
		}
		if start > last {
			visit(text[last:start], false, nil)
		}
		depth, end := 1, len(text)
		for i++; i < len(tags); i++ {
			if text[tags[i][0]+1] == '/' {
				depth--
			} else {
				depth++
			}
			if depth == 0 {
				end = tags[i][1]
				break
			}
		}
		raw := text[start:end]
		var s *Selection
		if (start == 0 || text[start-1] == '\n') &&
			(end == len(text) || text[end] == '\n' || strings.HasPrefix(text[end:], "\r\n")) {
			s = parseSelection(raw)
		}
		visit(raw, true, s)
		last = end
	}
	if last < len(text) {
		visit(text[last:], false, nil)
	}
}

func parseSelection(raw string) *Selection {
	if !strings.HasPrefix(raw, selectionOpen+"\n") && !strings.HasPrefix(raw, selectionOpen+"\r\n") {
		return nil
	}
	if !strings.HasSuffix(raw, selectionClose) {
		return nil
	}
	body := strings.TrimLeft(raw[len(selectionOpen):len(raw)-len(selectionClose)], "\r\n")
	comment := strings.Index(body, commentOpen)
	if comment < 0 {
		return nil
	}
	// Decode the bracket escape in source before the legacy entity decoder,
	// once. Literal &amp;#91; is still escaped here and stays literal. The
	// image tail is never decoded: references use the existing raw marker.
	mentions, rest := ParseMentions(strings.ReplaceAll(body[:comment], "&#91;", "["))
	if len(mentions) == 0 || rest != "" {
		return nil
	}
	body = body[comment+len(commentOpen):]
	end := strings.Index(body, commentClose)
	if end < 0 || strings.Contains(body[:end], "<") {
		return nil
	}
	s := Selection{Mentions: mentions, Comment: mentionUnescaper.Replace(strings.ReplaceAll(body[:end], "&#91;", "["))}
	tail := body[end+len(commentClose):]
	if tail == "" || (tail[0] != '\n' && !strings.HasPrefix(tail, "\r\n")) {
		return nil
	}
	for _, line := range strings.Split(strings.Trim(tail, "\r\n"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		match := imageMarkerPattern.FindStringSubmatch(line)
		if match == nil || match[0] != line {
			return nil
		}
		n, _ := strconv.Atoi(match[1])
		s.Images = append(s.Images, n)
	}
	if _, err := SelectionBlock(s); err != nil {
		return nil
	}
	return &s
}

// Excerpt is free owner text, without serialized selection/mention groups
// or attachment markers. Malformed groups remain opaque, literal text.
func (e Entry) Excerpt() string {
	var parts []string
	for _, text := range e.Parts {
		walkSelections(text, func(raw string, group bool, s *Selection) {
			if s != nil {
				return
			}
			if !group {
				_, raw = ParseMentions(raw)
				raw = imageMarkerPattern.ReplaceAllString(raw, "")
				var lines []string
				for _, line := range strings.Split(raw, "\n") {
					if _, _, ok := ParseImageLine(line); !ok {
						lines = append(lines, line)
					}
				}
				raw = strings.Join(lines, "\n")
			}
			parts = append(parts, raw)
		})
	}
	runes := []rune(strings.TrimSpace(strings.Join(parts, "\n")))
	if len(runes) > 240 {
		runes = runes[:240]
	}
	return string(runes)
}

// normalizeSelectionText makes the server the serializer of valid groups,
// retaining malformed groups as ordinary text. Validation of actual image
// ownership happens after Normalize assigns the entry's image numbers.
func normalizeSelectionText(text string) (string, int) {
	var b strings.Builder
	mentions := 0
	walkSelections(text, func(raw string, group bool, s *Selection) {
		if s != nil {
			raw, _ = SelectionBlock(*s)
			mentions += len(s.Mentions)
		}
		b.WriteString(raw)
	})
	return b.String(), mentions
}
