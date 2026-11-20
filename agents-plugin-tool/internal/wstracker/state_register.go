package wstracker

import "strings"

// ---------------------------------------------------------------- markers

type marker struct {
	kind, field, other, winner string
	mine, theirs               string
}

type supersession struct {
	field, by, why string
}

func mark(n, other *node, kind, field, winner, mine, theirs string) {
	for _, x := range n.markers {
		if x.other == other.id && x.field == field {
			return
		}
	}
	n.markers = append(n.markers, marker{kind: kind, field: field, other: other.id, winner: winner, mine: mine, theirs: theirs})
}

func supersede(n *node, field string, by *node, why string) {
	kept := n.superseded[:0:0]
	for _, x := range n.superseded {
		if x.field != field {
			kept = append(kept, x)
		}
	}
	if by != nil {
		kept = append(kept, supersession{field: field, by: by.id, why: why})
	}
	n.superseded = kept
}

// show renders a register value for a marker.
func show(v any) string {
	switch x := v.(type) {
	case nil:
		return "(none)"
	case priority:
		return strings.Join(x.names, " / ") + " (default " + x.names[x.def] + ")"
	case labelSet:
		if len(x) == 0 {
			return "(none)"
		}
		out := make([]string, len(x))
		for i, id := range x {
			out[i] = alias(id)
		}
		return strings.Join(out, ", ")
	case ccVal:
		if x {
			return "CC'd"
		}
		return "not CC'd"
	case statusVal:
		if x.note != "" {
			return x.status + ` ("` + x.note + `")`
		}
		return x.status
	case []string:
		if len(x) == 0 {
			return "nobody"
		}
		return strings.Join(x, ", ")
	case bool:
		if x {
			return "hidden"
		}
		return "false"
	case placeVal:
		at := "top level"
		if x.parent != "" {
			at = "under " + alias(x.parent)
		}
		switch {
		case !x.afterSet:
			return at + ", last"
		case x.after == "":
			return at + ", first"
		}
		return at + ", after " + alias(x.after)
	case int:
		return itoa(x)
	case string:
		return truncate60(x)
	}
	return ""
}

// winner picks the display value among pairwise concurrent heads: an agent
// write with a same-member human write beside it drops out (288), then the
// later ts, member, clone, seq wins (203, 271).
func winner(heads []head) head {
	var w *head
	for i := range heads {
		h := &heads[i]
		if h.n.origin != OriginHuman {
			dropped := false
			for _, o := range heads {
				if o.n.member == h.n.member && o.n.origin == OriginHuman {
					dropped = true
					break
				}
			}
			if dropped {
				continue
			}
		}
		if w == nil || later(h.n, w.n) {
			w = h
		}
	}
	return *w
}

// write folds one write into a multi-value register and returns the shown
// value. Writes by two members get a "concurrent change" marker; one
// member's losing write from another clone gets a quiet "superseded" note.
func (s *state) write(key, label string, n *node, v any) any {
	var heads []head
	for _, h := range s.reg[key] {
		if !s.hb(h.n, n) {
			heads = append(heads, h)
		}
	}
	conc := heads
	heads = append(append([]head(nil), heads...), head{n, v})
	s.reg[key] = heads
	w := winner(heads)
	if s.marks {
		for _, h := range conc {
			if h.n.member == n.member {
				continue
			}
			mark(h.n, n, "field", label, w.n.id, show(h.v), show(v))
			mark(n, h.n, "field", label, w.n.id, show(v), show(h.v))
		}
		for _, h := range heads {
			if h.n == w.n || h.n.member != w.n.member {
				supersede(h.n, label, nil, "")
			} else if later(h.n, w.n) {
				supersede(h.n, label, w.n, "human")
			} else {
				supersede(h.n, label, w.n, "later")
			}
		}
	}
	return w.v
}

// peek is the value write(key, _, n, v) would show, without writing.
func (s *state) peek(key string, n *node, v any) any {
	heads := []head{{n, v}}
	for _, h := range s.reg[key] {
		if !s.hb(h.n, n) {
			heads = append(heads, h)
		}
	}
	return winner(heads).v
}

// regWinner is the shown write of a register: a comment's mentions,
// attachments, and refs follow the text write that won.
func (s *state) regWinner(key string) *node { return winner(s.reg[key]).n }

func mentionsOf(p obj) []string {
	l, _ := p.list("mentions")
	out := make([]string, 0, len(l))
	for _, x := range l {
		out = append(out, jsString(x))
	}
	return out
}

// labelsOf is a payload's labels: absent or null is none.
func labelsOf(p obj) labelSet { return labelSet(p.strList("labels")) }

// liveLabel is the label id names when the list holds it and it is not
// removed, else nil.
func (s *state) liveLabel(id string) *label {
	if l := s.labels[id]; l != nil && l.removed == nil {
		return l
	}
	return nil
}

// liveLabels is ids without the labels the list does not hold live.
func (s *state) liveLabels(ids labelSet) []string {
	out := []string{}
	for _, id := range ids {
		if s.liveLabel(id) != nil {
			out = append(out, id)
		}
	}
	return out
}

// labelNamed is the live label other than except whose name equals name,
// ignoring case (label:<name> is matched lower-cased), else nil.
func (s *state) labelNamed(name, except string) *label {
	k := strings.ToLower(name)
	for _, id := range s.lorder {
		if l := s.liveLabel(id); l != nil && id != except && strings.ToLower(l.name) == k {
			return l
		}
	}
	return nil
}

func refsOf(p obj) []string {
	l, _ := p.list("refs")
	out := make([]string, 0, len(l))
	for _, x := range l {
		out = append(out, jsString(x))
	}
	return out
}

func attachmentsOf(p obj) []attachment {
	l, _ := p.list("attachments")
	out := make([]attachment, 0, len(l))
	for _, x := range l {
		a := obj(nil)
		if m, ok := x.(map[string]any); ok {
			a = obj(m)
		}
		size, _ := a["size"].(float64)
		out = append(out, attachment{oid: a.str("oid"), name: a.str("name"), mime: a.str("mime"), size: size})
	}
	return out
}

// ---------------------------------------------------------------- endings

// endRec ends a record. A pending handoff of it ends with it, as withdrawn
