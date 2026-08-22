package cli

import (
	"net/url"
	"testing"
)

// A URL reaching an error message reaches stderr, CI logs, and screenshots.
// Userinfo must not travel with it.
func TestRedactURLRemovesCredentials(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"password", "https://bob:hunter2@warder.example.com", "https://warder.example.com"},
		{"user only", "https://bob@warder.example.com", "https://warder.example.com"},
		{"plain", "https://warder.example.com", "https://warder.example.com"},
		{"loopback", "http://127.0.0.1:8081", "http://127.0.0.1:8081"},
		{"unparseable", "://nonsense", "the configured server"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactURL(tc.in); got != tc.want {
				t.Fatalf("redactURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRedactedURLNeverReachesTheMessage(t *testing.T) {
	err := &UnreachableError{BaseURL: redactURL("https://bob:hunter2@warder.example.com")}
	for _, forbidden := range []string{"hunter2", "bob@"} {
		if containsSubstring(err.Error(), forbidden) {
			t.Fatalf("message contains %q:\n%s", forbidden, err.Error())
		}
	}
}

func TestIsLoopbackURL(t *testing.T) {
	cases := map[string]bool{
		"http://127.0.0.1:8081":                    true,
		"http://localhost:8081":                    true,
		"http://LOCALHOST:8081":                    true,
		"http://[::1]:8081":                        true,
		"https://warder-runtime-ab12.onrender.com": false,
		"https://127.0.0.1.example.com":            false,
	}
	for raw, want := range cases {
		if got := isLoopbackURL(raw); got != want {
			t.Errorf("isLoopbackURL(%q) = %v, want %v", raw, got, want)
		}
	}
}

// The loopback message is the one a new user hits, so it has to name both ways
// out: point at a deployed Warder, or start a local one.
func TestLoopbackMessageNamesBothRemedies(t *testing.T) {
	err := &UnreachableError{BaseURL: "http://127.0.0.1:8081", Loopback: true}
	for _, want := range []string{"ward login --url", "warder-api serve", "ward status"} {
		if !containsSubstring(err.Error(), want) {
			t.Errorf("loopback message is missing %q:\n%s", want, err.Error())
		}
	}
}

func containsSubstring(haystack, needle string) bool {
	return len(haystack) >= len(needle) && func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	}()
}

// The discovery document arrives over the network and decides where a password
// is sent moments later, so it is checked rather than trusted.
func TestAcceptDiscovered(t *testing.T) {
	mustParse := func(raw string) *url.URL {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("bad test URL %q: %v", raw, err)
		}
		return parsed
	}

	cases := []struct {
		name       string
		base       string
		advertised string
		want       string
		ok         bool
	}{
		{"https to https", "https://dash.example.com", "https://runtime.example.com", "https://runtime.example.com", true},
		{"trailing slash trimmed", "https://dash.example.com", "https://runtime.example.com/", "https://runtime.example.com", true},
		{"http dashboard may name http", "http://127.0.0.1:3000", "http://127.0.0.1:8081", "http://127.0.0.1:8081", true},

		// The one that matters: a plaintext downgrade would let anyone able to
		// rewrite the response move a login onto a channel they can read.
		{"https may not be downgraded", "https://dash.example.com", "http://runtime.example.com", "", false},

		{"userinfo stripped", "https://dash.example.com", "https://bob:pw@runtime.example.com", "https://runtime.example.com", true},
		{"foreign scheme refused", "https://dash.example.com", "file:///etc/passwd", "", false},
		{"javascript refused", "https://dash.example.com", "javascript:alert(1)", "", false},
		{"empty refused", "https://dash.example.com", "", "", false},
		{"hostless refused", "https://dash.example.com", "https://", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := acceptDiscovered(mustParse(tc.base), tc.advertised)
			if ok != tc.ok {
				t.Fatalf("acceptDiscovered(%q) ok = %v, want %v", tc.advertised, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("acceptDiscovered(%q) = %q, want %q", tc.advertised, got, tc.want)
			}
		})
	}
}

func TestSameServer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://a.example.com", "https://a.example.com/", true},
		{"https://A.example.com", "https://a.example.com", true},
		{"https://a.example.com", "http://a.example.com", false},
		{"https://a.example.com", "https://b.example.com", false},
	}
	for _, tc := range cases {
		if got := sameServer(tc.a, tc.b); got != tc.want {
			t.Errorf("sameServer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
