package cli

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// UnreachableError reports that the Warder server did not answer.
//
// A typed error rather than a formatted string, because the advice worth
// printing depends on where the address came from, and only the caller knows
// that: a machine token names its own URL, while a developer's stored login
// carries whatever `ward login` was pointed at, which is loopback unless they
// said otherwise.
type UnreachableError struct {
	// BaseURL is the address that did not answer, already redacted.
	BaseURL string
	// Source names where the address came from, e.g. "WARDER_RUNTIME_URL".
	// Empty means the stored login.
	Source string
	// Loopback reports whether BaseURL points at this machine.
	Loopback bool
}

func (e *UnreachableError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "could not reach %s", e.BaseURL)

	switch {
	case e.Loopback:
		// The overwhelmingly common case, and the one the old message left
		// people to work out for themselves: `ward login` defaults to loopback,
		// so anyone who signed in without --url is pointed at a Warder on their
		// own machine whether they meant to be or not.
		b.WriteString("\n\nThat address is this machine, and nothing is listening on it.")
		if e.Source == "" {
			b.WriteString("\nIt is the default, so `ward login` used it unless you passed --url.")
		}
		b.WriteString("\n\nIf your Warder is deployed, point ward at it:")
		b.WriteString("\n    ward login --url https://<your-runtime-host>")
		b.WriteString("\n\nIf you meant to run one locally, start it first:")
		b.WriteString("\n    warder-api serve")
	case e.Source != "":
		fmt.Fprintf(&b, "\n\n%s names that address. Check the runtime service is\nrunning and reachable from here.", e.Source)
	default:
		b.WriteString("\n\nThat is the address you signed in to. Check the runtime service is")
		b.WriteString("\nrunning and reachable from here, then run `ward login` again if it moved.")
	}

	b.WriteString("\n\nCurrent settings: ward status")
	return b.String()
}

// redactURL removes anything credential-shaped from a URL before it is printed.
//
// An address can carry userinfo (https://user:password@host), and a URL that
// reaches an error message reaches stderr, CI logs, and screenshots. The old
// message printed the base URL verbatim.
func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		// Unparseable: say nothing rather than echo something unexamined.
		return "the configured server"
	}
	parsed.User = nil
	return parsed.String()
}

// isLoopbackURL reports whether a URL addresses this machine.
func isLoopbackURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// sameServer reports whether two URLs address the same Warder, ignoring
// differences that do not change which host answers.
func sameServer(a, b string) bool {
	parsedA, errA := url.Parse(strings.TrimRight(a, "/"))
	parsedB, errB := url.Parse(strings.TrimRight(b, "/"))
	if errA != nil || errB != nil {
		return strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(b, "/"))
	}
	return strings.EqualFold(parsedA.Scheme, parsedB.Scheme) &&
		strings.EqualFold(parsedA.Host, parsedB.Host) &&
		strings.TrimRight(parsedA.Path, "/") == strings.TrimRight(parsedB.Path, "/")
}
