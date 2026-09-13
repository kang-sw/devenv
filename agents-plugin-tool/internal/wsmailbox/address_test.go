package wsmailbox

import "testing"

func TestParseAddressSlug(t *testing.T) {
	addr, err := ParseAddress("amber-tide-fox@machine")
	if err != nil {
		t.Fatalf("ParseAddress: %v", err)
	}
	if addr.Kind != AddressSlug || addr.Name != "amber-tide-fox" || addr.Scope != ScopeMachine {
		t.Fatalf("ParseAddress = %#v", addr)
	}
}

func TestParseAddressReplyID(t *testing.T) {
	replyID := ReplyID([]byte("0123456789abcdef0123456789abcdef"), "some-session-key")
	addr, err := ParseAddress("id:" + replyID)
	if err != nil {
		t.Fatalf("ParseAddress: %v", err)
	}
	if addr.Kind != AddressReplyID || addr.ReplyID != replyID {
		t.Fatalf("ParseAddress = %#v", addr)
	}
}

func TestParseAddressRejectsBadShapes(t *testing.T) {
	cases := []string{
		"",
		"noatsign",
		"name@badscope",
		"@machine",
		"BadName@machine",
		"id:tooshort",
		"id:" + "zz" + repeatHex(62),
	}
	for _, raw := range cases {
		if _, err := ParseAddress(raw); err == nil {
			t.Fatalf("ParseAddress(%q) = nil error, want failure", raw)
		}
	}
}

func repeatHex(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = 'a'
	}
	return string(out)
}
