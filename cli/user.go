package cli

import (
	"context"
	"flag"
	"fmt"

	"csgclaw/internal/im"
)

func (a *App) runUser(ctx context.Context, args []string, globals GlobalOptions) error {
	if len(args) == 0 {
		return fmt.Errorf("user subcommand is required")
	}

	switch args[0] {
	case "list":
		return a.runUserList(ctx, args[1:], globals)
	case "kick":
		return a.runUserKick(ctx, args[1:], globals)
	default:
		return fmt.Errorf("unknown user subcommand %q", args[0])
	}
}

func (a *App) runUserList(ctx context.Context, args []string, globals GlobalOptions) error {
	fs := flag.NewFlagSet("user list", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("user list does not accept positional arguments")
	}

	client := NewAPIClient(globals.Endpoint, globals.Token, a.httpClient)
	users, err := client.ListUsers(ctx)
	if err != nil {
		return err
	}
	return a.renderUsers(globals.Output, users)
}

func (a *App) runUserKick(ctx context.Context, args []string, globals GlobalOptions) error {
	fs := flag.NewFlagSet("user kick", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("user kick requires exactly one id")
	}

	client := NewAPIClient(globals.Endpoint, globals.Token, a.httpClient)
	return client.KickUser(ctx, rest[0])
}

func (a *App) renderUsers(output string, users []im.User) error {
	switch output {
	case "", "table":
		return renderUsersTable(a.stdout, users)
	case "json":
		return writeJSON(a.stdout, users)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}
