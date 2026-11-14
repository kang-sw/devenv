package wswhiteboard

import (
	"fmt"
	"html/template"
	"strings"
	"time"
)

func (d itemData) HistoryAnchor() bool {
	return d.History == nil || d.History.ID != fmt.Sprintf("item-%d", d.ID)
}

func (d itemData) HistoryClassKey() string {
	switch d.Class {
	case ClassPolicy:
		return "wb.history.policy"
	case ClassTechnical:
		return "wb.history.technical"
	case ClassAnnounced:
		return "wb.history.announced"
	default:
		return "wb.history.check"
	}
}

func (d itemData) HistoryMark() string {
	switch d.Class {
	case ClassPolicy:
		return "P"
	case ClassTechnical:
		return "T"
	case ClassAnnounced:
		return "A"
	default:
		return "C"
	}
}

func (d itemData) HistoryStatusKey() string {
	switch d.Status {
	case StatusAssumed:
		return "wb.history.assumed"
	case StatusWithdrawn:
		return "wb.history.withdrawn"
	case StatusSuperseded:
		return "wb.history.superseded"
	default:
		return "wb.history.answered"
	}
}

// Only a literal plain ok is represented entirely by its answer badge and
// verbatim tooltip. A title-like rationale or a substantive quote is still
// content, never a string-similarity/length heuristic for an empty disclosure.
func (d itemData) HistoryQuote() string {
	if strings.TrimSpace(d.AnswerQuote) == "" {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(d.Answer), "ok") && strings.EqualFold(strings.TrimSpace(d.AnswerQuote), "ok") {
		return ""
	}
	return d.AnswerQuote
}

// Answer time is recorded data, not a fallback for the row's event time.
// A withdrawal/supersession can retain an earlier answer; show that distinct
// answer time explicitly without calling either timestamp an archive time.
func (d itemData) HistoryAnswerTime() string {
	at, err := time.Parse(time.RFC3339Nano, d.AnsweredAt)
	if err != nil {
		return ""
	}
	if d.History != nil {
		if event, err := time.Parse(time.RFC3339Nano, d.History.At); err == nil && event.Truncate(time.Second).Equal(at.Truncate(time.Second)) {
			return ""
		}
	}
	return d.AnsweredAt
}

// A picked option's remainder is interpretation, not another answer badge.
// Free-text answers already occupy the single badge and use TextInline there.
func (d itemData) HistoryReading() string {
	if !d.Void() && d.PickedOption() != nil {
		return d.Reading()
	}
	return ""
}

func (d itemData) HistoryPastAnswer() string {
	if d.Void() {
		return d.Answer
	}
	return ""
}

func (d itemData) HistoryRationaleHTML() template.HTML {
	if strings.TrimSpace(d.Rationale) == "" {
		return ""
	}
	return d.RationaleHTML()
}

func (d itemData) HistoryExpandable() bool {
	return len(d.Options) > 0 || d.RecLetter != "" || d.HistoryQuote() != "" || d.HistoryAnswerTime() != "" || d.HistoryPastAnswer() != "" || strings.TrimSpace(string(d.HistoryRationaleHTML())) != ""
}
