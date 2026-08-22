package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"
)

// Version is the CLI version, reported by `ward version` and in the User-Agent.
//
// A var rather than a const so that the release build can stamp the real tag
// with `-ldflags -X`. Linker flags cannot set a constant: the build would
// succeed and every released binary would quietly claim to be this value.
var Version = "0.1.0-dev"

// nowFunc is overridable in tests.
var nowFunc = time.Now

// defaultRuntimeURL is used when nothing else specifies one.
const defaultRuntimeURL = "http://127.0.0.1:8081"

// Execute dispatches a command.
func Execute(ctx context.Context, args []string) error {
	if len(args) == 0 {
		Usage()
		return nil
	}

	switch args[0] {
	case "init":
		return commandInit(ctx, args[1:])
	case "login":
		return commandLogin(ctx, args[1:])
	case "logout":
		return commandLogout(ctx, args[1:])
	case "status", "whoami":
		return commandStatus(ctx, args[1:])
	case "project", "projects":
		return commandProject(ctx, args[1:])
	case "environment", "environments", "env":
		return commandEnvironment(ctx, args[1:])
	case "secret", "secrets":
		return commandSecret(ctx, args[1:])
	case "run":
		return commandRun(ctx, args[1:])
	case "version", "--version", "-v":
		fmt.Println("ward " + Version)
		return nil
	case "help", "--help", "-h":
		Usage()
		return nil
	default:
		Usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// Usage prints the command overview.
func Usage() {
	fmt.Fprint(os.Stderr, `ward: run applications with the credentials they need, without holding them yourself

Usage:
  ward init                     Point this directory at a project and environment
  ward login                    Sign in and store a session for this machine
  ward logout                   Sign out and remove the stored session
  ward status                   Show who you are signed in as
  ward project list             List projects you can see
  ward environment list         List environments in a project
  ward secret list              List secret names in an environment (names only)
  ward run -- <command>         Run a command with authorized secrets injected

Examples:
  ward run -- npm run dev
  ward run --env production -- ./server
  ward run --key DATABASE_URL --key REDIS_URL -- npm test

Configuration:
  .warder.json in your project names the project and environment. It contains
  no credentials and is safe to commit.

  For machine runtimes, set WARDER_RUNTIME_URL and WARDER_TOKEN in the runtime's own
  environment instead of signing in.

Secret values are never printed, never written to disk, and never placed in
command-line arguments.
`)
}

// ---------------------------------------------------------------------------
// init
// ---------------------------------------------------------------------------

// commandInit writes .warder.json, naming the project and environment this
// working tree runs against.
//
// The file is meant to be committed, so that a team shares one target and
// nobody has to pass flags. It holds no credential, only two names: which is
// the reason it can be committed at all, and the reason the writer refuses to
// put anything else in it.
func commandInit(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	project := flags.String("project", "", "Project slug")
	environment := flags.String("env", "development", "Environment slug")
	runtimeURL := flags.String("url", "", "Warder runtime URL for this project")
	force := flags.Bool("force", false, "Overwrite an existing .warder.json")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *project == "" {
		return errors.New("which project?\n\nUsage: ward init --project <slug> [--env <slug>]")
	}

	const filename = ".warder.json"
	if _, err := os.Stat(filename); err == nil && !*force {
		return fmt.Errorf("%s already exists\n\nPass --force to replace it", filename)
	}

	// The runtime URL belongs in the committed file, not in every teammate's
	// shell. It is an address, not a credential: knowing where Warder lives
	// gets nobody a secret, and the alternative is each developer discovering
	// it by asking whoever deployed it.
	// Resolved here rather than stored raw, so the committed file names the
	// runtime address whichever address was passed. Everything downstream then
	// compares like with like.
	resolved := strings.TrimSpace(*runtimeURL)
	if resolved != "" {
		resolved = DiscoverRuntimeURL(ctx, resolved)
	}

	cfg := ProjectConfig{
		Project:     *project,
		Environment: *environment,
		RuntimeURL:  resolved,
	}
	encoded, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode the configuration: %w", err)
	}
	encoded = append(encoded, '\n')

	// Written world-readable on purpose: this file is committed and read by
	// everyone on the team. Nothing in it is sensitive, and giving it a
	// restrictive mode would wrongly imply otherwise.
	if err := os.WriteFile(filename, encoded, 0o644); err != nil {
		return fmt.Errorf("could not write %s: %w", filename, err)
	}

	fmt.Fprintf(os.Stderr, "Wrote %s for %s/%s.\n", filename, *project, *environment)
	if cfg.RuntimeURL != "" {
		fmt.Fprintf(os.Stderr, "Server: %s\n", redactURL(cfg.RuntimeURL))
	}
	fmt.Fprint(os.Stderr,
		"\nThis file holds no credentials and is safe to commit.\nNext: ward login, then ward run -- <your command>\n")
	return nil
}

// ---------------------------------------------------------------------------
// login / logout / status
// ---------------------------------------------------------------------------

func commandLogin(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	apiURL := flags.String("url", "", "Warder runtime URL")
	email := flags.String("email", "", "Your email address")
	if err := flags.Parse(args); err != nil {
		return err
	}

	// Precedence, most explicit first. The project file sits above the default
	// so that someone who clones a repository and runs `ward login` reaches the
	// team's Warder rather than a loopback address with nothing behind it.
	url := strings.TrimSpace(*apiURL)
	if url == "" {
		url = strings.TrimSpace(os.Getenv("WARDER_RUNTIME_URL"))
	}
	if url == "" {
		if cfg, err := LoadProjectConfig(); err == nil && cfg != nil {
			url = strings.TrimSpace(cfg.RuntimeURL)
		}
	}
	if url == "" {
		url = defaultRuntimeURL
	}

	// A dashboard address resolves to the runtime address behind it, so nobody
	// has to know there are two. Given the runtime address already, this is a
	// lookup that 404s and changes nothing.
	url = DiscoverRuntimeURL(ctx, url)

	// Printed before the password is asked for, not after. This names the host
	// the password is about to be sent to, and after discovery that host is not
	// necessarily the one that was typed: a reader who is about to type a
	// credential is entitled to see where it goes while they can still stop.
	fmt.Fprintf(os.Stderr, "Signing in to %s\n", redactURL(url))

	address := *email
	if address == "" {
		fmt.Fprint(os.Stderr, "Email: ")
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("could not read your email address")
		}
		address = strings.TrimSpace(line)
	}

	password, err := readPassword("Password: ")
	if err != nil {
		return err
	}

	client := NewClientFrom(url, loginURLSource(*apiURL))
	result, err := client.Login(ctx, address, password)

	// The password is cleared as soon as it has been used. Best effort: Go
	// strings are immutable, so this only shortens the window for the copy we
	// control.
	password = ""

	if err != nil {
		return err
	}

	expiresAt, _ := time.Parse(time.RFC3339, result.ExpiresAt)
	if err := SaveCredentials(&Credentials{
		APIURL:       url,
		RuntimeURL:   url,
		SessionToken: result.SessionToken,
		ExpiresAt:    expiresAt,
		Email:        result.User.Email,
		Organization: result.User.Organization,
	}); err != nil {
		return err
	}

	path, _ := credentialsPath()
	fmt.Fprintf(os.Stderr, "Signed in as %s (%s).\nSession stored in %s, readable only by you.\n",
		result.User.Email, result.User.Organization, path)
	return nil
}

// readPassword reads without echoing.
//
// Reading from the terminal directly keeps the password out of the shell's
// history and out of the process's argument vector. There is deliberately no
// --password flag: providing one would be a convenience that writes the
// password into `ps` output and the shell history file.
func readPassword(prompt string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Not a terminal: read a single line, so the command still works in a
		// pipeline, without echoing anything we control.
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", errors.New("could not read the password")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	fmt.Fprint(os.Stderr, prompt)
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", errors.New("could not read the password")
	}
	return string(raw), nil
}

func commandLogout(ctx context.Context, _ []string) error {
	creds, err := LoadCredentials()
	if err != nil && !errors.Is(err, ErrNotLoggedIn) {
		return err
	}

	if creds != nil && creds.SessionToken != "" {
		// Best effort: the local credential is removed regardless, but revoking
		// server-side is what actually ends the session. A local delete alone
		// would leave a working credential behind if the file were recovered.
		client := NewClient(runtimeURLFor(creds))
		if err := client.post(ctx, "/cli/logout", creds.SessionToken, nil, nil); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not revoke the session on the server: %v\n", err)
		}
	}

	if err := ClearCredentials(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Signed out.")
	return nil
}

func commandStatus(_ context.Context, _ []string) error {
	creds, err := LoadCredentials()
	if err != nil {
		if errors.Is(err, ErrNotLoggedIn) {
			fmt.Fprintln(os.Stderr, "Not signed in. Run `ward login`.")
			return nil
		}
		return err
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(writer, "Signed in as\t%s\n", creds.Email)
	fmt.Fprintf(writer, "Organization\t%s\n", creds.Organization)
	fmt.Fprintf(writer, "Server\t%s\n", runtimeURLFor(creds))

	if creds.Expired(nowFunc()) {
		fmt.Fprintf(writer, "Session\texpired: run `ward login`\n")
	} else {
		fmt.Fprintf(writer, "Session\tvalid until %s\n", creds.ExpiresAt.Local().Format(time.RFC1123))
	}

	if cfg, err := LoadProjectConfig(); err == nil && cfg != nil {
		fmt.Fprintf(writer, "Project\t%s\n", cfg.Project)
		fmt.Fprintf(writer, "Environment\t%s\n", cfg.Environment)
	}
	return writer.Flush()
}

// loginURLSource names where a login URL came from, for error messages.
func loginURLSource(flagValue string) string {
	if strings.TrimSpace(flagValue) != "" {
		return "--url"
	}
	if strings.TrimSpace(os.Getenv("WARDER_RUNTIME_URL")) != "" {
		return "WARDER_RUNTIME_URL"
	}
	if cfg, err := LoadProjectConfig(); err == nil && cfg != nil && strings.TrimSpace(cfg.RuntimeURL) != "" {
		return ".warder.json"
	}
	return ""
}

func runtimeURLFor(creds *Credentials) string {
	if creds.RuntimeURL != "" {
		return creds.RuntimeURL
	}
	if creds.APIURL != "" {
		return creds.APIURL
	}
	return defaultRuntimeURL
}

// ---------------------------------------------------------------------------
// project / environment / secret listings
// ---------------------------------------------------------------------------

func requireLogin() (*Credentials, *Client, error) {
	creds, err := LoadCredentials()
	if err != nil {
		if errors.Is(err, ErrNotLoggedIn) {
			return nil, nil, errors.New("not signed in\n\nRun `ward login`")
		}
		return nil, nil, err
	}
	if creds.Expired(nowFunc()) {
		return nil, nil, errors.New("your session has expired\n\nRun `ward login`")
	}
	return creds, NewClient(runtimeURLFor(creds)), nil
}

func commandProject(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "list" {
		return errors.New("usage: ward project list")
	}

	creds, client, err := requireLogin()
	if err != nil {
		return err
	}

	result, err := client.ListProjects(ctx, creds.SessionToken)
	if err != nil {
		return err
	}
	if len(result.Projects) == 0 {
		fmt.Fprintln(os.Stderr, "No projects.")
		return nil
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "SLUG\tNAME")
	for _, p := range result.Projects {
		fmt.Fprintf(writer, "%s\t%s\n", p.Slug, p.Name)
	}
	return writer.Flush()
}

func commandEnvironment(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment", flag.ContinueOnError)
	project := flags.String("project", "", "Project slug")
	if len(args) == 0 || args[0] != "list" {
		return errors.New("usage: ward environment list [--project <slug>]")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	creds, client, err := requireLogin()
	if err != nil {
		return err
	}

	slug := *project
	if slug == "" {
		cfg, err := LoadProjectConfig()
		if err != nil {
			return err
		}
		if cfg == nil || cfg.Project == "" {
			return errors.New("no project specified\n\nUse --project <slug>, or add .warder.json to this directory")
		}
		slug = cfg.Project
	}

	projectID, err := resolveProjectID(ctx, client, creds.SessionToken, slug)
	if err != nil {
		return err
	}

	result, err := client.ListEnvironments(ctx, creds.SessionToken, projectID)
	if err != nil {
		return err
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "SLUG\tNAME")
	for _, e := range result.Environments {
		fmt.Fprintf(writer, "%s\t%s\n", e.Slug, e.Name)
	}
	return writer.Flush()
}

// commandSecret lists secret names.
//
// There is no `ward secret reveal`. Revealing a value requires READ_SECRET and
// happens in the dashboard, where it can be confirmed deliberately and recorded
// against a person. A CLI subcommand that printed a credential to a terminal
// would put it in scrollback, in tmux buffers, in terminal recordings, and in
// the transcript of any AI agent running the command.
func commandSecret(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("secret", flag.ContinueOnError)
	project := flags.String("project", "", "Project slug")
	environment := flags.String("env", "", "Environment slug")
	if len(args) == 0 || args[0] != "list" {
		return errors.New("usage: ward secret list [--project <slug>] [--env <slug>]")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	creds, client, err := requireLogin()
	if err != nil {
		return err
	}

	projectSlug, environmentSlug := *project, *environment
	if projectSlug == "" || environmentSlug == "" {
		cfg, err := LoadProjectConfig()
		if err != nil {
			return err
		}
		if cfg != nil {
			if projectSlug == "" {
				projectSlug = cfg.Project
			}
			if environmentSlug == "" {
				environmentSlug = cfg.Environment
			}
		}
	}
	if projectSlug == "" || environmentSlug == "" {
		return errors.New("specify --project and --env, or add .warder.json to this directory")
	}

	projectID, err := resolveProjectID(ctx, client, creds.SessionToken, projectSlug)
	if err != nil {
		return err
	}
	environments, err := client.ListEnvironments(ctx, creds.SessionToken, projectID)
	if err != nil {
		return err
	}

	environmentID := ""
	for _, e := range environments.Environments {
		if e.Slug == environmentSlug {
			environmentID = e.ID
			break
		}
	}
	if environmentID == "" {
		return fmt.Errorf("no environment %q in project %q", environmentSlug, projectSlug)
	}

	result, err := client.ListSecrets(ctx, creds.SessionToken, environmentID)
	if err != nil {
		return err
	}
	if len(result.Secrets) == 0 {
		fmt.Fprintln(os.Stderr, "No secrets in this environment.")
		return nil
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "KEY\tVALUE\tSTATUS\tVERSION\tEXPIRES\tYOU CAN USE")
	for _, s := range result.Secrets {
		version := "-"
		if s.Version != nil {
			version = fmt.Sprintf("v%d", *s.Version)
		}
		expires := s.ExpiresAt
		if expires == "" {
			expires = "never"
		}
		canUse := "no"
		if s.CanUse {
			canUse = "yes"
		}
		// The value column always shows the mask. It is printed rather than
		// omitted so that the distinction the product is built on is visible
		// every time someone runs this: the name is yours to see, the value
		// is not.
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Key, s.Masked, s.Status, version, expires, canUse)
	}
	return writer.Flush()
}

func resolveProjectID(ctx context.Context, client *Client, sessionToken, slug string) (string, error) {
	projects, err := client.ListProjects(ctx, sessionToken)
	if err != nil {
		return "", err
	}
	for _, p := range projects.Projects {
		if p.Slug == slug {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("no project %q", slug)
}

// ---------------------------------------------------------------------------
// run
// ---------------------------------------------------------------------------

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func commandRun(ctx context.Context, args []string) error {
	// Everything after -- belongs to the child command and must not be parsed
	// as a flag of ours.
	var ours, theirs []string
	separatorFound := false
	for i, arg := range args {
		if arg == "--" {
			ours, theirs = args[:i], args[i+1:]
			separatorFound = true
			break
		}
	}
	if !separatorFound {
		return errors.New("missing -- separator\n\nUsage: ward run [flags] -- <command> [args...]")
	}

	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	project := flags.String("project", "", "Project slug")
	environment := flags.String("env", "", "Environment slug")
	quiet := flags.Bool("quiet", false, "Suppress the summary line")
	var keys stringList
	flags.Var(&keys, "key", "Request only this secret (repeatable)")

	if err := flags.Parse(ours); err != nil {
		return err
	}

	return Run(ctx, runOptions{
		Project:     *project,
		Environment: *environment,
		Keys:        keys,
		Command:     theirs,
		Quiet:       *quiet,
	})
}
