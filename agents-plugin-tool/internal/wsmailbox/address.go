package wsmailbox

import (
	"fmt"
	"regexp"
	"strings"
)

// namePattern bounds a bare mailbox name (the part before "@scope"): the
// word-chain generator (wskey.Generate, reused for WS_MAILBOX_AUTO) only
// ever produces lowercase words joined by hyphens, and an explicit
// WS_MAILBOX slug is user-chosen text that also becomes a JSON map key and
// appears in shipped tool output, so the same conservative charset guards
// both — no "@", no ":", no path separators, no whitespace.
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// IsValidName reports whether raw is an acceptable bare mailbox name.
func IsValidName(raw string) bool {
	return namePattern.MatchString(raw)
}

// ReplyIDPrefix is the polymorphic "to:" address form that addresses a
// reply-id capability directly (Decision 12), e.g. "id:7f3a...".
const ReplyIDPrefix = "id:"

// replyIDPattern bounds the hex digest ReplyID produces (64 lowercase hex
// chars for SHA-256), the same conservative "reject anything unexpected"
// posture as namePattern.
var replyIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// IsValidReplyID reports whether raw looks like a ReplyID-produced digest.
func IsValidReplyID(raw string) bool {
	return replyIDPattern.MatchString(raw)
}

// ParseSlugScope splits a "name@scope" identity string (WS_MAILBOX's value,
// or the slug half of a polymorphic "to:" address) into its validated parts.
func ParseSlugScope(raw string) (name string, scope Scope, err error) {
	raw = strings.TrimSpace(raw)
	name, scopeRaw, ok := strings.Cut(raw, "@")
	if !ok {
		return "", "", fmt.Errorf("wsmailbox: %q is not of the form name@scope", raw)
	}
	if !IsValidName(name) {
		return "", "", fmt.Errorf("wsmailbox: invalid mailbox name %q: want lowercase letters, digits, or hyphens, starting with a letter or digit", name)
	}
	if !IsValidScope(scopeRaw) {
		return "", "", fmt.Errorf("wsmailbox: invalid scope %q: want machine, worktree, or clone", scopeRaw)
	}
	return name, Scope(scopeRaw), nil
}

// Address is a parsed polymorphic "to:" value (Decision 12): exactly one of
// the Slug or ReplyID forms is populated, discriminated by Kind.
type Address struct {
	Kind    AddressKind
	Name    string // Kind == AddressSlug only
	Scope   Scope  // Kind == AddressSlug only
	ReplyID string // Kind == AddressReplyID only
}

type AddressKind string

const (
	AddressSlug    AddressKind = "slug"
	AddressReplyID AddressKind = "id"
)

// ParseAddress parses send's polymorphic "to:" argument: "slug@scope" or
// "id:<reply-id>".
func ParseAddress(raw string) (Address, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Address{}, fmt.Errorf("wsmailbox: to is required")
	}
	if rest, ok := strings.CutPrefix(raw, ReplyIDPrefix); ok {
		if !IsValidReplyID(rest) {
			return Address{}, fmt.Errorf("wsmailbox: %q is not a valid reply-id handle", raw)
		}
		return Address{Kind: AddressReplyID, ReplyID: rest}, nil
	}
	name, scope, err := ParseSlugScope(raw)
	if err != nil {
		return Address{}, err
	}
	return Address{Kind: AddressSlug, Name: name, Scope: scope}, nil
}
