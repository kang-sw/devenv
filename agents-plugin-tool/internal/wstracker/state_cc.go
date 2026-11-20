package wstracker

func ccOrgKey(item, org string) string { return item + ":" + org }

func ccEntryAt(it *item, member string) (*ccEntry, int) {
	for i := range it.cc {
		if it.cc[i].member == member {
			return &it.cc[i], i
		}
	}
	return nil, -1
}

func (s *state) addCCSource(it *item, member, org string, personal bool, since *node) {
	e, _ := ccEntryAt(it, member)
	if e == nil {
		it.cc = append(it.cc, ccEntry{member: member, since: since, orgs: map[string]bool{}})
		e = &it.cc[len(it.cc)-1]
	}
	if personal {
		e.personal = true
	}
	if org != "" {
		if e.orgs == nil {
			e.orgs = map[string]bool{}
		}
		e.orgs[org] = true
	}
}

func (s *state) removeCCSource(it *item, member, org string, all bool) {
	e, at := ccEntryAt(it, member)
	if e == nil {
		return
	}
	if all {
		e.personal, e.orgs = false, map[string]bool{}
	} else {
		delete(e.orgs, org)
	}
	if !e.personal && len(e.orgs) == 0 {
		it.cc = append(it.cc[:at:at], it.cc[at+1:]...)
	}
}

// applyCC folds a personal item.cc or item.uncc. The register still resolves
// concurrent personal writes; an uncc clears every source and excludes the
// member from all active org registrations on the item.
func (s *state) applyCC(n *node) {
	it, m := s.items[n.p.str("item")], n.p.str("member")
	if it == nil || m == "" {
		return
	}
	key := "cc:" + it.id + ":" + m
	on := bool(s.write(key, "CC of "+m+" on "+it.alias, n, ccVal(n.typ == TypeItemCC)).(ccVal))
	if on {
		delete(it.ccExcluded, m)
		for _, reg := range s.ccOrgsByItem[it.id] {
			delete(reg.excluded, m)
		}
		s.addCCSource(it, m, "", true, s.regWinner(key))
		return
	}
	it.ccExcluded[m] = true
	s.removeCCSource(it, m, "", true)
	for _, reg := range s.ccOrgsByItem[it.id] {
		reg.excluded[m] = true
		delete(reg.added, m)
	}
}

func (s *state) applyCCOrg(n *node) {
	it, orgID := s.items[n.p.str("item")], n.p.str("org")
	if it == nil || orgID == "" {
		return
	}
	k := ccOrgKey(it.id, orgID)
	on := bool(s.write("cco:"+k, "CC org "+orgID+" on "+it.alias, n, ccOrgVal(n.typ == TypeItemCCOrg)).(ccOrgVal))
	reg := s.ccOrgs[k]
	if reg == nil {
		reg = &ccOrgReg{item: it.id, org: orgID, added: map[string]bool{}, excluded: map[string]bool{}}
		s.ccOrgs[k] = reg
		s.ccOrgsByItem[it.id] = append(s.ccOrgsByItem[it.id], reg)
		s.ccOrder = append(s.ccOrder, k)
	}
	if on {
		if !reg.active {
			reg.since = s.regWinner("cco:" + k)
		}
		reg.active = true
		s.extendCCOrg(reg, n)
		return
	}
	for member := range reg.added {
		s.removeCCSource(it, member, orgID, false)
	}
	reg.active, reg.added = false, map[string]bool{}
	reg.since = nil
}

func (s *state) extendCCOrg(reg *ccOrgReg, n *node) {
	if reg == nil || !reg.active {
		return
	}
	it := s.items[reg.item]
	o := s.liveOrg(reg.org)
	if it == nil || o == nil || o.head == "" {
		return
	}
	s.extendCCOrgMembers(reg, it, n, s.orgMembers(o))
}

func (s *state) extendCCOrgMembers(reg *ccOrgReg, it *item, n *node, members []string) {
	for _, member := range members {
		if it.ccExcluded[member] || reg.excluded[member] || reg.added[member] {
			continue
		}
		reg.added[member] = true
		s.addCCSource(it, member, reg.org, false, n)
	}
}

func (s *state) refreshOrgCC(n *node) {
	members := map[string][]string{}
	computed := map[string]bool{}
	for _, k := range s.ccOrder {
		reg := s.ccOrgs[k]
		if reg == nil || !reg.active {
			continue
		}
		it := s.items[reg.item]
		if it == nil {
			continue
		}
		if !computed[reg.org] {
			members[reg.org] = s.orgMembers(s.liveOrg(reg.org))
			computed[reg.org] = true
		}
		s.extendCCOrgMembers(reg, it, n, members[reg.org])
	}
}
