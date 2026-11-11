package wstracker

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// The search grammar the dashboard's Items search box and tracker.query's q
// payload field share: one parser, so the screen and the tool agree.
//
// A query is whitespace-separated tokens, every token AND-ed. A key:value
// token whose key is one of status, assignee, cc, kind, project, label,
// due, priority, or is is a field filter; any other token, a pasted URL or foo:bar
// included, is free text. A "quoted phrase" is one free-text token, and a
// filter value may be quoted (project:"two words"). Keys and values are
// case-insensitive.
//
//	status:<s>    the item's derived status, or open (not done) or hidden;
//	              one of QueryStatuses
//	assignee:<m>  a live or pending assignee whose member id or display
//	              name contains m
//	cc:<m>        a CC'd member (ItemView.CC) whose member id or display
//	              name contains m; cc:me is the viewer, before any member
//	              whose id or name contains "me" (assignee: has no me)
//	kind:<k>      one of QueryKinds
//	project:<p>   the item's project, exactly
//	label:<l>     a live label of the item named l, exactly (a removed
//	              label matches nothing)
//	due:<op><d>   d is YYYY-MM-DD; op is <, >, <=, >=, or none for equality;
//	              an item with no due date never matches
//	priority:<op><n>  the item's priority level n (0 is the first level)
//	is:claimable  the viewer may claim the item now (ItemView.Claimable): it
//	              is open, the viewer is in its scope and not its fallback
//	              holder; one of QueryIs
//
// Free text matches the item's title, body, project, ID (also in the
// maya.pc-5 form), alias, and the text of the shown comments of its
// thread. A known key with a malformed value is a *QueryError naming the
// token; nothing is filtered then.

// QueryStatuses are the values status: and tracker.query's status filter
// take: open (not done) and hidden, then an item's derived statuses.
var QueryStatuses = []string{"open", "hidden", "idea", "todo", "doing", "blocked", "awaiting", "done", "pending"}

// QueryKinds are the values kind: and tracker.query's kind filter take. A
// todo create folds as a task, so todo is not one.
var QueryKinds = []string{"task", "topic"}

// QueryIs are the values is: takes: properties of the item for the viewer.
var QueryIs = []string{"claimable"}

// QueryError is a query whose Token has a known key and a value that key
// does not take; Reason says what it takes.
type QueryError struct {
	Token  string
	Reason string
}

func (e *QueryError) Error() string {
	return fmt.Sprintf("search %q: %s", e.Token, e.Reason)
}

// Query is a parsed search; the zero Query matches every item.
type Query struct {
	terms []queryTerm
}

// queryTerm is one token: key "" is free text, else a field filter.
type queryTerm struct {
	key   string
	op    string // due and priority: "<", ">", "<=", ">=", or "=" for none
	value string // lower-cased; for priority the decimal level
	num   int
}

// Empty reports a query with no tokens.
func (q Query) Empty() bool { return len(q.terms) == 0 }

// WantsHidden reports a status:hidden token, the one filter a hidden item
// can pass; a caller that leaves hidden items out by default keeps them
// for such a query.
func (q Query) WantsHidden() bool {
	for _, t := range q.terms {
		if t.key == "status" && t.value == "hidden" {
			return true
		}
	}
	return false
}

// ParseQuery parses s by the search grammar.
func ParseQuery(s string) (Query, error) {
	var q Query
	for _, tok := range scanQuery(s) {
		t, err := parseQueryToken(tok)
		if err != nil {
			return Query{}, err
		}
		if t.key != "" || t.value != "" {
			q.terms = append(q.terms, t)
		}
	}
	return q, nil
}

// queryToken is one scanned token: raw as typed, text with the quotes
// removed, and whether it started with a quote (a phrase is never a filter).
type queryToken struct {
	raw, text string
	phrase    bool
}

// scanQuery splits s on whitespace outside quotes. An unclosed quote runs
// to the end.
func scanQuery(s string) []queryToken {
	var out []queryToken
	rs := []rune(s)
	for i := 0; i < len(rs); {
		if unicode.IsSpace(rs[i]) {
			i++
			continue
		}
		start := i
		var text strings.Builder
		quoted := false
		for i < len(rs) && (quoted || !unicode.IsSpace(rs[i])) {
			if rs[i] == '"' {
				quoted = !quoted
			} else {
				text.WriteRune(rs[i])
			}
			i++
		}
		out = append(out, queryToken{raw: string(rs[start:i]), text: text.String(), phrase: rs[start] == '"'})
	}
	return out
}

func parseQueryToken(tok queryToken) (queryTerm, error) {
	free := queryTerm{value: strings.ToLower(tok.text)}
	if tok.phrase {
		return free, nil
	}
	k, v, ok := strings.Cut(tok.text, ":")
	if !ok {
		return free, nil
	}
	key := strings.ToLower(k)
	v = strings.ToLower(strings.TrimSpace(v))
	refuse := func(reason string) (queryTerm, error) {
		return queryTerm{}, &QueryError{Token: tok.raw, Reason: reason}
	}
	switch key {
	case "status":
		if !containsString(QueryStatuses, v) {
			return refuse("status takes one of " + strings.Join(QueryStatuses, ", "))
		}
		return queryTerm{key: key, value: v}, nil
	case "kind":
		if !containsString(QueryKinds, v) {
			return refuse("kind takes one of " + strings.Join(QueryKinds, ", "))
		}
		return queryTerm{key: key, value: v}, nil
	case "is":
		if !containsString(QueryIs, v) {
			return refuse("is takes one of " + strings.Join(QueryIs, ", "))
		}
		return queryTerm{key: key, value: v}, nil
	case "assignee", "cc", "project", "label":
		if v == "" {
			return refuse(key + " takes a value")
		}
		return queryTerm{key: key, value: v}, nil
	case "due":
		op, d := cutQueryOp(v)
		if !validDue(d) {
			return refuse("due takes a date YYYY-MM-DD, optionally after <, >, <=, or >=")
		}
		return queryTerm{key: key, op: op, value: d}, nil
	case "priority":
		op, n := cutQueryOp(v)
		lv, err := strconv.Atoi(n)
		if err != nil || lv < 0 || n != strconv.Itoa(lv) {
			return refuse("priority takes a level number, optionally after <, >, <=, or >=")
		}
		return queryTerm{key: key, op: op, value: n, num: lv}, nil
	}
	return free, nil
}

// cutQueryOp splits a comparison operator off v; none is "=".
func cutQueryOp(v string) (op, rest string) {
	for _, p := range []string{"<=", ">=", "<", ">"} {
		if r, ok := strings.CutPrefix(v, p); ok {
			return p, r
		}
	}
	return "=", v
}

func containsString(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// compareQuery reports c (the item's value compared with the token's: -1,
// 0, 1) passing op.
func compareQuery(op string, c int) bool {
	switch op {
	case "<":
		return c < 0
	case ">":
		return c > 0
	case "<=":
		return c <= 0
	case ">=":
		return c >= 0
	}
	return c == 0
}

// Match reports whether item id of v passes every token of q. A query with
// no status token keeps hidden items; the caller decides whether to show
// them.
func (q Query) Match(v *View, id string) bool {
	it, ok := v.Items[id]
	if !ok {
		return false
	}
	for _, t := range q.terms {
		if !t.match(v, it) {
			return false
		}
	}
	return true
}

func (t queryTerm) match(v *View, it ItemView) bool {
	status := ""
	if it.Status != nil {
		status = *it.Status
	}
	switch t.key {
	case "status":
		switch t.value {
		case "hidden":
			return it.Hidden
		case "open":
			return !it.Hidden && status != "done"
		}
		return !it.Hidden && status == t.value
	case "kind":
		return it.Kind == t.value
	case "is":
		return it.Claimable
	case "project":
		return strings.ToLower(it.Project) == t.value
	case "label":
		for _, id := range it.Labels {
			for _, l := range v.Labels {
				if l.ID == id && strings.ToLower(l.Name) == t.value {
					return true
				}
			}
		}
		return false
	case "assignee":
		for _, aid := range it.Assignments {
			a, ok := v.Assignments[aid]
			if !ok || (a.State != "live" && a.State != "pending") {
				continue
			}
			if strings.Contains(strings.ToLower(a.To), t.value) || strings.Contains(strings.ToLower(v.Members[a.To].Name), t.value) {
				return true
			}
		}
		return false
	case "cc":
		if t.value == "me" {
			return containsString(it.CC, v.Viewer.Member)
		}
		for _, m := range it.CC {
			if strings.Contains(strings.ToLower(m), t.value) || strings.Contains(strings.ToLower(v.Members[m].Name), t.value) {
				return true
			}
		}
		return false
	case "due":
		if it.Due == nil || len(*it.Due) < 10 {
			return false
		}
		return compareQuery(t.op, strings.Compare((*it.Due)[:10], t.value))
	case "priority":
		c := 0
		if it.PriorityLevel < t.num {
			c = -1
		} else if it.PriorityLevel > t.num {
			c = 1
		}
		return compareQuery(t.op, c)
	}
	for _, s := range []string{it.Title, it.Body, it.Project, it.ID, strings.Replace(it.ID, "#", "-", 1), it.Alias} {
		if strings.Contains(strings.ToLower(s), t.value) {
			return true
		}
	}
	for _, cid := range it.Thread {
		if c, ok := v.Comments[cid]; ok && c.Hidden == nil && strings.Contains(strings.ToLower(c.Body), t.value) {
			return true
		}
	}
	return false
}

// Search returns the IDs of the items of v that pass q, in tree order
// (ItemOrder), hidden items included when they pass.
func (v *View) Search(q Query) []string {
	out := []string{}
	for _, id := range v.ItemOrder {
		if q.Match(v, id) {
			out = append(out, id)
		}
	}
	return out
}
