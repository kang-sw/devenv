package wsinbox

import (
	"fmt"
	"regexp"
	"strings"
)

// A pick is an element the user chose on a localhost page in a web card, and
// a console block is the errors that page logged; the user attached them to
// the message so the lead reads what it points at. They travel the way
// mentions do (mentions.go): the message carries each as an element after
// the mention block and before the typed text, picks first, one per line:
//
//	<wb:pick url="http://localhost:5173/settings" selector="#save">Save</wb:pick>
//
// then the console blocks, one per line, each holding one error per line:
//
//	<wb:console url="http://localhost:5173/settings">first error
//	second error</wb:console>
//
// Escaping is the mentions': in attributes "&", "<", ">", and the quote; in
// the content "&", "<", and ">", so the first closing tag ends the element.
// The picks and the console blocks are each one paragraph.
const (
	pickClose    = "</wb:pick>"
	consoleClose = "</wb:console>"
)

var (
	pickHead    = regexp.MustCompile(`^<wb:pick url="([^"]*)" selector="([^"]*)">`)
	consoleHead = regexp.MustCompile(`^<wb:console url="([^"]*)">`)
)

// Pick is one picked element: the page's URL, a CSS selector for the element,
// and its text ("" for an element without text).
type Pick struct {
	URL      string `json:"url"`
	Selector string `json:"selector"`
	Text     string `json:"text"`
}

// Console is the errors one page logged, one per entry of Lines; a line holds
// no CR or LF.
type Console struct {
	URL   string   `json:"url"`
	Lines []string `json:"lines"`
}

// PickBlock renders picks as their elements, one per line; "" when there
// are none.
func PickBlock(picks []Pick) string {
	lines := make([]string, 0, len(picks))
	for _, p := range picks {
		lines = append(lines, `<wb:pick url="`+mentionAttrEscaper.Replace(p.URL)+`" selector="`+mentionAttrEscaper.Replace(p.Selector)+`">`+mentionTextEscaper.Replace(p.Text)+pickClose)
	}
	return strings.Join(lines, "\n")
}

// ConsoleBlock renders console blocks as their elements, one per line of
// the block's own errors; "" when there are none.
func ConsoleBlock(consoles []Console) string {
	elems := make([]string, 0, len(consoles))
	for _, c := range consoles {
		lines := make([]string, len(c.Lines))
		for i, l := range c.Lines {
			lines[i] = mentionTextEscaper.Replace(l)
		}
		elems = append(elems, `<wb:console url="`+mentionAttrEscaper.Replace(c.URL)+`">`+strings.Join(lines, "\n")+consoleClose)
	}
	return strings.Join(elems, "\n")
}

// ParseWebElements takes the pick elements, then the console elements, text
// starts with (after leading blank space) out of it, returning them unescaped
// and the text after them. Parsing stops at the first thing that is not a
// well-formed element of the kind expected next; with none, text comes back
// whole.
func ParseWebElements(text string) (picks []Pick, consoles []Console, rest string) {
	body := strings.TrimLeft(text, " \t\r\n")
	for {
		m := pickHead.FindStringSubmatch(body)
		if m == nil {
			break
		}
		content, tail, ok := elementBody(body[len(m[0]):], pickClose)
		if !ok {
			break
		}
		picks = append(picks, Pick{URL: mentionUnescaper.Replace(m[1]), Selector: mentionUnescaper.Replace(m[2]), Text: mentionUnescaper.Replace(content)})
		body = tail
	}
	for {
		m := consoleHead.FindStringSubmatch(body)
		if m == nil {
			break
		}
		content, tail, ok := elementBody(body[len(m[0]):], consoleClose)
		if !ok {
			break
		}
		var lines []string
		if content != "" {
			for _, l := range strings.Split(content, "\n") {
				lines = append(lines, mentionUnescaper.Replace(l))
			}
		}
		consoles = append(consoles, Console{URL: mentionUnescaper.Replace(m[1]), Lines: lines})
		body = tail
	}
	if len(picks) == 0 && len(consoles) == 0 {
		return nil, nil, text
	}
	return picks, consoles, body
}

// elementBody splits after (the text following an element's opening tag)
// into the escaped content up to closeTag and the text after the element's
// line break. ok is false when the element is unclosed, holds a raw "<", or
// has other text glued to its closing tag.
func elementBody(after, closeTag string) (content, tail string, ok bool) {
	end := strings.Index(after, closeTag)
	if end < 0 || strings.Contains(after[:end], "<") {
		return "", "", false
	}
	tail = after[end+len(closeTag):]
	if tail != "" && tail[0] != '\n' && !strings.HasPrefix(tail, "\r\n") {
		return "", "", false
	}
	return after[:end], strings.TrimLeft(tail, "\r\n"), true
}

// normalizeWeb checks and trims one send's picks and console blocks against
// the limits in inbox.go. A pick's url and selector are trimmed, required,
// and hold no CR or LF; its text is trimmed and may be empty. A console
// block needs a url; a line holding CR or LF is invalid, a blank line is
// dropped, and a block left with no lines is dropped. A count over a cap is
// ErrInvalid, as the mention cap is.
func normalizeWeb(picks []Pick, consoles []Console) ([]Pick, []Console, error) {
	if len(picks) > maxPicks {
		return nil, nil, fmt.Errorf("%w: more than %d picks", ErrInvalid, maxPicks)
	}
	if len(consoles) > maxConsoleBlocks {
		return nil, nil, fmt.Errorf("%w: more than %d console blocks", ErrInvalid, maxConsoleBlocks)
	}
	var outPicks []Pick
	for _, p := range picks {
		p.URL = strings.TrimSpace(p.URL)
		p.Selector = strings.TrimSpace(p.Selector)
		p.Text = strings.TrimSpace(p.Text)
		if err := checkWebURL("pick", p.URL); err != nil {
			return nil, nil, err
		}
		if p.Selector == "" || len(p.Selector) > maxSelectorBytes || strings.ContainsAny(p.Selector, "\r\n") {
			return nil, nil, fmt.Errorf("%w: pick selector %q (1-%d bytes, one line)", ErrInvalid, p.Selector, maxSelectorBytes)
		}
		if len(p.Text) > maxPickTextBytes {
			return nil, nil, fmt.Errorf("%w: pick text is longer than %d bytes", ErrInvalid, maxPickTextBytes)
		}
		outPicks = append(outPicks, p)
	}
	var outConsoles []Console
	for _, c := range consoles {
		c.URL = strings.TrimSpace(c.URL)
		if err := checkWebURL("console", c.URL); err != nil {
			return nil, nil, err
		}
		if len(c.Lines) > maxConsoleLines {
			return nil, nil, fmt.Errorf("%w: more than %d console lines in one block", ErrInvalid, maxConsoleLines)
		}
		var lines []string
		for _, l := range c.Lines {
			if strings.ContainsAny(l, "\r\n") {
				return nil, nil, fmt.Errorf("%w: a console line holds a line break", ErrInvalid)
			}
			if len(l) > maxConsoleLineBytes {
				return nil, nil, fmt.Errorf("%w: a console line is longer than %d bytes", ErrInvalid, maxConsoleLineBytes)
			}
			if strings.TrimSpace(l) != "" {
				lines = append(lines, l)
			}
		}
		if len(lines) == 0 {
			continue
		}
		outConsoles = append(outConsoles, Console{URL: c.URL, Lines: lines})
	}
	return outPicks, outConsoles, nil
}

// checkWebURL checks the url of a pick or console block: present, one line,
// and within maxWebURLBytes.
func checkWebURL(kind, url string) error {
	if url == "" || len(url) > maxWebURLBytes || strings.ContainsAny(url, "\r\n") {
		return fmt.Errorf("%w: %s url (1-%d bytes, one line)", ErrInvalid, kind, maxWebURLBytes)
	}
	return nil
}
