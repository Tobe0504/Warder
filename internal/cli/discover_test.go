package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverRuntimeURLEndToEnd(t *testing.T) {
	runtime := "http://runtime.internal:8081"

	dash := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/warder" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"runtimeUrl":"` + runtime + `"}`))
	}))
	defer dash.Close()

	if got := DiscoverRuntimeURL(context.Background(), dash.URL); got != runtime {
		t.Fatalf("discovery = %q, want %q", got, runtime)
	}

	// A server with no discovery document leaves the seed untouched.
	bare := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer bare.Close()
	if got := DiscoverRuntimeURL(context.Background(), bare.URL); got != bare.URL {
		t.Fatalf("404 should return the seed, got %q", got)
	}

	// So does one that is not there at all.
	const dead = "http://127.0.0.1:1"
	if got := DiscoverRuntimeURL(context.Background(), dead); got != dead {
		t.Fatalf("unreachable should return the seed, got %q", got)
	}

	// Garbage in the document is ignored rather than followed.
	junk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"runtimeUrl":"javascript:alert(1)"}`))
	}))
	defer junk.Close()
	if got := DiscoverRuntimeURL(context.Background(), junk.URL); got != junk.URL {
		t.Fatalf("bad scheme should return the seed, got %q", got)
	}
}
