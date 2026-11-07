package wsledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Ref layout. Every ref is a branch so host push permissions and branch
// protection rules apply to it.
const (
	refPrefix    = "refs/heads/tracker/"
	genesisRef   = refPrefix + "genesis"
	claimPrefix  = refPrefix + "id/"
	memberPrefix = refPrefix + "m/"

	rolesFile = "roles.json"
)

// Protocol-owned identity event types. Every other type is a domain event.
const (
	TypeMemberJoin   = "member.join"
	TypeAddAdmin     = "roles.add_admin"
	TypeRemoveAdmin  = "roles.remove_admin"
	TypeAddKey       = "roles.add_key"
	TypeRevokeKey    = "roles.revoke_key"
	TypeRemoveMember = "roles.remove_member"
)

func isIdentityType(t string) bool {
	switch t {
	case TypeMemberJoin, TypeAddAdmin, TypeRemoveAdmin, TypeAddKey, TypeRevokeKey, TypeRemoveMember:
		return true
	}
	return false
}

// idPattern constrains member and clone ids: lowercase so refs never collide
// on a case-insensitive filesystem, and no '.', '/', or '#', which the
// stream and event id forms use as separators.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidID reports whether s is a valid member or clone id.
func ValidID(s string) bool { return idPattern.MatchString(s) }

func streamKey(member, clone string) string { return member + "." + clone }

func splitStream(s string) (member, clone string, ok bool) {
	i := strings.IndexByte(s, '.')
	if i < 0 {
		return "", "", false
	}
	member, clone = s[:i], s[i+1:]
	return member, clone, ValidID(member) && ValidID(clone)
}

func memberRef(member, clone string) string { return memberPrefix + member + "/" + clone }

func claimRef(member string) string { return claimPrefix + member }

// parseMemberRef returns the stream a member ref belongs to.
func parseMemberRef(ref string) (member, clone string, ok bool) {
	rest, found := strings.CutPrefix(ref, memberPrefix)
	if !found {
		return "", "", false
	}
	member, clone, found = strings.Cut(rest, "/")
	if !found || !ValidID(member) || !ValidID(clone) {
		return "", "", false
	}
	return member, clone, true
}

func parseClaimRef(ref string) (member string, ok bool) {
	rest, found := strings.CutPrefix(ref, claimPrefix)
	if !found || !ValidID(rest) {
		return "", false
	}
	return rest, true
}

// parseKey normalizes an OpenSSH public key line to "<type> <base64>" (the
// comment dropped) and returns its SHA256 fingerprint in the form git and
// ssh-keygen print ("SHA256:<unpadded base64>").
func parseKey(line string) (norm, fp string, err error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", "", errors.New("public key must be \"<type> <base64>\"")
	}
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return "", "", fmt.Errorf("public key: %w", err)
	}
	// The wire form starts with the key type as an SSH string.
	if len(blob) < 4 {
		return "", "", errors.New("public key: truncated")
	}
	n := binary.BigEndian.Uint32(blob[:4])
	if uint64(n) > uint64(len(blob)-4) || string(blob[4:4+n]) != fields[0] {
		return "", "", errors.New("public key: type does not match its blob")
	}
	sum := sha256.Sum256(blob)
	return fields[0] + " " + fields[1], "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]), nil
}

// KeyFingerprint normalizes an OpenSSH public key line to "<type> <base64>"
// and returns its fingerprint ("SHA256:<unpadded base64>"), the form the
// identity state compares keys by. It is a pure accessor for the domain
// layer, which mirrors the key rules when it pre-validates identity events.
func KeyFingerprint(line string) (norm, fp string, err error) { return parseKey(line) }

// keyFP returns the fingerprint of a key, or "" when it does not parse.
func keyFP(line string) string {
	_, fp, err := parseKey(line)
	if err != nil {
		return ""
	}
	return fp
}

// Identity event payloads.

// JoinPayload is the payload of member.join.
type JoinPayload struct {
	Member string `json:"member"`
	Name   string `json:"name"`
	Key    string `json:"key"`
}

// MemberPayload is the payload of roles.add_admin, roles.remove_admin, and
// roles.remove_member.
type MemberPayload struct {
	Member string `json:"member"`
}

// KeyPayload is the payload of roles.add_key and roles.revoke_key.
type KeyPayload struct {
	Member string `json:"member"`
	Key    string `json:"key"`
}

// idPayload is the parsed payload of any identity event. keyNorm and keyFP
// are set when the type carries a key and it parses.
type idPayload struct {
	member  string
	name    string
	keyNorm string
	keyFP   string
}

func parseIDPayload(typ string, raw json.RawMessage) (*idPayload, error) {
	dec := func(v any) error {
		d := json.NewDecoder(bytes.NewReader(raw))
		return d.Decode(v)
	}
	p := &idPayload{}
	switch typ {
	case TypeMemberJoin:
		var j JoinPayload
		if err := dec(&j); err != nil {
			return nil, err
		}
		p.member, p.name = j.Member, j.Name
		norm, fp, err := parseKey(j.Key)
		if err != nil {
			return nil, err
		}
		p.keyNorm, p.keyFP = norm, fp
	case TypeAddKey, TypeRevokeKey:
		var k KeyPayload
		if err := dec(&k); err != nil {
			return nil, err
		}
		p.member = k.Member
		norm, fp, err := parseKey(k.Key)
		if err != nil {
			return nil, err
		}
		p.keyNorm, p.keyFP = norm, fp
	default:
		var m MemberPayload
		if err := dec(&m); err != nil {
			return nil, err
		}
		p.member = m.Member
	}
	if !ValidID(p.member) {
		return nil, fmt.Errorf("member %q is not a valid member id", p.member)
	}
	return p, nil
}

// Genesis roles file.

// Roles is the genesis roles.json. The protocol reads Admins and each
// member's ID, Name, and Keys; Leads (and any other field) is passed
// through to the domain untouched.
type Roles struct {
	Version int          `json:"version"`
	Admins  []string     `json:"admins"`
	Members []RoleMember `json:"members"`
}

// RoleMember is one member entry of roles.json.
type RoleMember struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Keys  []string        `json:"keys"`
	Leads json.RawMessage `json:"leads,omitempty"`
}
