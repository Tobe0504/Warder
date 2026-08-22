package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// commandOrganization implements `ward org`.
func commandOrganization(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ward org list | ward org use <slug>")
	}

	switch args[0] {
	case "list", "ls":
		return organizationList(ctx)
	case "use", "switch":
		return organizationUse(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command: ward org %s\n\nUsage: ward org list | ward org use <slug>", args[0])
	}
}

func organizationList(ctx context.Context) error {
	creds, client, err := requireLogin()
	if err != nil {
		return err
	}

	orgs, err := client.ListOrganizations(ctx, creds.SessionToken)
	if err != nil {
		return err
	}
	if len(orgs) == 0 {
		fmt.Fprintln(os.Stderr, "You are not a member of any organization.")
		return nil
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "\tSLUG\tNAME\tROLE")
	for _, org := range orgs {
		marker := " "
		if org.Current {
			marker = "*"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", marker, org.Slug, org.Name, strings.ToLower(org.Role))
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "\nSwitch with: ward org use <slug>")
	return nil
}

func organizationUse(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("org use", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return errors.New("which organization?\n\nUsage: ward org use <slug>\nList them with: ward org list")
	}

	wanted := strings.TrimSpace(flags.Arg(0))

	creds, client, err := requireLogin()
	if err != nil {
		return err
	}

	result, err := client.SwitchOrganization(ctx, creds.SessionToken, wanted)
	if err != nil {
		return err
	}

	if result.Unchanged {
		fmt.Fprintf(os.Stderr, "Already in %s (%s).\n", result.Organization, result.Slug)
		return nil
	}

	// The server revoked the old session when it minted this one, so the
	// stored credential is already dead: writing the new one is not an
	// optimisation, it is the only thing keeping this machine signed in.
	expiresAt, _ := time.Parse(time.RFC3339, result.ExpiresAt)
	updated := *creds
	updated.SessionToken = result.SessionToken
	updated.ExpiresAt = expiresAt
	updated.Organization = result.Organization

	if err := SaveCredentials(&updated); err != nil {
		return fmt.Errorf(
			"switched to %s, but the new session could not be saved: %w\n\nRun `ward login`",
			result.Organization, err)
	}

	fmt.Fprintf(os.Stderr, "Now in %s (%s) as %s.\n",
		result.Organization, result.Slug, strings.ToLower(result.Role))

	// A project file naming a project in the organization just left would send
	// every later command somewhere it cannot reach, and the error would name
	// the project rather than the switch that caused it.
	if cfg, err := LoadProjectConfig(); err == nil && cfg != nil && cfg.Project != "" {
		fmt.Fprintf(os.Stderr,
			"\nThis directory targets project %q. If that project belongs to another\norganization, run `ward init` here again.\n",
			cfg.Project)
	}
	return nil
}
