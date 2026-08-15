package identity

import (
	"context"

	"github.com/Tobe0504/Warder/internal/domain"
)

// The providers below are not implemented. They are declared so that the
// interface can be checked against the real shape of each mechanism now, while
// the work of integrating each one is deferred.
//
// Each replaces a long-lived bearer token with something the workload proves
// rather than holds, which is the direction the runtime story should move:
//
//	AwsIdentityProvider         a signed sts:GetCallerIdentity request, verified
//	                            against an expected account and role ARN.
//	GcpIdentityProvider         an instance identity JWT, verified against
//	                            Google's public keys with an expected audience.
//	AzureIdentityProvider       an IMDS-issued token for a managed identity.
//	KubernetesIdentityProvider  a projected service account token, verified via
//	                            the cluster's TokenReview API or its JWKS.
//	OIDCIdentityProvider        a generic OIDC token, which covers GitHub Actions
//	                            and most CI systems without a stored credential
//	                            at all.
//
// All of them share a shape the interface already accommodates: read a signed
// document from the request, verify it against an external authority, map the
// verified subject onto a machine identity, and derive the credential scope
// from that identity's configured binding rather than from the token.
//
// Two things must hold for any of them, and are worth stating before the code
// exists. The mapping from an external subject to a machine identity has to be
// exact-match against a stored binding, never a pattern, or a repository name
// or a role path becomes a way to impersonate. And the audience of the
// presented token must be checked against this deployment, or a token minted
// for another service can be replayed here.

// AwsIdentityProvider will authenticate AWS workloads.
type AwsIdentityProvider struct{}

func (p *AwsIdentityProvider) Name() string         { return "aws-iam" }
func (p *AwsIdentityProvider) Handles(Request) bool { return false }
func (p *AwsIdentityProvider) Authenticate(context.Context, Request) (*domain.Principal, error) {
	return nil, ErrUnauthenticated
}

var _ Provider = (*AwsIdentityProvider)(nil)

// OIDCIdentityProvider will authenticate any OIDC-federated workload, which
// covers GitHub Actions and most CI systems with no stored credential.
type OIDCIdentityProvider struct{}

func (p *OIDCIdentityProvider) Name() string         { return "oidc" }
func (p *OIDCIdentityProvider) Handles(Request) bool { return false }
func (p *OIDCIdentityProvider) Authenticate(context.Context, Request) (*domain.Principal, error) {
	return nil, ErrUnauthenticated
}

var _ Provider = (*OIDCIdentityProvider)(nil)

// KubernetesIdentityProvider will authenticate pods by projected service
// account token.
type KubernetesIdentityProvider struct{}

func (p *KubernetesIdentityProvider) Name() string         { return "kubernetes" }
func (p *KubernetesIdentityProvider) Handles(Request) bool { return false }
func (p *KubernetesIdentityProvider) Authenticate(context.Context, Request) (*domain.Principal, error) {
	return nil, ErrUnauthenticated
}

var _ Provider = (*KubernetesIdentityProvider)(nil)
