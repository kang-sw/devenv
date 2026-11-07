package wsdashboard

import (
	"strconv"
	"strings"
)

// CompareVersions orders two ws-mcp versions, returning -1, 0, or 1.
//
// A version is MAJOR.MINOR.PATCH with an optional leading "v" and an optional
// "-<pre>" suffix. The numeric core compares first; with equal cores a
// suffixed version ("0.48.10-dev") is newer than the bare release
// ("0.48.10"), and two suffixed versions compare as strings. A version that
// does not parse is older than any that does, and two unparsable versions
// are equal.
//
// This departs from semver, where a pre-release precedes its release: the
// source version names the release it follows (it becomes "X-dev" in the
// commit that releases X), so a "-dev" build is post-release code and must
// outrank a stale release daemon of the same core.
//
// The order is total, so two ws-mcp builds never each see the other's daemon
// as older: replacement cannot flap.
func CompareVersions(a, b string) int {
	ca, pa, oka := parseVersion(a)
	cb, pb, okb := parseVersion(b)
	switch {
	case !oka && !okb:
		return 0
	case !oka:
		return -1
	case !okb:
		return 1
	}
	for i := range ca {
		if ca[i] != cb[i] {
			return sign(ca[i] - cb[i])
		}
	}
	switch {
	case pa == pb:
		return 0
	case pa == "":
		return -1
	case pb == "":
		return 1
	}
	return strings.Compare(pa, pb)
}

func parseVersion(v string) (core [3]int, pre string, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v, pre = v[:i], v[i+1:]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return core, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return core, "", false
		}
		core[i] = n
	}
	return core, pre, true
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
