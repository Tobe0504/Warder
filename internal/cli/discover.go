package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// discoveryPath is where a Warder dashboard publishes its runtime address.
const discoveryPath = "/.well-known/warder"

// discoveryDocument is the dashboard's answer.
type discoveryDocument struct {
	RuntimeURL string `json:"runtimeUrl"`
}

// DiscoverRuntimeURL resolves an address a person typed into the one the CLI
// should actually talk to.
//
// A deployment has two hosts and only one of them is the CLI's, which is our
// topology, not something a developer should have to learn to run their own
// application. Given the dashboard address they already have, this finds the
// runtime address for them. Given the runtime address directly, the lookup
// 404s and the seed is returned unchanged, so nothing that worked before
// stops working.
//
// Failure is never fatal: an unreachable or unparseable document returns the
// seed. The caller is about to try that address anyway, and a clear connection
// error from the real request beats an obscure one from a probe.
func DiscoverRuntimeURL(ctx context.Context, seed string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(seed), "/")
	if trimmed == "" {
		return seed
	}

	base, err := url.Parse(trimmed)
	if err != nil || base.Host == "" {
		return seed
	}

	// Short, because this is a probe on the way to somewhere else. A dashboard
	// that cannot answer in three seconds should not hold up a login.
	client := &http.Client{
		Timeout: 3 * time.Second,
		// A redirect here could walk the lookup to a host the operator never
		// named, and the answer decides where a password is about to be sent.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, trimmed+discoveryPath, nil)
	if err != nil {
		return seed
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "warder-cli/"+Version)

	resp, err := client.Do(req)
	if err != nil {
		return seed
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return seed
	}

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return seed
	}

	var doc discoveryDocument
	if err := json.Unmarshal(payload, &doc); err != nil {
		return seed
	}

	resolved, ok := acceptDiscovered(base, doc.RuntimeURL)
	if !ok {
		return seed
	}
	return resolved
}

// acceptDiscovered decides whether an advertised runtime address may be used.
//
// The document is fetched over the network and decides where a password is
// sent moments later, so it is treated as a suggestion to be checked, not an
// instruction. A dashboard reached over HTTPS cannot hand back a plaintext
// address: that would let anyone able to rewrite the response walk a login
// down to a channel they can read.
func acceptDiscovered(base *url.URL, advertised string) (string, bool) {
	candidate := strings.TrimRight(strings.TrimSpace(advertised), "/")
	if candidate == "" {
		return "", false
	}

	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Host == "" {
		return "", false
	}

	switch parsed.Scheme {
	case "https":
	case "http":
		// Allowed only when the dashboard itself was plaintext, which in
		// practice means someone developing against loopback.
		if base.Scheme != "http" {
			return "", false
		}
	default:
		return "", false
	}

	// Userinfo in an advertised address is either a mistake or an attempt to
	// smuggle a credential into a command line.
	parsed.User = nil

	return strings.TrimRight(parsed.String(), "/"), true
}
