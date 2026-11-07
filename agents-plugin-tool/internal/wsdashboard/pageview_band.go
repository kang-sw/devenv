package wsdashboard

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// The status band around the live page's composer: the context bar above the
// message box, and the folder, branch, upstream standing, usage limits, and
// cost on one line below it. The figures are the snapshot's usage (what the
// host adapter last reported) and repo (what ws-mcp read); a part with no
// figure is left out. Times stay epoch milliseconds: the page formats a
// limit's reset and computes its pace, so the band's HTML changes only when
// a figure does.

const (
	// bandWarnPercent and bandCritPercent are where the context bar and a
	// usage limit turn warning and critical.
	bandWarnPercent = 80
	bandCritPercent = 95
)

// BandView is the status band.
type BandView struct {
	// Context is the context bar; nil without a context figure.
	Context *ContextBar
	// Repo is the session root's folder and branch; nil before ws-mcp's
	// first read.
	Repo *wslive.Repo
	// Limits are the usage limit chips, the 5-hour limit first.
	Limits []LimitChip
	// Cost is the cost chip's text; empty without a cost figure.
	Cost string
}

// Empty reports a band with nothing to show below the composer.
func (b BandView) Empty() bool { return b.Repo == nil && len(b.Limits) == 0 && b.Cost == "" }

// ContextBar is the context usage bar.
type ContextBar struct {
	// Known reports a percent; without one the bar shows "?" (the host does
	// not know its context size, as after a compaction).
	Known bool
	// Percent is the rounded percent; Fill the bar's width, clamped to 100.
	Percent, Fill int
	// Level is "", "warn", or "crit".
	Level string
	// Tokens is "<tokens> / <window>", either side "?" when unknown.
	Tokens string
	// Cache is the last response's cache hit rate ("92%"); empty when
	// unknown.
	Cache string
}

// Flag is the bar's warning word.
func (c ContextBar) Flag() string {
	switch c.Level {
	case "crit":
		return "near limit"
	case "warn":
		return "high"
	}
	return ""
}

// LimitChip is one usage limit chip: "42%/18:00 (+5%)" for the 5-hour
// limit, "17%/Sun (-72%)" for the weekly one.
type LimitChip struct {
	// Kind is wslive.LimitFiveHour or wslive.LimitSevenDay; Name is its
	// words for a title.
	Kind, Name string
	Percent    int
	Level      string
	// ResetsAt is the reset (epoch ms; 0 unknown) and ResetFormat the page's
	// format for it: a clock time for the 5-hour limit, a weekday for the
	// weekly one. ResetText is its fallback text (UTC).
	ResetsAt              int64
	ResetFormat, ResetText string
	// WindowSec is the limit's window, which the page's pace reads.
	WindowSec int64
	// Both marks a chip shown beside the other limit's; Hi the one of the
	// two under more pressure (higher percent, a tie keeping the 5-hour
	// limit), which alone stays on a narrow band.
	Both, Hi bool
}

// bandView builds the band from a snapshot.
func bandView(snap *wslive.Snapshot) BandView {
	var b BandView
	if snap.Repo != nil {
		repo := *snap.Repo
		b.Repo = &repo
	}
	u := snap.Usage
	if u == nil {
		return b
	}
	if u.Context != nil {
		b.Context = contextBar(*u.Context)
	}
	for _, l := range u.Limits {
		b.Limits = append(b.Limits, limitChip(l))
	}
	if len(b.Limits) == 2 {
		b.Limits[0].Both, b.Limits[1].Both = true, true
		hi := 0
		if b.Limits[1].Percent > b.Limits[0].Percent {
			hi = 1
		}
		b.Limits[hi].Hi = true
	}
	if u.Cost != nil {
		b.Cost = costText(*u.Cost)
	}
	return b
}

func level(pct int) string {
	switch {
	case pct >= bandCritPercent:
		return "crit"
	case pct >= bandWarnPercent:
		return "warn"
	}
	return ""
}

func contextBar(c wslive.UsageContext) *ContextBar {
	bar := &ContextBar{}
	if c.Percent != nil {
		bar.Known = true
		bar.Percent = int(math.Round(*c.Percent))
		bar.Fill = min(max(bar.Percent, 0), 100)
		bar.Level = level(bar.Percent)
	}
	tokens, window := "?", "?"
	if c.Tokens != nil {
		tokens = commafy(*c.Tokens)
	}
	if c.Window > 0 {
		window = tokenCount(c.Window)
	}
	bar.Tokens = tokens + " / " + window
	if c.CacheHit != nil {
		bar.Cache = strconv.Itoa(int(math.Round(*c.CacheHit))) + "%"
	}
	return bar
}

func limitChip(l wslive.UsageLimit) LimitChip {
	pct := int(math.Round(l.Percent))
	chip := LimitChip{Kind: l.Kind, Percent: pct, Level: level(pct), ResetsAt: l.ResetsAt}
	reset := time.UnixMilli(l.ResetsAt).UTC()
	if l.Kind == wslive.LimitSevenDay {
		chip.Name, chip.ResetFormat, chip.WindowSec = "weekly", "wd", 7*24*3600
		chip.ResetText = reset.Format("Mon")
	} else {
		chip.Name, chip.ResetFormat, chip.WindowSec = "5-hour", "hm", 5*3600
		chip.ResetText = reset.Format("15:04")
	}
	if l.ResetsAt <= 0 {
		chip.ResetText = ""
	}
	return chip
}

// costText is the cost chip: "$1.23" for the host's own figure, "~$1.23" for
// an estimate, "~$1.23+" for an estimate that left out calls of unknown
// price.
func costText(c wslive.UsageCost) string {
	text := fmt.Sprintf("$%.2f", c.USD)
	if c.Estimate {
		text = "~" + text
		if c.Incomplete {
			text += "+"
		}
	}
	return text
}

// commafy is n with thousands separators.
func commafy(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := n < 0
	if neg {
		s = s[1:]
	}
	var out []byte
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// tokenCount is a window size: "300K", "1M", "1.5M", or the plain number
// when it is not a round thousand.
func tokenCount(n int64) string {
	switch {
	case n >= 1_000_000 && n%1_000_000 == 0:
		return strconv.FormatInt(n/1_000_000, 10) + "M"
	case n >= 1_000_000:
		return strconv.FormatFloat(float64(n)/1_000_000, 'f', 1, 64) + "M"
	case n >= 1_000 && n%1_000 == 0:
		return strconv.FormatInt(n/1_000, 10) + "K"
	}
	return commafy(n)
}
