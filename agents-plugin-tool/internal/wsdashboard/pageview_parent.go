package wsdashboard

import "strings"

// parentChainCap bounds the upward walk of a session's owners: a corrupt
// record or layout can name a cycle, and the visited set alone would still
// let a long forged chain cost a render.
const parentChainCap = 16

// ParentView is the head's parent jump: the session that owns the viewed one,
// shown as a pill left of the identity. Nil on a session nobody owns.
type ParentView struct {
	// ReplyID is the immediate parent's reply-id; Name its display label
	// (ReplyID when it has none).
	ReplyID, Name string
	// Linked reports a parent this daemon still serves, so its page is a
	// link target; a pruned one shows as a muted, inert badge.
	Linked bool
	// Path is the ancestor chain, root first, parent last ("Root › Mid"),
	// when the session sits deeper than one level; empty otherwise.
	Path string
}

// parentSource is what resolveParent reads: the index (a session by reply-id)
// and the rail layout's nesting, for a session the registry names no owner
// of. Both are in-memory reads.
type parentSource struct {
	session      func(replyID string) (Session, bool)
	layoutParent func(replyID string) string
}

// resolveParent walks s's owners upward: its registered ParentReplyID first
// (a daemon-spawned child or worker), else the rail layout's parent (a
// hired or hand-nested session). A visited set and parentChainCap bound the
// walk. A parent the index no longer holds ends it, since its own owner
// cannot be read.
func resolveParent(s Session, src parentSource) *ParentView {
	seen := map[string]bool{s.ReplyID: true}
	var ids, names []string
	linked := false
	cur, curID := s, s.ReplyID
	for len(ids) < parentChainCap {
		p := cur.ParentReplyID
		if !ValidReplyID(p) && src.layoutParent != nil {
			p = src.layoutParent(curID)
		}
		if !ValidReplyID(p) || seen[p] {
			break
		}
		seen[p] = true
		ps, ok := Session{}, false
		if src.session != nil {
			ps, ok = src.session(p)
		}
		name := p
		if ok && ps.Label != "" {
			name = ps.Label
		}
		ids, names = append(ids, p), append(names, name)
		if len(ids) == 1 {
			linked = ok
		}
		if !ok {
			break
		}
		cur, curID = ps, p
	}
	if len(ids) == 0 {
		return nil
	}
	v := &ParentView{ReplyID: ids[0], Name: names[0], Linked: linked}
	if len(names) > 1 {
		rev := make([]string, len(names))
		for i, n := range names {
			rev[len(names)-1-i] = n
		}
		v.Path = strings.Join(rev, " › ")
	}
	return v
}
