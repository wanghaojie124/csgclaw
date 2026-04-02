package cli

import (
	"context"
	"flag"
	"fmt"

	"csgclaw/internal/agent"
)

func (a *App) runAgent(ctx context.Context, args []string, globals GlobalOptions) error {
	if len(args) == 0 {
		a.usageCommandGroup("agent", "Manage agents.", "csgclaw agent <subcommand> [flags]", []string{
			"list               List agents",
			"create             Create an agent",
			"delete <id>        Delete an agent",
			"status [id]        Show one agent or list all agents",
		})
		return flag.ErrHelp
	}
	if isHelpArg(args[0]) {
		a.usageCommandGroup("agent", "Manage agents.", "csgclaw agent <subcommand> [flags]", []string{
			"list               List agents",
			"create             Create an agent",
			"delete <id>        Delete an agent",
			"status [id]        Show one agent or list all agents",
		})
		return flag.ErrHelp
	}

	switch args[0] {
	case "list":
		return a.runAgentList(ctx, args[1:], globals)
	case "create":
		return a.runAgentCreate(ctx, args[1:], globals)
	case "delete":
		return a.runAgentDelete(ctx, args[1:], globals)
	case "status":
		return a.runAgentStatus(ctx, args[1:], globals)
	default:
		a.usageCommandGroup("agent", "Manage agents.", "csgclaw agent <subcommand> [flags]", []string{
			"list               List agents",
			"create             Create an agent",
			"delete <id>        Delete an agent",
			"status [id]        Show one agent or list all agents",
		})
		return fmt.Errorf("unknown agent subcommand %q", args[0])
	}
}

func (a *App) runAgentList(ctx context.Context, args []string, globals GlobalOptions) error {
	fs := a.newCommandFlagSet("agent list", "csgclaw agent list [flags]", "List agents.")
	filter := fs.String("filter", "", "filter by state")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("agent list does not accept positional arguments")
	}

	client := NewAPIClient(globals.Endpoint, globals.Token, a.httpClient)
	agents, err := client.ListAgents(ctx)
	if err != nil {
		return err
	}
	if *filter != "" {
		agents = filterAgentsByStatus(agents, *filter)
	}
	return a.renderAgents(globals.Output, agents)
}

func (a *App) runAgentCreate(ctx context.Context, args []string, globals GlobalOptions) error {
	fs := a.newCommandFlagSet("agent create", "csgclaw agent create [flags]", "Create an agent.")
	id := fs.String("id", "", "agent id")
	name := fs.String("name", "", "agent name")
	description := fs.String("description", "", "agent description")
	modelID := fs.String("model-id", "", "agent model identifier")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("agent create does not accept positional arguments")
	}

	client := NewAPIClient(globals.Endpoint, globals.Token, a.httpClient)
	created, err := client.CreateAgent(ctx, agent.CreateRequest{
		ID:          *id,
		Name:        *name,
		Description: *description,
		ModelID:     *modelID,
	})
	if err != nil {
		return err
	}
	return a.renderAgents(globals.Output, []agent.Agent{created})
}

func (a *App) runAgentDelete(ctx context.Context, args []string, globals GlobalOptions) error {
	fs := a.newCommandFlagSet("agent delete", "csgclaw agent delete <id> [flags]", "Delete an agent.")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("agent delete requires exactly one id")
	}

	client := NewAPIClient(globals.Endpoint, globals.Token, a.httpClient)
	return client.DeleteAgent(ctx, rest[0])
}

func (a *App) runAgentStatus(ctx context.Context, args []string, globals GlobalOptions) error {
	fs := a.newCommandFlagSet("agent status", "csgclaw agent status [id] [flags]", "Show one agent or list all agents.")
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

	return a.runAgentList(ctx, args, globals)
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
