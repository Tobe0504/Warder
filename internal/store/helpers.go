package store

import (
	"net/netip"

	"github.com/Tobe0504/Warder/internal/domain"
)

// nullInet renders a client address for the inet column, yielding NULL when the
// address is absent or unparseable rather than storing a value that looks
// authoritative but is not.
func nullInet(addr string) any {
	if addr == "" {
		return nil
	}
	parsed, err := netip.ParseAddr(addr)
	if err != nil {
		return nil
	}
	return parsed.String()
}

// capabilityStrings converts capabilities for storage in a text[] column.
//
// The result is always non-nil. A nil Go slice binds as SQL NULL rather than as
// an empty array, which a NOT NULL array column rejects, and which, on a
// nullable column, would read back as "unknown" where the caller meant "none".
func capabilityStrings(caps []domain.Capability) []string {
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, string(c))
	}
	return out
}

// textArray normalizes a string slice for a NOT NULL text[] column, for the
// same reason as capabilityStrings.
func textArray(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// capabilitiesFrom converts a text[] column back into capabilities.
//
// Unrecognized values are dropped rather than carried forward. A capability
// this build does not understand cannot be enforced, so treating it as
// meaningless is the only safe reading, and the policy engine denies anything
// it cannot recognize regardless.
func capabilitiesFrom(values []string) []domain.Capability {
	out := make([]domain.Capability, 0, len(values))
	for _, v := range values {
		c := domain.Capability(v)
		if domain.ValidCapability(c) {
			out = append(out, c)
		}
	}
	return out
}
