package wswhiteboard

import (
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// stateBadgeView is presentation only; the run remains the clock authority.
type stateBadgeView struct {
	Class, Icon, Label, Owner, Description                            string
	ETA, Elapsed, Estimate, Initial, Extensions, Percent, OverMinutes int64
	Over                                                              bool
}

func stateBadge(lang string, state *TopicState, now time.Time) stateBadgeView {
	b := stateBadgeView{}
	if state == nil {
		return b
	}
	switch state.Value {
	case StateImplementing:
		b.Class, b.Label = "impl", wsi18n.T(lang, "js.wb.state.implementing")
		if run := state.Run; run != nil {
			b.Owner, b.ETA, b.Initial, b.Extensions = run.Owner, run.ETAMinutes, run.InitialETAMinutes, int64(run.ExtensionCount)
			b.Elapsed = runElapsed(run, now) / 60000
			b.Over = b.Elapsed > b.ETA
			b.OverMinutes = b.Elapsed - b.ETA
			b.Percent = min(100, b.Elapsed*100/b.ETA)
		}
	case StateResearching:
		b.Class, b.Label = "research", wsi18n.T(lang, "js.wb.state.researching")
		b.Owner, b.Estimate = state.Owner, state.ETAMinutes
	case StateDiscussing:
		b.Class, b.Label = "discuss", wsi18n.T(lang, "js.wb.state.discussing")
	case StateWaiting:
		b.Class, b.Label = "wait", wsi18n.T(lang, "js.wb.state.waiting")
	case StateAwaitingConfirmation:
		b.Class, b.Label = "confirm", wsi18n.T(lang, "js.wb.state.awaiting-confirmation")
	default:
		return b
	}
	b.Icon = "st-" + b.Class
	parts := []string{b.Label}
	if b.Owner != "" {
		parts = append(parts, b.Owner)
	}
	if b.ETA > 0 {
		parts = append(parts, wsi18n.T(lang, "js.wb.state.eta.tip", b.ETA, b.Elapsed))
		if b.Over {
			parts = append(parts, wsi18n.T(lang, "js.wb.state.over", b.Elapsed-b.ETA))
		}
		if b.Extensions > 0 {
			parts = append(parts, wsi18n.T(lang, "js.wb.state.extended", b.Extensions, b.Initial))
		}
	}
	if b.Estimate > 0 {
		parts = append(parts, wsi18n.T(lang, "js.wb.state.estimate", b.Estimate))
	}
	b.Description = strings.Join(parts, " · ")
	return b
}

func (p *pageData) StateBadge(state *TopicState) stateBadgeView {
	return stateBadge(p.Lang, state, time.Now())
}
