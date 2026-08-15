// Package ratelimit provides the request throttling applied to authentication,
// secret delivery, and sensitive administrative operations.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter decides whether an action may proceed.
//
// It is an interface because the in-memory implementation below is per-process,
// which is the right trade for the MVP and the wrong one for a multi-instance
// deployment. Swapping in a shared backend later should not touch a handler.
type Limiter interface {
	// Allow reports whether the action keyed by key may proceed now, and how
	// long to wait if not.
	Allow(key string) (allowed bool, retryAfter time.Duration)
}

// Rule describes an allowance: Burst actions immediately, refilling at
// Rate actions per Period.
type Rule struct {
	// Burst is the maximum number of actions available at once.
	Burst int
	// Rate is the number of actions restored per Period.
	Rate int
	// Period is the refill interval.
	Period time.Duration
}

// Rules for each surface.
//
// The three surfaces have genuinely different shapes, and giving them one
// shared limit would mean either throttling a legitimate deployment or leaving
// login open to sustained guessing.
var (
	// Login is guessing-shaped: a human types a password a handful of times.
	// Anything beyond that is an attack, and the cost of being wrong is a brief
	// wait for one person.
	Login = Rule{Burst: 5, Rate: 5, Period: time.Minute}

	// Invitation covers redeeming an invitation.
	//
	// Deliberately its own bucket rather than sharing Login's. The two actions
	// have nothing in common but being unauthenticated: a wave of people
	// accepting invitations after an onboarding announcement would otherwise
	// lock everyone out of sign-in, and a password-guessing attack would lock
	// out acceptance.
	//
	// It is also looser, because guessing is not the threat it defends against.
	// An invitation token carries 256 bits from a CSPRNG; no rate limit is what
	// makes that unguessable. This bounds enumeration and abuse volume.
	Invitation = Rule{Burst: 10, Rate: 10, Period: time.Minute}

	// RuntimeAuth is exchange-shaped: a process starts, authenticates once, and
	// runs. A crash loop legitimately produces bursts, so the allowance is
	// wider than login while still bounding a token brute-force attempt.
	//
	// Brute force is not the reason this limit exists: a 256-bit credential is
	// not going to be guessed. It exists so that a compromised network position
	// cannot be used to enumerate or hammer the endpoint cheaply.
	RuntimeAuth = Rule{Burst: 30, Rate: 30, Period: time.Minute}

	// RuntimeDeliver is called once per process start, occasionally more.
	RuntimeDeliver = Rule{Burst: 60, Rate: 60, Period: time.Minute}

	// Reveal is the most sensitive human action in the product. It is
	// deliberately tight: a person revealing more than a few values in a minute
	// is either doing something unusual or is not a person.
	Reveal = Rule{Burst: 5, Rate: 10, Period: time.Hour}

	// Admin covers ordinary dashboard reads and writes.
	Admin = Rule{Burst: 120, Rate: 120, Period: time.Minute}

	// Sensitive covers token creation, rotation, and grant changes.
	Sensitive = Rule{Burst: 20, Rate: 20, Period: time.Minute}
)

// InMemory is a token-bucket limiter held in process memory.
//
// Its limitation is worth stating plainly: with N application instances behind
// a load balancer, the effective limit is N times the configured one, and
// restarting an instance clears its buckets. That is acceptable for the MVP
// because these limits are defence in depth rather than the primary control:
// credentials carry 256 bits of entropy and passwords are Argon2id, but a
// production deployment that cares about the login limit specifically should
// move this to a shared backend. See docs/security/threat-model.md.
type InMemory struct {
	rule Rule

	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// New constructs an in-memory limiter for a rule.
func New(rule Rule) *InMemory {
	return NewWithClock(rule, time.Now)
}

// NewWithClock constructs a limiter with an injected clock, for tests.
func NewWithClock(rule Rule, now func() time.Time) *InMemory {
	if now == nil {
		now = time.Now
	}
	return &InMemory{rule: rule, buckets: make(map[string]*bucket), now: now}
}

// Allow implements Limiter.
func (l *InMemory) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(l.rule.Burst), lastSeen: now}
		l.buckets[key] = b
	}

	b.tokens = l.refilled(b, now)
	b.lastSeen = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	// Time until one token is available again.
	wait := time.Duration((1 - b.tokens) / l.perSecond() * float64(time.Second))
	return false, wait
}

func (l *InMemory) perSecond() float64 {
	return float64(l.rule.Rate) / l.rule.Period.Seconds()
}

// refilled reports what a bucket's token count would be at time now.
//
// Refill is lazy; it happens when a bucket is touched, so the stored count is
// only meaningful together with lastSeen. Sweep needs the same calculation, and
// having it in one place is what keeps the two from disagreeing.
func (l *InMemory) refilled(b *bucket, now time.Time) float64 {
	elapsed := now.Sub(b.lastSeen)
	if elapsed <= 0 {
		return b.tokens
	}
	return min(b.tokens+elapsed.Seconds()*l.perSecond(), float64(l.rule.Burst))
}

// Sweep discards buckets whose allowance has fully refilled and which have not
// been touched recently, so that a stream of distinct keys, one per client
// address, say: cannot grow memory without bound.
//
// The refilled count is computed rather than read, because refill is lazy: an
// exhausted bucket that was abandoned still holds a stored count of zero no
// matter how much time has passed. Reading the stored value instead would make
// exactly the abandoned buckets uncollectable, which is the opposite of what a
// sweeper is for, and those are the ones an attacker cycling through addresses
// generates.
//
// Discarding a fully refilled bucket is equivalent to keeping it: a fresh
// bucket starts full. A bucket that is still under its limit is retained, so
// sweeping can never restore an allowance someone has spent.
func (l *InMemory) Sweep(idleFor time.Duration) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-idleFor)
	removed := 0

	for key, b := range l.buckets {
		if b.lastSeen.Before(cutoff) && l.refilled(b, now) >= float64(l.rule.Burst) {
			delete(l.buckets, key)
			removed++
		}
	}
	return removed
}

// Len reports how many buckets are being tracked, for tests and metrics.
func (l *InMemory) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// StartSweeper runs Sweep periodically until stop is closed.
func (l *InMemory) StartSweeper(every, idleFor time.Duration, stop <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.Sweep(idleFor)
			case <-stop:
				return
			}
		}
	}()
}
