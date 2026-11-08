package wstracker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// leadsNobody is (209): a removed member must lead nobody, so removing one
// who is still some live member's lead is refused until their followers are
// moved with roles.set_lead.
func (s *state) leadsNobody(target string) rejection {
	var followers []string
	for _, id := range s.morder {
		if m := s.members[id]; m.removed == nil && m.lead() == target {
			followers = append(followers, id)
		}
	}
	if len(followers) == 0 {
		return ""
	}
	return rejection(target + " still leads " + strings.Join(followers, ", ") +
		"; a removed member must lead nobody (move them with roles.set_lead first)")
}

// Veto returns the tracker's protocol veto hook for a store with this
// genesis roles.json: it flags a roles.remove_member of a member who still
// leads someone (209). Leads are the domain's: genesis leads plus the
// roles.set_lead events the domain applies in the removal's causal past.
//
// wsledger.VetoFunc receives no genesis, and genesis leads cannot be
// derived from the past events alone, hence the constructor. Open, Create,
// Join, and Register set it.
func Veto(genesis json.RawMessage) wsledger.VetoFunc {
	return func(ev wsledger.Event, past []wsledger.Event) error {
		if ev.Type != wsledger.TypeRemoveMember {
			return nil
		}
		var p wsledger.MemberPayload
		if json.Unmarshal(ev.Payload, &p) != nil {
			return nil // the protocol flags a malformed payload itself
		}
		r := replayRoles(past, genesis, newCausal(past).Concurrent)
		if rej := r.S.leadsNobody(p.Member); rej != "" {
			return errors.New(string(rej))
		}
		return nil
	}
}

// Open opens an existing wsledger store with Veto set on its options (and
// any veto the caller set run after it). The veto needs the genesis, which
// only an open store reads, so the store is opened once without it to read
// the genesis and then again with it.
func Open(ctx context.Context, opts wsledger.Options) (*wsledger.Ledger, error) {
	return withVeto(ctx, opts, func(o wsledger.Options) (*wsledger.Ledger, error) { return wsledger.Open(ctx, o) })
}

// Create makes a new store with no remote (wsledger.Create) and returns it
// reopened with the Veto hook installed, as Open does.
func Create(ctx context.Context, opts wsledger.Options) (*wsledger.Ledger, error) {
	return withVeto(ctx, opts, func(o wsledger.Options) (*wsledger.Ledger, error) { return wsledger.Create(ctx, o) })
}

// Join joins the store in opts.Dir as a new member (wsledger.Join) and
// returns it reopened with the Veto hook installed, as Open does.
func Join(ctx context.Context, opts wsledger.Options) (*wsledger.Ledger, error) {
	return withVeto(ctx, opts, func(o wsledger.Options) (*wsledger.Ledger, error) { return wsledger.Join(ctx, o) })
}

// Register creates or joins the store at url (wsledger.Register) and
// returns it reopened with the Veto hook installed, as Open does.
func Register(ctx context.Context, opts wsledger.Options, url string) (*wsledger.Ledger, error) {
	return withVeto(ctx, opts, func(o wsledger.Options) (*wsledger.Ledger, error) { return wsledger.Register(ctx, o, url) })
}

// withVeto runs first (Open, or a constructor that writes no removal)
// without the veto to learn the genesis, closes it, and opens the store
// again with Veto installed.
func withVeto(ctx context.Context, opts wsledger.Options, first func(wsledger.Options) (*wsledger.Ledger, error)) (*wsledger.Ledger, error) {
	probe := opts
	probe.Veto = nil
	l, err := first(probe)
	if err != nil {
		return nil, err
	}
	genesis := bytes.Clone(l.Identity().Genesis)
	if err := l.Close(); err != nil {
		return nil, err
	}
	veto, theirs := Veto(genesis), opts.Veto
	opts.Veto = veto
	if theirs != nil {
		opts.Veto = func(ev wsledger.Event, past []wsledger.Event) error {
			if err := veto(ev, past); err != nil {
				return err
			}
			return theirs(ev, past)
		}
	}
	return wsledger.Open(ctx, opts)
}
