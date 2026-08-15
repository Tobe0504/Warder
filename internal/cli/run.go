package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"
)

// runOptions configures `ward run`.
type runOptions struct {
	Project     string
	Environment string
	// Keys narrows the request to the secrets this command actually needs.
	Keys []string
	// Command is everything after the -- separator.
	Command []string
	Quiet   bool
}

// Run executes a command with authorized secrets injected into its environment.
//
// This is the command the product exists for. What it does not do is as
// important as what it does:
//
//   - It never prints a value, at any verbosity, in any error path.
//   - It never writes one to disk, and specifically never to .env.
//   - It never places one in an argument vector, where it would be visible in
//     `ps` output to every user on the machine.
//   - It passes values only through the child process's environment block,
//     which is inherited directly and is not visible to other users on a
//     correctly configured system.
//
// The honest limit, stated here and in the documentation: once the child has
// the values, the child has them. A process authorized to use a credential can
// read its own environment. The guarantee is that the developer who ran the
// command never had to know the credential, not that the program they started
// cannot see it.
func Run(ctx context.Context, opts runOptions) error {
	if len(opts.Command) == 0 {
		return errors.New("nothing to run\n\nUsage: ward run -- <command> [args...]")
	}

	credential, baseURL, source, err := resolveRuntimeCredential()
	if err != nil {
		return err
	}

	project, environment := opts.Project, opts.Environment
	if project == "" || environment == "" {
		// A committed .warder.json is the usual source, so that a team shares
		// one target without anyone passing flags.
		if cfg, err := LoadProjectConfig(); err == nil && cfg != nil {
			if project == "" {
				project = cfg.Project
			}
			if environment == "" {
				environment = cfg.Environment
			}
		} else if err != nil {
			return err
		}
	}

	client := NewClient(baseURL)

	// Step one: exchange the long-lived credential for a short-lived session.
	auth, err := client.RuntimeAuth(ctx, credential, project, environment)
	if err != nil {
		return err
	}

	// Step two: fetch only what was asked for.
	delivery, err := client.FetchSecrets(ctx, auth.AccessToken, opts.Keys)
	if err != nil {
		return err
	}

	if !opts.Quiet {
		// The summary names keys and counts. It never names a value, and there
		// is no flag that makes it do so.
		fmt.Fprintf(os.Stderr, "warder: %s/%s as %s (%s), %d secret(s) injected\n",
			auth.Project, auth.Environment, auth.Identity, auth.ActorType, len(delivery.Secrets))

		if len(delivery.Secrets) > 0 {
			keys := make([]string, 0, len(delivery.Secrets))
			for key := range delivery.Secrets {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			fmt.Fprintf(os.Stderr, "warder: %s\n", strings.Join(keys, ", "))
		}
		if len(delivery.Denied) > 0 {
			sort.Strings(delivery.Denied)
			fmt.Fprintf(os.Stderr, "warder: not authorized: %s\n", strings.Join(delivery.Denied, ", "))
		}
		if len(delivery.Unavailable) > 0 {
			sort.Strings(delivery.Unavailable)
			fmt.Fprintf(os.Stderr, "warder: unavailable (expired or revoked): %s\n",
				strings.Join(delivery.Unavailable, ", "))
		}
		if source != "" {
			fmt.Fprintf(os.Stderr, "warder: authenticated via %s\n", source)
		}
	}

	// If specific keys were requested and any were refused, stop rather than
	// starting a process that will fail confusingly several seconds later when
	// it tries to open a connection with a variable that is not set.
	if len(opts.Keys) > 0 && (len(delivery.Denied) > 0 || len(delivery.Unavailable) > 0) {
		return fmt.Errorf("not every requested secret is available; refusing to start %q", opts.Command[0])
	}

	return execute(ctx, opts.Command, delivery.Secrets)
}

// execute starts the child process with the secrets in its environment.
func execute(ctx context.Context, command []string, values map[string]string) error {
	// The parent's environment is inherited, with the delivered values layered
	// on top so that a secret from Warder wins over a stale one in the shell.
	environ := os.Environ()

	// Values are appended as KEY=value entries in the environment block. This
	// is not an argument vector: it is not visible in `ps`, and it is not
	// written anywhere.
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	filtered := environ[:0]
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := values[name]; !replaced {
			filtered = append(filtered, entry)
		}
	}
	environ = filtered

	for _, key := range keys {
		environ = append(environ, key+"="+values[key])
	}

	// The runtime credential is removed from the child's environment. The child
	// needs the secrets, not the ability to ask for more of them, and leaving
	// it in place would hand every dependency and every agent in that process
	// tree a credential they could use against the broker directly.
	//
	// VAULT_TOKEN is stripped too. It is the name this CLI used before, and a
	// deployment that still sets it would otherwise pass a live credential into
	// the child: the one place a leftover name is worth carrying.
	environ = withoutVars(environ, "WARDER_TOKEN", "VAULT_TOKEN")

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = environ
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		// The error names the command, never the environment it was given.
		return fmt.Errorf("could not start %q: %w", command[0], err)
	}

	// Signals are forwarded so that Ctrl-C reaches the child and it can shut
	// down cleanly, rather than the CLI exiting and orphaning it.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(signals)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	for {
		select {
		case sig := <-signals:
			if cmd.Process != nil {
				_ = cmd.Process.Signal(sig)
			}
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}
		case err := <-done:
			// The child's exit code is propagated so that `ward run -- npm
			// test` fails the build exactly as `npm test` would.
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			return err
		}
	}
}

func withoutVars(environ []string, names ...string) []string {
	out := environ[:0]
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if !contains(names, name) {
			out = append(out, entry)
		}
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}

// resolveRuntimeCredential finds the credential to authenticate with.
//
// A machine token in the environment takes precedence over a developer's stored
// login, so that the same command works unchanged in CI and in a container: the
// platform sets WARDER_TOKEN, and nothing else about the invocation changes.
func resolveRuntimeCredential() (credential, baseURL, source string, err error) {
	if token := strings.TrimSpace(os.Getenv("WARDER_TOKEN")); token != "" {
		url := strings.TrimSpace(os.Getenv("WARDER_RUNTIME_URL"))
		if url == "" {
			return "", "", "", errors.New("WARDER_TOKEN is set but WARDER_RUNTIME_URL is not")
		}
		return token, url, "WARDER_TOKEN", nil
	}

	creds, err := LoadCredentials()
	if err != nil {
		if errors.Is(err, ErrNotLoggedIn) {
			return "", "", "", errors.New(
				"not logged in\n\nRun `ward login`, or set WARDER_RUNTIME_URL and WARDER_TOKEN for a machine runtime")
		}
		return "", "", "", err
	}

	if creds.Expired(nowFunc()) {
		return "", "", "", errors.New("your login has expired\n\nRun `ward login`")
	}

	url := creds.RuntimeURL
	if url == "" {
		url = creds.APIURL
	}
	return creds.SessionToken, url, "", nil
}
