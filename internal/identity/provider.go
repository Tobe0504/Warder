// Package identity turns a presented credential into an authenticated
// principal.
//
// Authentication is separated from authorization on purpose. This package
// answers only "who is this, and what ceiling does the credential they used
// impose". It never decides what they may do; that is authz.Engine's job,
// working from the principal produced here.
//
// The separation is what makes the roadmap in docs/architecture tractable: AWS
// IAM, GCP workload identity, Kubernetes service accounts, and OIDC federation
// are each a new implementation of the interface below, and none of them
// require a change to policy evaluation or to any handler.
package identity

import (
	"context"
	"errors"
	"net/http"

	"github.com/Tobe0504/Warder/internal/domain"
)

// ErrUnauthenticated is returned for every authentication failure.
//
// There is deliberately only one. A malformed credential, an unknown one, a
// revoked one, an expired one, and one belonging to a disabled identity are
// indistinguishable to the caller, so an attacker holding a stolen token cannot
// learn whether it was ever valid or merely stopped being so.
var ErrUnauthenticated = errors.New("authentication failed")

// Request is the transport-independent view of an authentication attempt.
//
// It is not an *http.Request, because a future Kubernetes or AWS provider
// authenticates from a projected service account token or a signed identity
// document rather than from HTTP alone.
type Request struct {
	// Credential is the presented bearer credential, if any.
	Credential string

	// Headers carries the raw request headers, for providers that authenticate
	// from a signed document rather than a bearer token.
	Headers http.Header

	// ClientIP is the observed client address, for audit and rate limiting.
	ClientIP string

	// UserAgent is recorded on the resulting session.
	UserAgent string
}

// Provider authenticates a request and produces a principal.
type Provider interface {
	// Name identifies the provider in audit records.
	Name() string

	// Handles reports whether this provider recognizes the credential's shape.
	// It is a cheap pre-check so that a chain does not perform database work
	// for every provider on every request.
	Handles(req Request) bool

	// Authenticate resolves the principal, or returns ErrUnauthenticated.
	Authenticate(ctx context.Context, req Request) (*domain.Principal, error)
}

// Chain tries each provider in order and returns the first success.
//
// Order matters only for efficiency, not for security: Handles distinguishes
// credential kinds by their prefix, so at most one provider claims any given
// credential.
type Chain struct {
	providers []Provider
}

// NewChain builds a provider chain.
func NewChain(providers ...Provider) *Chain { return &Chain{providers: providers} }

// Authenticate resolves a principal from the first provider that handles the
// credential.
func (c *Chain) Authenticate(ctx context.Context, req Request) (*domain.Principal, error) {
	for _, p := range c.providers {
		if !p.Handles(req) {
			continue
		}
		return p.Authenticate(ctx, req)
	}
	return nil, ErrUnauthenticated
}
