// Conversation and index views project their records for pages.

package wsdashboard

import (
	"html/template"
	"path/filepath"
	"sort"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// replyRender renders lead replies as markdown: finished a reply entry,
// streaming the reply still streaming.
type replyRender struct {
	finished, streaming func(string) template.HTML
}

// replyMarkdown renders lead replies under the served page's link rule
// (servedTargets): a file target is an action link for the page's link
// menu, and an image inside root is the image route's URL. A finished
// reply's inline HTML is kept through the reply sanitizer under the same
// rule (wswhiteboard.TextTargets.HTML) and its mermaid fences are drawn by
// the page; the streaming reply shows raw HTML as text and keeps mermaid
// fences code blocks, since it re-renders every tick and a half-closed tag
// would shake the layout and a diagram would redraw with it.
func replyMarkdown(root, replyID string) replyRender {
	return replyRenderUnder(servedTargets(root, replyID))
}

// replyRenderUnder is replyMarkdown under the link rule targets.
func replyRenderUnder(targets wswhiteboard.TextTargets) replyRender {
	streaming, finished := targets, targets
	finished.Mermaid = true
	finished.HTML = true
	return replyRender{
		finished:  func(s string) template.HTML { return wswhiteboard.RenderText(s, finished) },
		streaming: func(s string) template.HTML { return wswhiteboard.RenderText(s, streaming) },
	}
}

// conversation turns the ring into the page's stream: each mail line gets
// its body while the mailbox history still holds it and a link to its peer's
// page when this daemon serves the peer. A lead reply's markdown is left to
// the entry render (renderEntries), which caches it.
func conversation(lead wslive.Lead, ring []wslive.RingRecord, bodies map[string]string, contacts []Contact, lookup sessionLookup, render replyRender, self string) ConversationView {
	byAddress := map[string]string{}
	for _, c := range contacts {
		if c.ReplyID != "" {
			byAddress[c.Address] = c.ReplyID
		}
	}
	cv := ConversationView{Reply: lead.Reply, Phase: lead.Phase, Permission: lead.Permission, Summary: liveSummary(lead.ReasoningSummary), markdown: render.finished}
	if lead.Reply != nil {
		cv.ReplyHTML = render.streaming(lead.Reply.Text)
	}
	_, cv.Running, cv.Generating = splitBatches(lead.Batches)
	cv.ToolGap = lead.ToolGap
	for _, r := range ring {
		e := ConvEntry{RingEntry: r.RingEntry, Key: r.Name, At: r.At.UnixMilli()}
		switch r.Kind {
		case wslive.RingUser:
			cv.Users++
		case wslive.RingMail:
			cv.Mails++
			if r.HistoryKey != "" {
				e.Body, e.HasBody = bodies[r.HistoryKey]
			}
			rid := byAddress[r.Peer]
			if rid == "" && hasReplyIDPrefix(r.Peer) {
				rid = r.Peer[len(wsmailbox.ReplyIDPrefix):]
			}
			if rid != "" {
				if s, ok := lookup.get(rid); ok {
					e.PeerReplyID, e.PeerLinked = rid, true
					if hasReplyIDPrefix(r.Peer) {
						e.PeerLabel = s.Label
					}
				}
			}
		}
		cv.Entries = append(cv.Entries, e)
	}
	linkMailWakes(cv.Entries, self)
	collapseBoardNudges(cv.Entries)
	return cv
}

// IndexView is what / renders.
type IndexView struct {
	// Lang, LangPref, and Theme are SessionView's.
	Lang, LangPref, Theme string
	Version               string
	Projects              []ProjectGroup
	// Live and Busy count the live sessions and those not idle.
	Live, Busy int
}

// ProjectGroup is one project root's sessions on /.
type ProjectGroup struct {
	Name, Path string
	// Sessions are the live sessions; Ended the ended ones still inside
	// endedSessionWindow; HiddenEnded counts the older ended ones.
	Sessions    []IndexRow
	Ended       []IndexRow
	HiddenEnded int
}

// IndexRow is one session on /.
type IndexRow struct {
	ReplyID string
	// Label is the session's display label, shown in place of ReplyID;
	// empty when it has none.
	Label   string
	Harness string
	Live    bool
	// Adapter, Phase, and Tool come from a live snapshot; Phase is empty
	// without an adapter.
	Adapter       bool
	Phase         string
	Tool          string
	RunningAgents int
	HasBoard      bool
	CountPolicy   int
	// LastSeen is the registration's latest stamp (epoch ms).
	LastSeen int64
	// Hosted and Approval are the rail row's (RailRow).
	Hosted   bool
	Exit     *HostExit
	Approval *ApprovalWait
}

// indexInput is one indexed session with what / shows of it.
type indexInput struct {
	Session  Session
	Snapshot *wslive.Snapshot
}

// buildIndexView groups sessions by project. A live session is listed; an
// ended one only while it was last seen within endedSessionWindow.
func buildIndexView(sessions []indexInput, now time.Time, version string) IndexView {
	view := IndexView{Version: version}
	byProject := map[string]*ProjectGroup{}
	var order []string
	for _, in := range sessions {
		reg := in.Session.Registration
		group, ok := byProject[reg.Project]
		if !ok {
			group = &ProjectGroup{Name: filepath.Base(reg.Project), Path: reg.Project}
			byProject[reg.Project] = group
			order = append(order, reg.Project)
		}
		row := IndexRow{
			ReplyID:     reg.ReplyID,
			Label:       in.Session.Label,
			Harness:     reg.Harness,
			Live:        reg.Live(now),
			HasBoard:    in.Session.HasBoard,
			CountPolicy: in.Session.CountPolicy,
			LastSeen:    reg.LastSeen().UnixMilli(),
			Hosted:      reg.Hosted,
			Exit:        hostExit(reg),
		}
		if row.Exit == nil && adapterPresent(in.Snapshot, now) {
			row.Adapter = true
			if row.Live {
				row.Approval = approvalWait(reg, in.Snapshot)
			}
			row.Phase = in.Snapshot.Lead.Phase
			if row.Phase == "" {
				row.Phase = wslive.PhaseIdle
			}
			if _, running, generating := splitBatches(in.Snapshot.Lead.Batches); running != nil {
				row.Tool = running.ToolName
			} else if generating != nil {
				row.Tool = generating.ToolName
			}
			if p := in.Snapshot.Lead.Permission; p != nil {
				row.Tool = p.ToolName
			}
			for _, a := range in.Snapshot.Agents {
				if a.Status == wslive.StatusRunning {
					row.RunningAgents++
				}
			}
		}
		switch {
		case row.Live:
			group.Sessions = append(group.Sessions, row)
			view.Live++
			if row.Adapter && row.Phase != wslive.PhaseIdle {
				view.Busy++
			}
		case now.Sub(reg.LastSeen()) <= endedSessionWindow:
			group.Ended = append(group.Ended, row)
		default:
			group.HiddenEnded++
		}
	}
	sort.Strings(order)
	for _, path := range order {
		group := byProject[path]
		newest := func(list []IndexRow) {
			sort.SliceStable(list, func(i, j int) bool { return list[i].LastSeen > list[j].LastSeen })
		}
		newest(group.Sessions)
		newest(group.Ended)
		if len(group.Sessions) == 0 && len(group.Ended) == 0 {
			continue // only long-ended sessions: the project leaves /
		}
		view.Projects = append(view.Projects, *group)
	}
	return view
}
