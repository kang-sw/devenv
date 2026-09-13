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

// TestIsValidNameLengthBoundary pins namePattern's 64-char ceiling
// (`^[a-z0-9][a-z0-9-]{0,63}$` = 1 lead char + up to 63 more): 63 and 64 chars
// are accepted, 65 is rejected. The boundary matters because a bare name also
// becomes a JSON map key and appears in shipped tool output.
func TestIsValidNameLengthBoundary(t *testing.T) {
	repeat := func(n int) string {
		out := make([]byte, n)
		for i := range out {
			out[i] = 'a'
		}
		return string(out)
	}
	for _, tc := range []struct {
		length int
		want   bool
	}{
		{length: 63, want: true},
		{length: 64, want: true},
		{length: 65, want: false},
	} {
		if got := IsValidName(repeat(tc.length)); got != tc.want {
			t.Fatalf("IsValidName(%d chars) = %v, want %v", tc.length, got, tc.want)
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
