package cli

import (
	"context"
	"flag"
	"fmt"

	"csgclaw/internal/agent"
)

func (a *App) runAgent(ctx context.Context, args []string, globals GlobalOptions) error {
	if len(args) == 0 {
		return fmt.Errorf("agent subcommand is required")
	}

	switch args[0] {
	case "status":
		return a.runAgentStatus(ctx, args[1:], globals)
	default:
		return fmt.Errorf("unknown agent subcommand %q", args[0])
	}
}

func (a *App) runAgentStatus(ctx context.Context, args []string, globals GlobalOptions) error {
	fs := flag.NewFlagSet("agent status", flag.ContinueOnError)
	fs.SetOutput(a.stderr)

	filter := fs.String("filter", "", "filter by state")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := NewAPIClient(globals.Endpoint, globals.Token, a.httpClient)
	rest := fs.Args()
	if len(rest) > 1 {
		return fmt.Errorf("agent status accepts at most one id")
	}

	if len(rest) == 1 {
		got, err := client.GetAgent(ctx, rest[0])
		if err != nil {
			return err
		}
		return a.renderAgents(globals.Output, []agent.Agent{got})
	}

	agents, err := client.ListAgents(ctx)
	if err != nil {
		return err
	}
	if *filter != "" {
		agents = filterAgentsByStatus(agents, *filter)
	}
	return a.renderAgents(globals.Output, agents)
}

func (a *App) renderAgents(output string, agents []agent.Agent) error {
	switch output {
	case "", "table":
		return renderAgentsTable(a.stdout, agents)
	case "json":
		return writeJSON(a.stdout, agents)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func filterAgentsByStatus(agents []agent.Agent, status string) []agent.Agent {
	filtered := make([]agent.Agent, 0, len(agents))
	for _, a := range agents {
		if a.Status == status {
			filtered = append(filtered, a)
		}
	}
	return filtered
}
