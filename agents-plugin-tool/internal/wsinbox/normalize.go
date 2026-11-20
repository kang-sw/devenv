// Send normalization and validation for inbox entries.
package wsinbox

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Normalize trims and checks one send: empty parts are dropped, chips are
// validated, a later chip for an item (or note for a topic) replaces an
// earlier one, an empty topic note is dropped, a mention is trimmed and
// dropped when blank (its text counts toward the text limit), a pick and a
// console block are validated and capped (normalizeWeb; they do not count
// toward the text limit). Close requests dedupe by topic (last reason wins),
// count as content even without a reason, and their reasons count toward the
// text limit. A repeated image path is dropped with its
// number. An image, a pick, and a console block count as content. Image
// numbers are optional: ImageNums is empty or parallel to Images, a number is
// unique and at most maxImageNum, and an image numbered 0 (or every image,
// when ImageNums is empty) gets the next number above the highest. The
// result always numbers its images. It returns ErrEmpty when nothing is left
// to send and an ErrInvalid error for a malformed one. A command entry
// (normalizeCommand) carries its command alone.
func Normalize(e Entry) (Entry, error) {
	if e.UTCOffsetMinutes != nil && (*e.UTCOffsetMinutes < -720 || *e.UTCOffsetMinutes > 840) {
		return Entry{}, fmt.Errorf("%w: UTC offset must be -720..840 minutes", ErrInvalid)
	}
	if e.Command != nil {
		return normalizeCommand(e)
	}
	out := Entry{SchemaVersion: SchemaVersion, SentAt: e.SentAt, Steer: e.Steer, UTCOffsetMinutes: e.UTCOffsetMinutes}
	size, selectionMentions := 0, 0
	for _, p := range e.Parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var count int
		p, count = normalizeSelectionText(p)
		selectionMentions += count
		size += len(p)
		out.Parts = append(out.Parts, p)
	}
	if len(e.Mentions)+selectionMentions > maxMentions {
		return Entry{}, fmt.Errorf("%w: more than %d mentions", ErrInvalid, maxMentions)
	}
	for _, m := range e.Mentions {
		m.Topic = strings.TrimSpace(m.Topic)
		m.ID = strings.TrimSpace(m.ID)
		m.Entry = strings.TrimSpace(m.Entry)
		m.Text = strings.TrimSpace(m.Text)
		if m.Topic == "" || len(m.Topic) > maxTopicLen || strings.ContainsAny(m.Topic, "\r\n") {
			return Entry{}, fmt.Errorf("%w: mention topic %q", ErrInvalid, m.Topic)
		}
		if m.ID != "" && !chipIDPattern.MatchString(m.ID) {
			return Entry{}, fmt.Errorf("%w: mention item id %q", ErrInvalid, m.ID)
		}
		if m.Entry != "" && !mentionEntryPattern.MatchString(m.Entry) {
			return Entry{}, fmt.Errorf("%w: mention entry %q (expected eN)", ErrInvalid, m.Entry)
		}
		if m.Text == "" {
			continue
		}
		size += len(m.Text)
		out.Mentions = append(out.Mentions, m)
	}
	if len(e.CloseRequests) > maxTopics {
		return Entry{}, fmt.Errorf("%w: more than %d close requests", ErrInvalid, maxTopics)
	}
	for _, r := range e.CloseRequests {
		r.Topic, r.Reason = strings.TrimSpace(r.Topic), strings.TrimSpace(r.Reason)
		if r.Topic == "" || len(r.Topic) > maxTopicLen || strings.ContainsAny(r.Topic, "\x00\r\n") {
			return Entry{}, fmt.Errorf("%w: close request topic %q", ErrInvalid, r.Topic)
		}
		if len(r.Reason) > maxNoteBytes {
			return Entry{}, fmt.Errorf("%w: close reason for %s is longer than %d bytes", ErrInvalid, r.Topic, maxNoteBytes)
		}
		found := false
		for i := range out.CloseRequests {
			if out.CloseRequests[i].Topic == r.Topic {
				out.CloseRequests[i] = r
				found = true
				break
			}
		}
		if !found {
			out.CloseRequests = append(out.CloseRequests, r)
		}
	}
	for _, r := range out.CloseRequests {
		size += len(r.Reason)
	}
	if size > maxTextBytes {
		return Entry{}, fmt.Errorf("%w: text is longer than %d bytes", ErrInvalid, maxTextBytes)
	}
	var err error
	if out.Picks, out.Consoles, err = normalizeWeb(e.Picks, e.Consoles); err != nil {
		return Entry{}, err
	}
	if len(e.Chips) > maxChips {
		return Entry{}, fmt.Errorf("%w: more than %d answers", ErrInvalid, maxChips)
	}
	for _, c := range e.Chips {
		c.ID = strings.TrimSpace(c.ID)
		c.Answer = strings.TrimSpace(c.Answer)
		c.Note = strings.TrimSpace(c.Note)
		if !chipIDPattern.MatchString(c.ID) {
			return Entry{}, fmt.Errorf("%w: item id %q", ErrInvalid, c.ID)
		}
		if c.Answer != "" && !answerPattern.MatchString(c.Answer) {
			return Entry{}, fmt.Errorf("%w: answer %q for item %s", ErrInvalid, c.Answer, c.ID)
		}
		if len(c.Note) > maxNoteBytes {
			return Entry{}, fmt.Errorf("%w: note for item %s is longer than %d bytes", ErrInvalid, c.ID, maxNoteBytes)
		}
		if c.Answer == "" && c.Note == "" {
			continue
		}
		out.Chips = putChip(out.Chips, c)
	}
	if len(e.Topics) > maxTopics {
		return Entry{}, fmt.Errorf("%w: more than %d topic notes", ErrInvalid, maxTopics)
	}
	for _, tn := range e.Topics {
		tn.Topic = strings.TrimSpace(tn.Topic)
		tn.Note = strings.TrimSpace(tn.Note)
		// A topic key is one answer-block line's bracketed head, so it
		// holds no line break and no "]^", which ends the head.
		if tn.Topic == "" || len(tn.Topic) > maxTopicLen || strings.ContainsAny(tn.Topic, "\r\n") || strings.Contains(tn.Topic, "]^") {
			return Entry{}, fmt.Errorf("%w: topic %q", ErrInvalid, tn.Topic)
		}
		if len(tn.Note) > maxNoteBytes {
			return Entry{}, fmt.Errorf("%w: note for topic %s is longer than %d bytes", ErrInvalid, tn.Topic, maxNoteBytes)
		}
		if tn.Note == "" {
			continue
		}
		out.Topics = putTopic(out.Topics, tn)
	}
	if len(e.ImageNums) != 0 && len(e.ImageNums) != len(e.Images) {
		return Entry{}, fmt.Errorf("%w: %d image numbers for %d images", ErrInvalid, len(e.ImageNums), len(e.Images))
	}
	var nums []int
	for i, path := range e.Images {
		if path = strings.TrimSpace(path); path == "" {
			continue
		}
		if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
			return Entry{}, fmt.Errorf("%w: image path %q", ErrInvalid, path)
		}
		if indexOf(out.Images, path) >= 0 {
			continue
		}
		n := 0
		if len(e.ImageNums) != 0 {
			n = e.ImageNums[i]
		}
		if n < 0 || n > maxImageNum {
			return Entry{}, fmt.Errorf("%w: image number %d", ErrInvalid, n)
		}
		if n != 0 && containsInt(nums, n) {
			return Entry{}, fmt.Errorf("%w: image number %d given twice", ErrInvalid, n)
		}
		out.Images = append(out.Images, path)
		nums = append(nums, n)
	}
	if len(out.Images) > MaxImages {
		return Entry{}, fmt.Errorf("%w: more than %d images", ErrInvalid, MaxImages)
	}
	if len(out.Images) > 0 {
		next := maxInt(nums)
		for i := range nums {
			if nums[i] == 0 {
				next++
				nums[i] = next
			}
		}
		out.ImageNums = nums
	}
	for _, p := range out.Parts {
		for _, part := range ParseSelections(p) {
			if part.Selection == nil {
				continue
			}
			for _, n := range part.Selection.Images {
				if !containsInt(out.ImageNums, n) {
					return Entry{}, fmt.Errorf("%w: selection image %d is not attached", ErrInvalid, n)
				}
			}
		}
	}
	if len(out.Parts) == 0 && len(out.Chips) == 0 && len(out.Topics) == 0 && len(out.Mentions) == 0 && len(out.CloseRequests) == 0 && len(out.Picks) == 0 && len(out.Consoles) == 0 && len(out.Images) == 0 {
		return Entry{}, ErrEmpty
	}
	return out, nil
}

// normalizeCommand checks a command send: a name (one leading slash
// dropped) that commandNamePattern accepts, arguments trimmed and within
// the text limit, and no other content, since a command is never joined
// with a message.
func normalizeCommand(e Entry) (Entry, error) {
	c := Command{Name: strings.TrimPrefix(strings.TrimSpace(e.Command.Name), "/"), Args: strings.TrimSpace(e.Command.Args)}
	if !commandNamePattern.MatchString(c.Name) {
		return Entry{}, fmt.Errorf("%w: command name %q", ErrInvalid, c.Name)
	}
	if len(c.Args) > maxTextBytes {
		return Entry{}, fmt.Errorf("%w: command arguments are longer than %d bytes", ErrInvalid, maxTextBytes)
	}
	for _, p := range e.Parts {
		if strings.TrimSpace(p) != "" {
			return Entry{}, fmt.Errorf("%w: a command carries no text", ErrInvalid)
		}
	}
	if len(e.Chips) > 0 || len(e.Topics) > 0 || len(e.Mentions) > 0 || len(e.CloseRequests) > 0 || len(e.Picks) > 0 || len(e.Consoles) > 0 || len(e.Images) > 0 || e.Steer || e.Interrupt {
		return Entry{}, fmt.Errorf("%w: a command carries no answers, mentions, close requests, picks, console blocks, or images", ErrInvalid)
	}
	return Entry{SchemaVersion: SchemaVersion, SentAt: e.SentAt, UTCOffsetMinutes: e.UTCOffsetMinutes, Command: &c}, nil
}

// putChip replaces the chip for c.ID in place, or appends c.
func putChip(chips []Chip, c Chip) []Chip {
	for i := range chips {
		if chips[i].ID == c.ID {
			chips[i] = c
			return chips
		}
	}
	return append(chips, c)
}

// putTopic replaces the note for t.Topic in place, or appends t.
func putTopic(topics []TopicNote, t TopicNote) []TopicNote {
	for i := range topics {
		if topics[i].Topic == t.Topic {
			topics[i] = t
			return topics
		}
	}
	return append(topics, t)
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

func containsInt(list []int, n int) bool {
	for _, v := range list {
		if v == n {
			return true
		}
	}
	return false
}

func maxInt(list []int) int {
	m := 0
	for _, v := range list {
		if v > m {
			m = v
		}
	}
	return m
}
