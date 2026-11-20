package wsdashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

const hudMaxDepth = 4

// HUDView is the serialized hierarchical org-chart projection embedded in
// #r-tree .ag-source for the floating compact HUD.
type HUDView struct {
	Lead      string    `json:"lead,omitempty"`
	Completed int       `json:"completed"`
	Peers     int       `json:"peers"`
	Roots     []HUDNode `json:"roots"`
}

// HUDNode is one node in the compact horizontal org-chart HUD.
type HUDNode struct {
	ID          string    `json:"id"`
	ReplyID     string    `json:"replyId,omitempty"`
	Linked      bool      `json:"linked,omitempty"`
	Label       string    `json:"label"`
	Harness     string    `json:"harness"`
	State       string    `json:"state"`
	Model       string    `json:"model,omitempty"`
	Effort      string    `json:"effort,omitempty"`
	ElapsedMs   int64     `json:"elapsedMs,omitempty"`
	ActiveSince int64     `json:"activeSince,omitempty"`
	Chain       []string  `json:"chain,omitempty"`
	Task        string    `json:"task,omitempty"`
	Overflow    int       `json:"overflow,omitempty"`
	Children    []HUDNode `json:"children,omitempty"`
	// OwnRow marks nodes rendered in #r-tree's .agent list so hudJSON can omit
	// their Task from .ag-source (keeping tree-rows shell invariant on caption edits).
	OwnRow bool `json:"-"`
}

type hudRawNode struct {
	agent    wslive.Agent
	linked   bool
	ownRow   bool
	children []*hudRawNode
}

// hudProjection builds the compact HUD forest from the session's own agents and
// any live linked delegate subtrees, keeping own-snapshot ParentID placement
// authoritative on duplicate agent IDs and capping depth at hudMaxDepth.
func hudProjection(self, leadLabel, leadHarness string, agents []wslive.Agent, peers int, lookup sessionLookup, now time.Time) HUDView {
	out := HUDView{
		Lead:  leadLabel,
		Peers: peers,
		Roots: []HUDNode{},
	}
	seenIDs := map[string]bool{}
	roots := buildHUDRawForest(agents, true, seenIDs, lookup)
	visitedSessions := map[string]bool{self: true}
	expandHUDLinked(roots, seenIDs, visitedSessions, lookup, now)

	visible := map[*hudRawNode]bool{}
	var mark func(*hudRawNode) bool
	mark = func(n *hudRawNode) bool {
		st := hudNodeState(n.agent)
		vis := st != "idle" || n.agent.Alive
		for _, c := range n.children {
			if mark(c) {
				vis = true
			}
		}
		visible[n] = vis
		return vis
	}
	for _, r := range roots {
		mark(r)
	}
	out.Roots = projectHUDNodes(roots, 1, normalizeHUDHarness(leadHarness), nil, visible, &out.Completed, now)
	return out
}

func buildHUDRawForest(agents []wslive.Agent, ownRow bool, seenIDs map[string]bool, lookup sessionLookup) []*hudRawNode {
	present := map[string]bool{}
	for _, a := range agents {
		if !seenIDs[a.ID] {
			present[a.ID] = true
		}
	}
	children := map[string][]wslive.Agent{}
	var roots []wslive.Agent
	for _, a := range agents {
		if seenIDs[a.ID] {
			continue
		}
		if a.ParentID != "" && a.ParentID != a.ID && present[a.ParentID] {
			children[a.ParentID] = append(children[a.ParentID], a)
		} else {
			roots = append(roots, a)
		}
	}
	var build func([]wslive.Agent) []*hudRawNode
	build = func(list []wslive.Agent) []*hudRawNode {
		var out []*hudRawNode
		for _, a := range list {
			if seenIDs[a.ID] {
				continue
			}
			seenIDs[a.ID] = true
			node := &hudRawNode{agent: a, ownRow: ownRow}
			if a.ReplyID != "" {
				if s, ok := lookup.get(a.ReplyID); ok && (s.HasBoard || s.Agent) {
					node.linked = true
				}
			}
			node.children = build(children[a.ID])
			out = append(out, node)
		}
		return out
	}
	return build(roots)
}

func expandHUDLinked(nodes []*hudRawNode, seenIDs, visited map[string]bool, lookup sessionLookup, now time.Time) {
	for _, n := range nodes {
		expandHUDLinked(n.children, seenIDs, visited, lookup, now)
		rid := n.agent.ReplyID
		if rid == "" || visited[rid] || !hudShouldExpandLinked(n.agent) {
			continue
		}
		visited[rid] = true
		if _, ok := lookup.get(rid); !ok {
			continue
		}
		if snap := lookup.snap(rid); adapterPresent(snap, now) {
			attached := buildHUDRawForest(snap.Agents, false, seenIDs, lookup)
			expandHUDLinked(attached, seenIDs, visited, lookup, now)
			n.children = append(n.children, attached...)
		}
	}
}

func hudShouldExpandLinked(a wslive.Agent) bool {
	return a.Status == wslive.StatusRunning || a.Holding || a.Alive
}

func projectHUDNodes(nodes []*hudRawNode, depth int, parentHarness string, chain []string, visible map[*hudRawNode]bool, completed *int, now time.Time) []HUDNode {
	out := []HUDNode{}
	for _, n := range nodes {
		if !visible[n] {
			*completed += countHUDRawSubtree(n)
			continue
		}
		a := n.agent
		harness := resolveHUDHarness(a, parentHarness)
		label := a.Label
		if label == "" {
			label = shortID(a.ID)
		}
		an := AgentNode{Agent: a}
		node := HUDNode{
			ID:          a.ID,
			ReplyID:     a.ReplyID,
			Linked:      n.linked,
			Label:       label,
			Harness:     harness,
			State:       hudNodeState(a),
			Model:       an.DisplayModel(),
			Effort:      a.Effort,
			ElapsedMs:   max(a.ActiveMs, 0),
			ActiveSince: an.ActiveSince(),
			Chain:       slices.Clone(chain),
			Task:        hudNodeTask(a),
			OwnRow:      n.ownRow,
		}
		if depth < hudMaxDepth {
			nextChain := append(slices.Clone(chain), label)
			kids := projectHUDNodes(n.children, depth+1, harness, nextChain, visible, completed, now)
			if len(kids) > 0 {
				node.Children = kids
			}
		} else {
			node.Overflow = foldHUDDepthOverflow(n.children, visible, completed)
		}
		out = append(out, node)
	}
	return out
}

func countHUDRawSubtree(n *hudRawNode) int {
	total := 1
	for _, c := range n.children {
		total += countHUDRawSubtree(c)
	}
	return total
}

func foldHUDDepthOverflow(nodes []*hudRawNode, visible map[*hudRawNode]bool, completed *int) int {
	overflow := 0
	for _, n := range nodes {
		if visible[n] {
			overflow++
		} else {
			*completed++
		}
		overflow += foldHUDDepthOverflow(n.children, visible, completed)
	}
	return overflow
}

func hudNodeState(a wslive.Agent) string {
	switch {
	case a.Permission != nil:
		return "permission"
	case a.Status == wslive.StatusFailed:
		return "failed"
	case a.Status == wslive.StatusRunning:
		if a.Phase == wslive.PhaseTool || a.Phase == wslive.PhaseReplying {
			return "tool"
		}
		return "thinking"
	case a.Holding:
		return "holding"
	default:
		return "idle"
	}
}

func normalizeHUDHarness(h string) string {
	switch strings.ToLower(strings.TrimSpace(h)) {
	case "claude", "codex", "agy", "pi":
		return strings.ToLower(strings.TrimSpace(h))
	default:
		return ""
	}
}

func resolveHUDHarness(a wslive.Agent, parentHarness string) string {
	if h := normalizeHUDHarness(a.Harness); h != "" {
		return h
	}
	if parentHarness != "" {
		return parentHarness
	}
	if a.AgentKind == wslive.AgentKindPi {
		return "pi"
	}
	return ""
}

func hudNodeTask(a wslive.Agent) string {
	if a.Permission != nil {
		return formatHUDToolTask(a.Permission.ToolName, a.Permission.ArgsPreview)
	}
	_, running, generating := splitBatches(a.Batches)
	if running != nil {
		return formatHUDToolTask(running.ToolName, running.ArgsPreview)
	}
	if generating != nil && generating.ToolName != "" {
		return toolName(generating.ToolName)
	}
	return a.Caption
}

func formatHUDToolTask(name, args string) string {
	short := toolName(name)
	args = strings.TrimSpace(args)
	if args == "" {
		return short
	}
	if short == "" {
		return args
	}
	return short + " " + args
}
