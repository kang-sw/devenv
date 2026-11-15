package wswhiteboard

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// An extension is a revision of the same run, not a new execution start.
// Validate against the immediately preceding authority, not just its snapshot.
func validExtension(prior TopicStateView, r topicStateRecord) bool {
	if prior.State == nil || prior.State.Value != StateImplementing || prior.State.Run == nil {
		return false
	}
	a, b := prior.State.Run, r.Run
	return b != nil && a.ID == b.ID && a.StartedAt == b.StartedAt && a.Owner == b.Owner && a.InitialETAMinutes == b.InitialETAMinutes && b.ETAMinutes > a.ETAMinutes && b.ExtensionCount == a.ExtensionCount+1
}

func applyTopicETA(p *problems, work *State, cmd command, topicOf topicResolver, now time.Time) {
	before := len(p.List)
	topic, ok := topicOf(cmd)
	text, _ := cmd.Attr("minutes")
	minutes, err := strconv.ParseInt(text, 10, 64)
	if text == "" || strings.ContainsFunc(text, func(r rune) bool { return r < '0' || r > '9' }) || err != nil || minutes < 1 || minutes > maxETAMinutes {
		p.Add("%s: required positive decimal integer, at most %d", cmd.At("minutes"), maxETAMinutes)
	}
	reason, _ := cmd.Attr("reason")
	reason = strings.TrimSpace(reason)
	if !validStateText(reason, 2048) {
		p.Add("%s: required, 1-2048 UTF-8 bytes after trimming, without control characters", cmd.At("reason"))
	}
	if !ok {
		return
	}
	if !work.topicKnown(topic) || work.topicClosed(topic) {
		p.Add("%s: ETA requires a known open implementing topic", cmd.Label())
		return
	}
	view := work.TopicStates()[topic]
	if view.State == nil || view.State.Value != StateImplementing || view.State.Run == nil {
		p.Add("%s: ETA requires known implementing state", cmd.Label())
		return
	}
	if len(p.List) != before {
		return
	}
	run := *view.State.Run
	if minutes <= run.ETAMinutes {
		p.Add("%s: total ETA must be strictly greater than %d minutes", cmd.At("minutes"), run.ETAMinutes)
		return
	}
	start, _ := time.Parse(time.RFC3339Nano, run.StartedAt)
	if _, ok := stateDeadline(start, minutes); !ok {
		p.Add("%s: deadline is outside UTC RFC3339Nano bounds", cmd.Label())
		return
	}
	if _, ok := stateTimestamp(now); !ok {
		p.Add("%s: server time is outside UTC RFC3339Nano bounds", cmd.Label())
		return
	}
	run.ETAMinutes, run.ExtensionCount = minutes, run.ExtensionCount+1
	r := topicStateRecord{V: 1, Op: "extend", Run: &run, Reason: reason}
	work.appendStateEvent(topic, r, fmt.Sprintf("Topic ETA extended: total %d minutes (initial %d); extension %d; reason: %s", minutes, run.InitialETAMinutes, run.ExtensionCount, reason), now)
}
