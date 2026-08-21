package cli

import "testing"

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
