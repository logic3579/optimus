//go:build dbtest

package prometheus

import (
	"errors"
	"net/netip"
)

// NewLoopbackPolicyForDBTest permits one exact loopback address for an
// in-process test server. Production builds do not expose this constructor and
// NewPolicy continues to reject every loopback destination.
func NewLoopbackPolicyForDBTest(prefix string, resolver Resolver) (*Policy, error) {
	parsed, err := netip.ParsePrefix(prefix)
	if err != nil || parsed != parsed.Masked() ||
		(parsed.Addr().Is4() && parsed.Bits() != 32) ||
		(parsed.Addr().Is6() && parsed.Bits() != 128) ||
		!parsed.Addr().IsLoopback() {
		return nil, errors.New("dbtest loopback policy requires an exact /32 or /128 loopback prefix")
	}
	policy, err := NewPolicy([]string{prefix}, resolver)
	if err != nil {
		return nil, err
	}
	policy.allowLoopbackDBTest = true
	return policy, nil
}
