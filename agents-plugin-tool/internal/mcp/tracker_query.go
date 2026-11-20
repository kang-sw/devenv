// Tracker query parsing and report formatting.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

type trackerQuery struct {
	store, scope, item          string
	kind, status, project, text string
	// q is the payload's search text, parsed into search by the
	// dashboard search box's grammar (wstracker.ParseQuery).
	q      string
	search wstracker.Query
	json   bool
}

var (
	trackerScopes = []string{"mine", "received", "assigned", "team", "related", "all"}
	// todo is not a filter value: the store folds a todo create as a task.
	trackerKinds = wstracker.QueryKinds
	// trackerStatuses are the status filter's values, shared with the
	// search grammar's status: key: open (neither done nor dropped) and
	// hidden, then an item's derived statuses.
	trackerStatuses = wstracker.QueryStatuses
)

// parseTrackerQuery reads the payload object and format argument.
func parseTrackerQuery(rawPayload, rawFormat any) (trackerQuery, []string) {
	var q trackerQuery
	var problems []string
	add := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }
	if rawFormat != nil {
		switch rawFormat {
		case "json":
			q.json = true
		case "", "text":
		default:
			add("format: must be \"json\" or omitted; got %v", rawFormat)
		}
	}
	payload := map[string]any{}
	if rawPayload != nil {
		m, ok := rawPayload.(map[string]any)
		if !ok {
			add("payload: must be an object")
		}
		payload = m
	}
	fields := map[string]*string{"store": &q.store, "scope": &q.scope, "item": &q.item, "kind": &q.kind, "status": &q.status, "project": &q.project, "text": &q.text, "q": &q.q}
	names := make([]string, 0, len(payload))
	for name := range payload {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dst, known := fields[name]
		if !known {
			add("payload.%s: unknown (known: store, scope, item, kind, status, project, text, q)", name)
			continue
		}
		v, ok := payload[name].(string)
		if !ok {
			add("payload.%s: must be a string", name)
			continue
		}
		*dst = strings.TrimSpace(v)
	}
	enum := func(name, v string, values []string) {
		if v == "" {
			return
		}
		for _, allowed := range values {
			if v == allowed {
				return
			}
		}
		add("payload.%s: must be one of %s; got %q", name, strings.Join(values, ", "), v)
	}
	enum("scope", q.scope, trackerScopes)
	enum("kind", q.kind, trackerKinds)
	if st, ok := wstracker.NormQueryStatus(q.status); ok {
		q.status = st // awaiting, the older name of in-review, is its alias
	} else {
		enum("status", q.status, trackerStatuses)
	}
	if search, err := wstracker.ParseQuery(q.q); err != nil {
		add("payload.q: %v", err)
	} else {
		q.search = search
	}
	narrowed := q.scope != "" || q.item != "" || q.filtered()
	switch {
	case q.store == "" && narrowed:
		add("payload: scope, item, and the filters need store")
	case q.item != "" && (q.scope != "" || q.filtered()):
		add("payload.item: shows one item; drop scope and the filters")
	case q.json && narrowed:
		add("format: json returns the store's whole view state; drop scope, item, and the filters")
	}
	if q.item != "" {
		id, err := parseTrackerID(q.item)
		if err != nil {
			add("payload.item: must be an item ID such as maya.pc-5; got %q", q.item)
		}
		q.item = id
	}
	if q.scope == "" {
		q.scope = "mine"
	}
	return q, problems
}

// filtered reports a query with any item filter: kind, status, project,
// text, or q.
func (q trackerQuery) filtered() bool {
	return q.kind != "" || q.status != "" || q.project != "" || q.text != "" || q.q != ""
}

// trackerStoreSummary is one row of the store list.
type trackerStoreSummary struct {
	Entry wstrackerhost.StoreEntry
	// View is nil when the store could not be read; Err says why.
	View *wstrackerhost.StoreView
	Err  error
}

func (s *Server) handleTrackerQuery(ctx context.Context, id json.RawMessage, args map[string]any) response {
	const tool = "tracker.query"
	key, _, err := s.trackerSessionKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	q, problems := parseTrackerQuery(args["payload"], args["format"])
	if len(problems) > 0 {
		return toolErrorTextResponse(id, s.trackerRefusal(tool, problems))
	}
	h, err := s.trackerHost()
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	registered, err := h.Stores()
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	if q.store == "" {
		profiles, err := h.Profiles()
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
		}
		summaries := make([]trackerStoreSummary, 0, len(registered))
		for _, e := range registered {
			sum := trackerStoreSummary{Entry: e}
			if sum.Err = h.PreRead(ctx, e.ID); sum.Err == nil {
				sum.View, sum.Err = h.View(ctx, e.ID, 0)
			}
			summaries = append(summaries, sum)
		}
		if q.json {
			resp := toolJSONResponse(id, trackerStoreListJSON(summaries, profiles), nil)
			// JSON output carries no warning line; the knob write is best effort.
			_ = s.trackerSucceeded(key, "")
			return resp
		}
		return toolTextResponse(id, s.trackerSucceeded(key, formatTrackerStoreList(summaries, profiles)), nil)
	}
	found := false
	for _, e := range registered {
		found = found || e.ID == q.store
	}
	if !found {
		ids := make([]string, 0, len(registered))
		for _, e := range registered {
			ids = append(ids, e.ID)
		}
		reason := "no store is registered"
		if len(ids) > 0 {
			reason = "registered: " + strings.Join(ids, ", ")
		}
		return toolErrorTextResponse(id, s.trackerRefusal(tool, []string{fmt.Sprintf("payload.store: %q is not a registered store (%s)", q.store, reason)}))
	}
	if err := h.PreRead(ctx, q.store); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %s", tool, wstrackerhost.RedactCredentials(err.Error())))
	}
	v, err := h.View(ctx, q.store, 0)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %s", tool, wstrackerhost.RedactCredentials(err.Error())))
	}
	v.Redact()
	switch {
	case q.json:
		resp := toolJSONResponse(id, v, nil)
		_ = s.trackerSucceeded(key, "") // as for the JSON store list
		return resp
	case q.item != "":
		if _, ok := v.Items[q.item]; !ok {
			return toolErrorTextResponse(id, s.trackerRefusal(tool, []string{fmt.Sprintf("payload.item: %s is not an item of %s", trackerAlias(q.item), q.store)}))
		}
		return toolTextResponse(id, s.trackerSucceeded(key, formatTrackerItem(v, q.item)), nil)
	default:
		return toolTextResponse(id, s.trackerSucceeded(key, formatTrackerScope(v, q)), nil)
	}
}

// trackerRefusal lists every problem, then the tool's input grammar from
// the tracker playbook, so one retry can fix the call: for tracker.update
// the shared command syntax and the "## Commands" section (commandRefusal),
// for tracker.query the "## Query" section.
func (s *Server) trackerRefusal(tool string, problems []string) string {
	if tool != "tracker.query" {
		return s.commandRefusal(tool, problems, trackerPlaybookName)
	}
	var sb strings.Builder
	writeRefusalProblems(&sb, tool, problems)
	if grammar := s.trackerPlaybookSection("## Query"); grammar != "" {
		sb.WriteString("\n" + grammar + "\n")
	} else {
		fmt.Fprintf(&sb, "\nRead the %s playbook with %s/playbook.read for the input format.\n", trackerPlaybookName, s.runtimeNamespace())
	}
	return sb.String()
}

// trackerPlaybookSection returns the tracker playbook's section under
// heading; "" when it cannot be read.
func (s *Server) trackerPlaybookSection(heading string) string {
	rsrcRoot, err := s.resolveRsrcRoot("")
	if err != nil {
		return ""
	}
	body, _, err := renderPlaybookBody(s, rsrcRoot, trackerPlaybookName, nil, s.configOptions(), "", "", "", nil, "")
	if err != nil {
		return ""
	}
	return markdownSection(body, heading)
}

// trackerManualSection is the workflow manual's knob-on tracker section:
// the tracker playbook's content under "### Tracker" (its title dropped,
// its headings demoted under the section), so the lead needs no extra read.
func trackerManualSection(s *Server, rsrcRoot fs.FS) (string, error) {
	playbook, _, err := renderPlaybookBody(s, rsrcRoot, trackerPlaybookName, nil, s.configOptions(), "", "", "", nil, "")
	if err != nil {
		return "", fmt.Errorf("render %s playbook: %w", trackerPlaybookName, err)
	}
	var sb strings.Builder
	sb.WriteString("### Tracker\n\n")
	inFence := false
	for _, line := range strings.Split(strings.TrimSpace(playbook), "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
		}
		if !inFence {
			if strings.HasPrefix(line, "# ") {
				continue
			}
			if strings.HasPrefix(line, "## ") {
				line = "##" + line
			}
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// trackerStoreListJSON is the store list's format json shape.
func trackerStoreListJSON(summaries []trackerStoreSummary, profiles wstrackerhost.SavedProfiles) map[string]any {
	stores := make([]map[string]any, 0, len(summaries))
	for _, sum := range summaries {
		e := sum.Entry
		row := map[string]any{"id": e.ID, "label": e.DisplayLabel(), "member": e.Member, "remote": nil}
		if e.Remote != "" {
			row["remote"] = wstrackerhost.RedactCredentials(e.Remote)
		}
		if e.Hidden {
			row["hidden"] = true
		}
		if sum.View != nil {
			sum.View.Redact()
			row["sync"] = sum.View.Sync
			row["today"] = sum.View.My.Today
		} else if sum.Err != nil {
			row["error"] = wstrackerhost.RedactCredentials(sum.Err.Error())
		}
		stores = append(stores, row)
	}
	return map[string]any{"stores": stores, "profiles": profiles}
}

// formatTrackerStoreList is the store list's text: one block per store
// (its id and label, member, whether it is hidden, remote and sync state, and the user's open
// work counts), then the saved profiles, the one used last first.
func formatTrackerStoreList(summaries []trackerStoreSummary, profiles wstrackerhost.SavedProfiles) string {
	var sb strings.Builder
	if len(summaries) == 0 {
		sb.WriteString("stores: none registered\n")
	} else {
		sb.WriteString("stores:\n")
	}
	for _, sum := range summaries {
		e := sum.Entry
		fmt.Fprintf(&sb, "- %s", e.ID)
		if e.Label != "" {
			fmt.Fprintf(&sb, " (%s)", e.Label)
		}
		fmt.Fprintf(&sb, ", member %s", e.Member)
		if e.Hidden {
			// Hidden only from the dashboard's store list; still read here.
			sb.WriteString(", hidden on the dashboard")
		}
		if sum.View == nil {
			fmt.Fprintf(&sb, ", unreadable: %s\n", wstrackerhost.RedactCredentials(fmt.Sprint(sum.Err)))
			continue
		}
		sum.View.Redact()
		fmt.Fprintf(&sb, ", %s\n", formatTrackerSyncLine(sum.View))
		t := sum.View.My.Today
		fmt.Fprintf(&sb, "  my work %d, to answer %d, to confirm %d\n", len(t.Work), len(t.Pending), len(t.Confirms))
	}
	if len(profiles.Profiles) == 0 {
		sb.WriteString("profiles: none saved\n")
		return sb.String()
	}
	sb.WriteString("profiles:\n")
	if last, ok := profiles.LastProfile(); ok {
		fmt.Fprintf(&sb, "- %s (used last)\n", formatTrackerProfile(last))
	}
	for i, p := range profiles.Profiles {
		if i != profiles.Last {
			fmt.Fprintf(&sb, "- %s\n", formatTrackerProfile(p))
		}
	}
	return sb.String()
}

// formatTrackerSyncLine is a store's remote and sync state on one line.
func formatTrackerSyncLine(v *wstrackerhost.StoreView) string {
	if v.Store.Remote == nil {
		// A store with no remote has nowhere to push, so no unpushed count.
		return "no remote"
	}
	parts := []string{"remote " + *v.Store.Remote, "offline"}
	if v.Sync.Online {
		parts[1] = "online"
	}
	if v.Sync.SyncedAt != nil {
		parts = append(parts, "synced "+*v.Sync.SyncedAt)
	} else {
		parts = append(parts, "never synced")
	}
	if v.Sync.Error != "" {
		parts = append(parts, "last error: "+v.Sync.Error)
	}
	unpushed := 0
	for _, c := range v.Sync.Unpushed {
		unpushed += len(c.Events)
	}
	if unpushed > 0 {
		parts = append(parts, fmt.Sprintf("unpushed writes %d", unpushed))
	}
	return strings.Join(parts, ", ")
}

// formatTrackerProfile is a profile as one line: org, then the fields.
func formatTrackerProfile(p wstracker.Profile) string {
	s := fmt.Sprintf("given=%q", p.Given)
	if p.Family != "" {
		s = fmt.Sprintf("family=%q ", p.Family) + s
	}
	if p.Org != "" {
		s = fmt.Sprintf("org=%q ", p.Org) + s
	}
	return s
}
