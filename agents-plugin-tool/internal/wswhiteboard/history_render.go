package wswhiteboard

import (
	"html/template"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

func (d itemData) HistoryAnchor() bool {
	return d.History == nil || d.History.ID == ""
}

func (d itemData) HistoryClassLabel() string {
	switch d.Class {
	case ClassPolicy:
		return wsi18n.T(d.Page.Lang, "wb.history.policy")
	case ClassTechnical:
		return wsi18n.T(d.Page.Lang, "wb.history.technical")
	case ClassAnnounced:
		return wsi18n.T(d.Page.Lang, "wb.history.announced")
	default:
		return wsi18n.T(d.Page.Lang, "wb.history.check")
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

func (d itemData) HistoryStatusLabel() string {
	switch d.Status {
	case StatusOpen:
		return StatusOpen
	case StatusCommented:
		return "user commented, lead's move"
	case StatusAssumed:
		return wsi18n.T(d.Page.Lang, "wb.history.assumed")
	case StatusWithdrawn:
		return wsi18n.T(d.Page.Lang, "wb.history.withdrawn")
	case StatusSuperseded:
		return wsi18n.T(d.Page.Lang, "wb.history.superseded")
	default:
		return wsi18n.T(d.Page.Lang, "wb.history.answered")
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
	// A stamped owner-answer row already shows the actual send time;
	// the mutable item's agent-apply clock is not another owner answer.
	if d.History != nil && d.History.OwnerAnswer() && d.History.AnswerSentAt > 0 {
		return ""
	}
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

// Keep substantive free answers in the disclosure, since their summary chip
// may be clipped. Plain ok needs no duplicate; picked options render via opts.
func (d itemData) HistoryReading() string {
	if !d.Void() {
		if d.PickedOption() == nil && strings.EqualFold(strings.TrimSpace(d.Answer), "ok") {
			return ""
		}
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
	return d.TimelineCheck() || d.HistoryReading() != "" || len(d.Options) > 0 || d.RecLetter != "" || d.HistoryQuote() != "" || d.HistoryAnswerTime() != "" || d.HistoryPastAnswer() != "" || strings.TrimSpace(string(d.HistoryRationaleHTML())) != ""
}
