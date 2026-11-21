// Page format helpers build localized template display values.

package wsdashboard

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
	"github.com/kang-sw/devenv/internal/wslive"
)

// releaseRepository is the GitHub owner/name the ws binary is released from,
// the Settings footer's repository link. The plugin's runtime.json
// release_repository says the same for the launcher's binary download, but the
// shared daemon never reads runtime.json, so the value is restated here and a
// test pins the two together.
const releaseRepository = "kang-sw/werkschaft"

// pageFuncs is the template FuncMap for one language: the page helpers and
// the translation funcs t, tn, and tcode, with the Go-built display strings
// (token usage, batch lines, phase and band words) bound to lang.
func pageFuncs(lang string, icons map[string]bool) template.FuncMap {
	return template.FuncMap{
		"icon": func(name string) (template.HTML, error) {
			if !icons[name] {
				return "", fmt.Errorf("dashboard page: no sprite symbol %q", name)
			}
			return template.HTML(`<svg class="i" aria-hidden="true"><use href="#i-` + name + `"/></svg>`), nil
		},
		"settingsgear": func(scope, target, worktree string) settingsGear {
			return settingsGear{scope, target, worktree}
		},
		"region":   renderRegionElement,
		"clock":    clockText,
		"spanText": spanText,
		"toolName": toolName,
		"shortID":  shortID,
		"hudJSON": func(v HUDView) (template.JS, error) {
			var strip func([]HUDNode) []HUDNode
			strip = func(in []HUDNode) []HUDNode {
				if len(in) == 0 {
					return in
				}
				out := make([]HUDNode, len(in))
				for i, n := range in {
					if n.OwnRow {
						n.Task = ""
					}
					n.Children = strip(n.Children)
					out[i] = n
				}
				return out
			}
			v.Roots = strip(v.Roots)
			b, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return template.JS(b), nil
		},
		"sinceText": func(at, ms int64) string { return spanText(max(at-ms+500, 0)) },
		"marked":    func(text string) template.HTML { return markedText(lang, text) },
		// The tool summary's token and batch lines.
		"tokenText":        func(requests int, u *wslive.TokenUsage) string { return tokenText(lang, requests, u) },
		"tokenTitle":       func(requests int, u *wslive.TokenUsage) string { return tokenTitle(lang, requests, u) },
		"requests":         summaryRequests,
		"batchNums":        func(b wslive.ToolBatch) []batchNum { return batchNums(lang, b) },
		"batchArgs":        batchArgs,
		"argsList":         batchArgsList,
		"phaseWord":        func(phase string) string { return phaseWord(lang, phase) },
		"ctxBoundaryTitle": func(kind, arg string) string { return ctxBoundaryTitle(lang, kind, arg) },
		// Retained for the SHA256-pinned historical template fixture (testdata/pre_effects/dashboard.tmpl).
		"flagText":  flagText,
		"limitName": limitName,
		// Translation: t is a table lookup, tn the count form of a key, tcode
		// a lookup whose one placeholder renders as <code>.
		"t":     func(key string, args ...any) string { return wsi18n.T(lang, key, args...) },
		"tn":    func(key string, n int, args ...any) string { return wsi18n.Plural(lang, key, n, args...) },
		"tcode": func(key string, arg string) template.HTML { return codeText(lang, key, arg) },
		// lang is the language this set renders in; langs the available ones;
		// i18n the inline script giving the page scripts their table.
		"lang":  func() string { return effectiveLang(lang) },
		"langs": wsi18n.Languages,
		"i18n":  func() template.JS { return wsi18n.ScriptExcept(lang, "js.settings.") },
		// pwa is a page head's manifest link and theme-color metas (pwa.go).
		"pwa": pwaHead,
		// repoURL is the Settings footer's link to the ws repository.
		"repoURL": func() string { return "https://github.com/" + releaseRepository },
		// guideshot is a landing guide step's screenshot (landing_guide.go).
		"guideshot": func(step, alt string) template.HTML { return guideShotHTML(lang, step, alt) },
	}
}

// effectiveLang is lang when a table exists for it, else the default.
func effectiveLang(lang string) string {
	if wsi18n.Supported(lang) {
		return lang
	}
	return wsi18n.Default
}

// codeText is the table text for key with its {0} placeholder drawn as a
// <code> element holding arg; the rest of the text and arg are escaped.
func codeText(lang, key, arg string) template.HTML {
	text, _ := wsi18n.Lookup(lang, key)
	before, after, found := strings.Cut(text, "{0}")
	if !found {
		return template.HTML(template.HTMLEscapeString(text))
	}
	return template.HTML(template.HTMLEscapeString(before) + "<code>" + template.HTMLEscapeString(arg) + "</code>" + template.HTMLEscapeString(after))
}

// phaseWord is a lead or agent phase's display word.
func phaseWord(lang, phase string) string {
	switch phase {
	case "permission":
		return wsi18n.T(lang, "phase.approval")
	case "thinking":
		return wsi18n.T(lang, "phase.thinking")
	case "replying":
		return wsi18n.T(lang, "phase.replying")
	case "tool":
		return wsi18n.T(lang, "phase.tool")
	case "compacting":
		return wsi18n.T(lang, "phase.compacting")
	case "idle":
		return wsi18n.T(lang, "phase.idle")
	}
	return phase
}

func ctxBoundaryTitle(lang, icon, arg string) string {
	switch icon {
	case "zap":
		return wsi18n.T(lang, "band.ctx.boundary.hard", arg)
	case "bell-ring":
		return wsi18n.T(lang, "band.ctx.boundary.m2", arg)
	case "bell":
		return wsi18n.T(lang, "band.ctx.boundary.m1", arg)
	case "info":
		return wsi18n.T(lang, "band.ctx.boundary.soft", arg)
	}
	return ""
}

func flagText(flag string) string {
	if flag == "near limit" {
		return "near limit"
	}
	return "high"
}

func limitName(name string) string {
	if name == "weekly" {
		return "weekly"
	}
	return "5-hour"
}

// clockText is a time's fallback text, UTC and fixed, so a region's HTML
// does not change with the time it was rendered at; the page script replaces
// it with local or relative time.
func clockText(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format("15:04:05Z")
}

// toolName is a tool's display name: an MCP tool, mcp__<server>__<tool>, shows
// as <tool> (the server name may itself hold single underscores). The full
// name stays in a title attribute where the page shows it.
func toolName(name string) string {
	if rest, ok := strings.CutPrefix(name, "mcp__"); ok {
		if _, tool, found := strings.Cut(rest, "__"); found && tool != "" {
			return tool
		}
	}
	return name
}

// shortID is the leading part of an agent or reply id, enough to tell rows
// apart without a 17-character hex string taking the row.
func shortID(id string) string {
	if r := []rune(id); len(r) > 6 {
		return string(r[:6])
	}
	return id
}

// spanText is a duration's text.
func spanText(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d/time.Second)) + "s"
	case d < time.Hour:
		return fmt.Sprintf("%dm %ds", int(d/time.Minute), int(d%time.Minute/time.Second))
	}
	return fmt.Sprintf("%dh %dm", int(d/time.Hour), int(d%time.Hour/time.Minute))
}

// batchSpanText is one batch's duration: tenths of a second under ten
// seconds, since most batches take one or two, else spanText.
func batchSpanText(ms int64) string {
	switch {
	case ms < 100:
		return "<0.1s"
	case ms < 10_000:
		return strconv.FormatFloat(float64(ms)/1000, 'f', 1, 64) + "s"
	}
	return spanText(ms)
}

// compactTokens is a token count in compact form: 950, 1.2k, 12.3k, 210k, 1.4M.
func compactTokens(n int64) string {
	switch {
	case n < 1000:
		return strconv.FormatInt(n, 10)
	case n < 99_950:
		return strconv.FormatFloat(float64(n)/1000, 'f', 1, 64) + "k"
	case n < 999_500:
		return strconv.FormatInt((n+500)/1000, 10) + "k"
	}
	return strconv.FormatFloat(float64(n)/1e6, 'f', 1, 64) + "M"
}

// tokenText is a token usage in one compact line: the model requests it
// covers when more than one, the whole prompt the model read (input plus
// cache reads and writes) with the cache-write part as a trailing +N when
// there was any, then the output.
func tokenText(lang string, requests int, u *wslive.TokenUsage) string {
	if u == nil {
		return ""
	}
	var parts []string
	if requests > 1 {
		parts = append(parts, wsi18n.T(lang, "tok.calls", requests))
	}
	prompt := wsi18n.T(lang, "tok.prompt", compactTokens(u.Input+u.CacheRead+u.CacheWrite))
	if u.CacheWrite > 0 {
		prompt += " +" + compactTokens(u.CacheWrite)
	}
	parts = append(parts, prompt, wsi18n.T(lang, "tok.out", compactTokens(u.Output)))
	return strings.Join(parts, " · ")
}

// tokenTitle is a token usage's exact counts, for a title attribute: every
// part tokenText folds together, named.
func tokenTitle(lang string, requests int, u *wslive.TokenUsage) string {
	if u == nil {
		return ""
	}
	title := wsi18n.T(lang, "tok.title", commafy(u.Input+u.CacheRead+u.CacheWrite),
		commafy(u.Input), commafy(u.CacheRead), commafy(u.CacheWrite), commafy(u.Output))
	if requests > 1 {
		title = wsi18n.T(lang, "tok.title.calls", requests) + " · " + title
	}
	return title
}

// summaryRequests is the model requests a tool summary covers: one per
// batch, the omitted ones included.
func summaryRequests(s *wslive.ToolSummary) int {
	return len(s.Batches) + s.BatchesOmitted
}

// batchNum is one number on a tool summary batch line: its text, a title
// naming it with the exact value, the separator before it, and an optional
// class.
type batchNum struct {
	Sep, Text, Title, Class string
}

// batchNums is a tool summary batch line's trailing numbers, unlabelled:
// its duration, failed calls, and the request's prompt (input plus cache
// reads and writes, the cache-write part as a trailing +N) and output, each
// with its own title. A batch without per-call tools (an older adapter's)
// leads with its call count, which its chips cannot show; so does one whose
// calls outnumber those it named.
func batchNums(lang string, b wslive.ToolBatch) []batchNum {
	var nums []batchNum
	add := func(sep, text, title, class string) {
		if len(nums) == 0 {
			sep = ""
		}
		nums = append(nums, batchNum{Sep: sep, Text: text, Title: title, Class: class})
	}
	named := 0
	for _, t := range b.Tools {
		named += t.Calls
	}
	if named < b.Calls {
		add(" · ", "×"+strconv.Itoa(b.Calls), wsi18n.Plural(lang, "batch.calls", b.Calls), "")
	}
	if b.DurationMs > 0 {
		add(" · ", batchSpanText(b.DurationMs), wsi18n.T(lang, "batch.duration", commafy(b.DurationMs)), "")
	}
	if b.Failed > 0 {
		add(" · ", "✗"+strconv.Itoa(b.Failed), wsi18n.T(lang, "count.failed", b.Failed), "bf")
	}
	if u := b.Usage; u != nil {
		prompt := u.Input + u.CacheRead + u.CacheWrite
		add(" · ", compactTokens(prompt), wsi18n.T(lang, "tok.title.prompt", commafy(prompt), commafy(u.Input), commafy(u.CacheRead), commafy(u.CacheWrite)), "")
		if u.CacheWrite > 0 {
			add(" ", "+"+compactTokens(u.CacheWrite), wsi18n.T(lang, "tok.title.cachewrite", commafy(u.CacheWrite)), "")
		}
		add(" · ", compactTokens(u.Output), wsi18n.T(lang, "tok.out", commafy(u.Output)), "")
	}
	return nums
}

// batchArgs is a one-call batch's argument preview, shown after its tool
// name; empty for a batch of several calls or one that carried none.
func batchArgs(b wslive.ToolBatch) string {
	if b.Calls != 1 || len(b.CallArgs) != 1 {
		return ""
	}
	return b.CallArgs[0].ArgsPreview
}

// batchArgsList is a batch of several calls' "name args" lines, one per
// carried call, for the row's title; empty for a one-call batch (its args
// show inline) or one that carried no argument preview.
func batchArgsList(b wslive.ToolBatch) string {
	if b.Calls < 2 || len(b.CallArgs) == 0 {
		return ""
	}
	lines := make([]string, len(b.CallArgs))
	for i, c := range b.CallArgs {
		lines[i] = strings.TrimSpace(toolName(c.Name) + " " + c.ArgsPreview)
	}
	return strings.Join(lines, "\n")
}
