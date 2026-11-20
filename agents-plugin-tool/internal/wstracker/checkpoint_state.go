package wstracker

import (
	"maps"
	"slices"
)

// checkpoint saves fold-owned state without borrowing mutable containers.
// Event nodes are stable identities owned by replay, not by the fold: keep
// their pointers (including those in register values and body lines). hb is
// retained too; a caller changing the causal index can replace it on the copy.
func (s *state) checkpoint() *state {
	cp := *s
	cp.morder = slices.Clone(s.morder)
	cp.aorder = slices.Clone(s.aorder)
	cp.iorder = slices.Clone(s.iorder)
	cp.corder = slices.Clone(s.corder)
	cp.rorder = slices.Clone(s.rorder)
	cp.lorder = slices.Clone(s.lorder)
	cp.oorder = slices.Clone(s.oorder)
	cp.ccOrder = slices.Clone(s.ccOrder)
	cp.place = slices.Clone(s.place)
	cp.admins = maps.Clone(s.admins)
	cp.prio.names = slices.Clone(s.prio.names)

	cp.members = maps.Clone(s.members)
	for id, m := range s.members {
		if m == nil {
			continue
		}
		v := *m // Profile contains only strings.
		v.keys = slices.Clone(m.keys)
		v.revoked = slices.Clone(m.revoked)
		v.leads = slices.Clone(m.leads)
		cp.members[id] = &v
	}
	cp.items = maps.Clone(s.items)
	for id, it := range s.items {
		if it == nil {
			continue
		}
		v := *it
		if it.due != nil {
			d := *it.due
			v.due = &d
		}
		if it.start != nil {
			d := *it.start
			v.start = &d
		}
		if it.open != nil {
			o := *it.open
			v.open = &o
		}
		v.mentions = slices.Clone(it.mentions)
		v.attachments = slices.Clone(it.attachments)
		v.refs = slices.Clone(it.refs)
		v.labels = slices.Clone(it.labels)
		v.ccExcluded = maps.Clone(it.ccExcluded)
		v.cc = slices.Clone(it.cc)
		for i := range v.cc {
			v.cc[i].orgs = maps.Clone(it.cc[i].orgs)
		}
		cp.items[id] = &v
	}

	// Records appear in three indexes. Memoize by pointer, not id, so all
	// indexes in the copy address the same new record, never the saved one.
	records := map[*rec]*rec{}
	cloneRec := func(r *rec) *rec {
		if r == nil {
			return nil
		}
		if v := records[r]; v != nil {
			return v
		}
		v := *r
		v.writes = slices.Clone(r.writes)
		records[r] = &v
		return &v
	}
	cp.asg = maps.Clone(s.asg)
	for id, r := range s.asg {
		cp.asg[id] = cloneRec(r)
	}
	cp.recs = slices.Clone(s.recs)
	for i, r := range s.recs {
		cp.recs[i] = cloneRec(r)
	}
	cp.byItem = maps.Clone(s.byItem)
	for id, rs := range s.byItem {
		out := slices.Clone(rs)
		for i, r := range rs {
			out[i] = cloneRec(r)
		}
		cp.byItem[id] = out
	}

	rules := map[*rule]*rule{}
	cloneRule := func(r *rule) *rule {
		if r == nil {
			return nil
		}
		if v := rules[r]; v != nil {
			return v
		}
		v := *r
		v.excluded = maps.Clone(r.excluded)
		rules[r] = &v
		return &v
	}
	cp.rules = slices.Clone(s.rules)
	for i, r := range s.rules {
		cp.rules[i] = cloneRule(r)
	}
	cp.ruleByID = maps.Clone(s.ruleByID)
	for id, r := range s.ruleByID {
		cp.ruleByID[id] = cloneRule(r)
	}

	cp.comments = maps.Clone(s.comments)
	for id, c := range s.comments {
		if c == nil {
			continue
		}
		v := *c
		v.mentions = slices.Clone(c.mentions)
		v.attachments = slices.Clone(c.attachments)
		v.refs = slices.Clone(c.refs)
		cp.comments[id] = &v
	}
	cp.reqs = maps.Clone(s.reqs)
	for id, q := range s.reqs {
		if q != nil {
			v := *q
			cp.reqs[id] = &v
		}
	}
	cp.reg = maps.Clone(s.reg)
	for key, hs := range s.reg {
		out := slices.Clone(hs)
		for i, h := range hs {
			out[i].v = checkpointHeadValue(h.v)
		}
		cp.reg[key] = out
	}
	cp.bodies = maps.Clone(s.bodies)
	for id, f := range s.bodies {
		if f == nil {
			continue
		}
		v := *f
		v.writes = slices.Clone(f.writes)
		v.val = checkpointBodyValue(f.val)
		v.base = maps.Clone(f.base)
		for key, b := range f.base {
			v.base[key] = checkpointBodyValue(b)
		}
		cp.bodies[id] = &v
	}
	cp.labels = maps.Clone(s.labels)
	for id, l := range s.labels {
		if l != nil {
			v := *l
			cp.labels[id] = &v
		}
	}
	cp.orgs = maps.Clone(s.orgs)
	for id, o := range s.orgs {
		if o != nil {
			v := *o
			cp.orgs[id] = &v
		}
	}
	cp.ccOrgs = maps.Clone(s.ccOrgs)
	for key, reg := range s.ccOrgs {
		if reg != nil {
			v := *reg
			v.added = maps.Clone(reg.added)
			v.excluded = maps.Clone(reg.excluded)
			cp.ccOrgs[key] = &v
		}
	}
	cp.ccOrgsByItem = map[string][]*ccOrgReg{}
	for _, key := range cp.ccOrder {
		if reg := cp.ccOrgs[key]; reg != nil {
			cp.ccOrgsByItem[reg.item] = append(cp.ccOrgsByItem[reg.item], reg)
		}
	}
	return &cp
}

func checkpointBodyValue(v bodyVal) bodyVal {
	return bodyVal{lines: slices.Clone(v.lines), atts: slices.Clone(v.atts)}
}

// These are the values written by state.write and applyBody. Keep this
// exhaustive: a new mutable register type must not silently alias a save.
func checkpointHeadValue(v any) any {
	switch x := v.(type) {
	case nil, string, bool, int, float64, ccVal, ccOrgVal, statusVal, placeVal:
		return v
	case []string:
		return slices.Clone(x)
	case labelSet:
		return slices.Clone(x)
	case priority:
		x.names = slices.Clone(x.names)
		return x
	default:
		panic("wstracker: unsupported checkpoint register value")
	}
}
