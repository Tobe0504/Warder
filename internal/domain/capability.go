// Package domain contains the core vocabulary of the Secret Access Broker.
//
// The central idea of the product lives in this package: the difference between
// being able to USE a credential and being able to SEE it. Those are two
// distinct capabilities and one never implies the other.
package domain

import "slices"

// Capability is a single, indivisible permission. Capabilities never imply one
// another. In particular USE_SECRET does not imply READ_SECRET: a workload may
// be fully authorized to use DATABASE_URL while no human holding the same
// authorization is permitted to see its value.
type Capability string

const (
	// CapReadMetadata allows discovering that a secret exists, along with its
	// key, version number, status and expiry. It never exposes the value.
	CapReadMetadata Capability = "READ_METADATA"

	// CapUseSecret allows a plaintext value to be delivered to an authenticated
	// runtime for injection into a process environment. It is the capability
	// behind `ward run`. It does not permit a human-facing reveal.
	CapUseSecret Capability = "USE_SECRET"

	// CapReadSecret allows a plaintext value to be revealed to a human through
	// the dashboard. This capability is deliberately never granted by role; it
	// must always be an explicit, audited, and normally time-boxed grant.
	CapReadSecret Capability = "READ_SECRET"

	CapCreateSecret Capability = "CREATE_SECRET"
	CapRotateSecret Capability = "ROTATE_SECRET"
	CapRevokeSecret Capability = "REVOKE_SECRET"

	// CapManageAccess allows granting and revoking access for other identities.
	// A holder of MANAGE_ACCESS can grant themselves READ_SECRET; that is an
	// accepted and audited property of the model, not a bypass of it.
	CapManageAccess Capability = "MANAGE_ACCESS"

	CapReadAudit Capability = "READ_AUDIT"

	CapManageProject      Capability = "MANAGE_PROJECT"
	CapManageOrganization Capability = "MANAGE_ORGANIZATION"
)

// AllCapabilities is the closed set of capabilities the policy engine accepts.
// Anything outside this set is rejected at the API boundary rather than being
// silently stored and later misinterpreted.
var AllCapabilities = []Capability{
	CapReadMetadata,
	CapUseSecret,
	CapReadSecret,
	CapCreateSecret,
	CapRotateSecret,
	CapRevokeSecret,
	CapManageAccess,
	CapReadAudit,
	CapManageProject,
	CapManageOrganization,
}

// ValidCapability reports whether c is a recognized capability.
func ValidCapability(c Capability) bool {
	return slices.Contains(AllCapabilities, c)
}

// SecretPlaneCapabilities are the capabilities that can lead to plaintext
// leaving the cryptographic boundary. They are never derived from a role and
// must always come from an explicit access grant.
var SecretPlaneCapabilities = []Capability{CapUseSecret, CapReadSecret}

// IsSecretPlane reports whether the capability can result in plaintext being
// released, and therefore requires an explicit grant.
func IsSecretPlane(c Capability) bool {
	return slices.Contains(SecretPlaneCapabilities, c)
}

// Role is an organization-level membership role. Roles intentionally carry only
// management capabilities. They never carry USE_SECRET or READ_SECRET, because
// "who administers the platform" and "whose processes may consume this specific
// credential" are different questions with different blast radii.
type Role string

const (
	RoleOwner     Role = "OWNER"
	RoleAdmin     Role = "ADMIN"
	RoleDeveloper Role = "DEVELOPER"
	RoleViewer    Role = "VIEWER"
)

// ValidRole reports whether r is a recognized role.
func ValidRole(r Role) bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleDeveloper, RoleViewer:
		return true
	}
	return false
}

// roleCapabilities maps a role to the capabilities it confers across the whole
// organization.
//
// Deliberately absent from every row: USE_SECRET and READ_SECRET. An owner who
// wants to run an application, or to read a value, holds an access grant that
// says so and that appears in the audit log and the access UI. There is no
// ambient authority over secret material anywhere in this table.
var roleCapabilities = map[Role][]Capability{
	RoleOwner: {
		CapReadMetadata,
		CapCreateSecret,
		CapRotateSecret,
		CapRevokeSecret,
		CapManageAccess,
		CapReadAudit,
		CapManageProject,
		CapManageOrganization,
	},
	RoleAdmin: {
		CapReadMetadata,
		CapCreateSecret,
		CapRotateSecret,
		CapRevokeSecret,
		CapManageAccess,
		CapReadAudit,
		CapManageProject,
	},
	RoleDeveloper: {
		CapReadMetadata,
	},
	RoleViewer: {
		CapReadMetadata,
	},
}

// CapabilitiesForRole returns the organization-wide capabilities conferred by a
// role. The returned slice is a copy and is safe for the caller to modify.
func CapabilitiesForRole(r Role) []Capability {
	caps, ok := roleCapabilities[r]
	if !ok {
		return nil
	}
	out := make([]Capability, len(caps))
	copy(out, caps)
	return out
}
