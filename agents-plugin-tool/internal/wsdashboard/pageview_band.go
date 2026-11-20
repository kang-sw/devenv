package wsdashboard

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// The status band around the live page's composer: the context bar above the
// message box and its derived session-hover cost. The composer footer below the
// message box now holds the slim model/effort control strip (r-sline is empty).
// The figures are the snapshot's usage (what the host adapter last reported)
// and repo (what ws-mcp read for gitbadge follow).

const (
	// bandWarnPercent and bandCritPercent are fallback thresholds for the
	// context bar when no resolved ws compaction thresholds exist (e.g.
	// external terminal and handed-off sessions).
	bandWarnPercent = 80
	bandCritPercent = 95
)

// BandView is the status band.
type BandView struct {
	// Context is the context bar; nil without a context figure.
	Context *ContextBar
	// Repo is the snapshot's repo figures; nil before ws-mcp's first read.
	Repo *wslive.Repo
	// Limits are retained on BandView for snapshot callers, though r-sline no
	// longer renders footer quota chips.
	Limits []LimitChip
	// Cost is the cost text (used by the rail row hover card).
	Cost string
}

// Empty reports a band with nothing to show below the composer.
func (b BandView) Empty() bool {
	return true
}

// ContextBar is the context usage bar, scaled to ws compaction hard point when
// available or the native context window otherwise.
type ContextBar struct {
	// Known reports a known current token count; without one the bar shows "?".
	Known bool
	// Percent is the rounded unclamped percent; Fill is clamped 0..100 for
	// aria-valuenow; FillWidth is the clamped percent formatted for CSS width.
	Percent, Fill int
	FillWidth     string
	// Level is "", "warn", "d-hot", "crit", or "d-unknown".
	Level string
	// Tokens is "<current> / <target>" (e.g. "51,293 / 180K" or "? / 180K").
	Tokens string
	// Native is the optional native window parenthetical (e.g. "1.0M"), omitted
	// when target == native or native is unknown.
	Native string
	// FullTokens is Tokens plus " (<Native>)" when Native is non-empty.
	FullTokens string
	// Cache is the last response's cache hit rate ("92%"); empty when unknown.
	Cache string
	// Dynamic boundary tick positions in percent (0..100, "%.1f"), empty if absent.
	SoftPct       string
	MilestonePcts []string
	// Floating boundary icon above ONLY the highest crossed threshold.
	FloatIcon     string
	FloatLeft     string
	FloatAtHard   bool
	FloatColor    string
	FloatTitleKey string
	FloatTitleArg string
	// Tip breakdown values for .ctx-tip.
	TipCurrent string
	TipSoft    string
	TipM1      string
	TipM2      string
	TipHard    string
	TipNative  string
}

// LimitChip is retained for compatibility with BandView callers.
type LimitChip struct {
	Kind, Name string
	Percent    int
	Level      string
	ResetsAt   int64
	ResetFormat, ResetText string
	WindowSec  int64
	Both, Hi   bool
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

func fmtPct(num, denom int64) string {
	if denom <= 0 {
		return ""
	}
	p := float64(num) / float64(denom) * 100
	if p < 0 {
		p = 0
	} else if p > 100 {
		p = 100
	}
	return strconv.FormatFloat(p, 'f', 1, 64)
}

func contextBar(c wslive.UsageContext) *ContextBar {
	bar := &ContextBar{FillWidth: "0.0", Level: "d-unknown"}
	comp := c.Compaction
	var hard, soft int64
	var m1, m2 int64
	if comp != nil {
		soft, hard = comp.Soft, comp.Hard
		if len(comp.Milestones) >= 2 {
			m1, m2 = comp.Milestones[0], comp.Milestones[1]
		}
	}
	denom := c.Window
	if hard > 0 {
		denom = hard
	}
	if denom > 0 {
		if soft > 0 && soft < denom {
			bar.SoftPct = fmtPct(soft, denom)
		}
		if hard > 0 && m1 > 0 && m2 > 0 && m1 < denom && m2 < denom {
			bar.MilestonePcts = []string{fmtPct(m1, denom), fmtPct(m2, denom)}
		}
	}

	if c.Tokens != nil && denom > 0 {
		cur := *c.Tokens
		exactPct := float64(cur) / float64(denom) * 100
		bar.Known = true
		bar.Percent = int(math.Round(exactPct))
		clamped := math.Min(100, math.Max(0, exactPct))
		bar.Fill = min(max(bar.Percent, 0), 100)
		bar.FillWidth = strconv.FormatFloat(clamped, 'f', 1, 64)
		bar.Level = ""
		if hard > 0 {
			switch {
			case cur >= hard:
				bar.Level = "crit"
				bar.FloatIcon = "zap"
				bar.FloatAtHard = true
				bar.FloatColor = "var(--wb-fail)"
				bar.FloatTitleKey = "band.ctx.boundary.hard"
				bar.FloatTitleArg = tokenCount(hard)
			case m2 > 0 && cur >= m2:
				bar.Level = "d-hot"
				bar.FloatIcon = "bell-ring"
				bar.FloatLeft = fmtPct(m2, denom)
				bar.FloatColor = "var(--wb-hot)"
				bar.FloatTitleKey = "band.ctx.boundary.m2"
				bar.FloatTitleArg = tokenCount(m2)
			case m1 > 0 && cur >= m1:
				bar.Level = "warn"
				bar.FloatIcon = "bell"
				bar.FloatLeft = fmtPct(m1, denom)
				bar.FloatColor = "var(--wb-warn)"
				bar.FloatTitleKey = "band.ctx.boundary.m1"
				bar.FloatTitleArg = tokenCount(m1)
			case soft > 0 && cur >= soft:
				bar.Level = "warn"
				bar.FloatIcon = "info"
				bar.FloatLeft = fmtPct(soft, denom)
				bar.FloatColor = "var(--wb-warn)"
				bar.FloatTitleKey = "band.ctx.boundary.soft"
				bar.FloatTitleArg = tokenCount(soft)
			}
		} else {
			bar.Level = level(bar.Percent)
		}
		bar.TipCurrent = commafy(cur)
	} else if c.Tokens != nil && c.Percent != nil {
		bar.Known = true
		bar.Percent = int(math.Round(*c.Percent))
		bar.Fill = min(max(bar.Percent, 0), 100)
		bar.FillWidth = strconv.FormatFloat(float64(bar.Fill), 'f', 1, 64)
		bar.Level = level(bar.Percent)
		bar.TipCurrent = commafy(*c.Tokens)
	}

	curText, targetText := "?", "?"
	if c.Tokens != nil {
		curText = commafy(*c.Tokens)
	}
	if denom > 0 {
		targetText = tokenCount(denom)
	}
	bar.Tokens = curText + " / " + targetText
	if c.Window > 0 && denom > 0 && denom != c.Window {
		bar.Native = tokenCount(c.Window)
		bar.FullTokens = bar.Tokens + " (" + bar.Native + ")"
	} else {
		bar.FullTokens = bar.Tokens
	}
	if soft > 0 {
		bar.TipSoft = tokenCount(soft)
	}
	if m1 > 0 && m2 > 0 {
		bar.TipM1 = tokenCount(m1)
		bar.TipM2 = tokenCount(m2)
	}
	if hard > 0 {
		bar.TipHard = tokenCount(hard)
	}
	if c.Window > 0 {
		bar.TipNative = tokenCount(c.Window)
	} else if denom > 0 {
		bar.TipNative = tokenCount(denom)
	}
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

// tokenCount is a window or threshold size: "180K", "1.0M", "1.5M", or the
// comma-formatted number when it is not a round thousand.
func tokenCount(n int64) string {
	switch {
	case n >= 1_000_000 && n%1_000_000 == 0:
		return strconv.FormatInt(n/1_000_000, 10) + ".0M"
	case n >= 1_000_000:
		return strconv.FormatFloat(float64(n)/1_000_000, 'f', 1, 64) + "M"
	case n >= 1_000 && n%1_000 == 0:
		return strconv.FormatInt(n/1_000, 10) + "K"
	}
	return commafy(n)
}
