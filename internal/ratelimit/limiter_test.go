package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestBurstThenRefusal(t *testing.T) {
	l := New(Rule{Burst: 3, Rate: 3, Period: time.Minute})

	for i := range 3 {
		if ok, _ := l.Allow("client"); !ok {
			t.Fatalf("request %d within the burst was refused", i+1)
		}
	}

	ok, retryAfter := l.Allow("client")
	if ok {
		t.Fatal("a request beyond the burst was allowed")
	}
	if retryAfter <= 0 {
		t.Fatal("a refusal should say how long to wait")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := New(Rule{Burst: 1, Rate: 1, Period: time.Minute})

	if ok, _ := l.Allow("alice"); !ok {
		t.Fatal("first request refused")
	}
	if ok, _ := l.Allow("alice"); ok {
		t.Fatal("alice exceeded her allowance")
	}
	if ok, _ := l.Allow("bob"); !ok {
		t.Fatal("bob was throttled by alice's usage")
	}
}

func TestRefill(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	l := NewWithClock(Rule{Burst: 2, Rate: 60, Period: time.Minute}, clock)

	l.Allow("k")
	l.Allow("k")
	if ok, _ := l.Allow("k"); ok {
		t.Fatal("allowance was not exhausted")
	}

	// One token per second at this rate.
	now = now.Add(time.Second)
	if ok, _ := l.Allow("k"); !ok {
		t.Fatal("a token should have refilled after a second")
	}

	// Refill must not exceed the burst, or a long idle period would bank an
	// unbounded allowance.
	now = now.Add(time.Hour)
	allowed := 0
	for range 10 {
		if ok, _ := l.Allow("k"); ok {
			allowed++
		}
	}
	if allowed != 2 {
		t.Fatalf("expected the bucket to cap at the burst of 2, got %d", allowed)
	}
}

// Abandoned buckets are the ones that accumulate, and because refill is lazy
// they hold a stored count of zero forever. The sweeper has to reclaim them, or
// an attacker cycling through client addresses grows the map without bound.
func TestSweepReclaimsAbandonedExhaustedBuckets(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	l := NewWithClock(Rule{Burst: 2, Rate: 2, Period: time.Minute}, clock)

	l.Allow("spent")
	l.Allow("spent") // exhausted, then never touched again
	l.Allow("light")

	if l.Len() != 2 {
		t.Fatalf("expected 2 buckets, got %d", l.Len())
	}

	// Ten minutes later both allowances have fully refilled in real terms, so
	// both buckets are equivalent to fresh ones and may be discarded.
	now = now.Add(10 * time.Minute)

	if removed := l.Sweep(time.Minute); removed != 2 {
		t.Fatalf("expected both abandoned buckets to be reclaimed, got %d", removed)
	}
	if l.Len() != 0 {
		t.Fatalf("expected an empty map, got %d buckets", l.Len())
	}
}

// Sweeping must never hand back an allowance someone is still spending, or
// waiting for the sweeper would be a way to reset a limit.
func TestSweepDoesNotRestoreASpentAllowance(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	l := NewWithClock(Rule{Burst: 1, Rate: 1, Period: time.Hour}, clock)

	l.Allow("attacker")

	now = now.Add(30 * time.Minute) // half a token refilled: still under the burst
	l.Sweep(time.Minute)

	if ok, _ := l.Allow("attacker"); ok {
		t.Fatal("sweeping restored an exhausted allowance")
	}
	if l.Len() != 1 {
		t.Fatal("a bucket still under its limit was discarded")
	}
}

func TestConcurrentUseIsSafe(t *testing.T) {
	l := New(Rule{Burst: 100, Rate: 100, Period: time.Minute})

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0

	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.Allow("shared"); ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowed != 100 {
		t.Fatalf("expected exactly the burst to be allowed, got %d", allowed)
	}
}
