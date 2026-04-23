package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"

	"csgclaw/cli/command"
	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
)

type HTTPClient = apiclient.HTTPClient

type APIClient struct {
	*apiclient.Client
}

func NewAPIClient(endpoint, token string, client HTTPClient) *APIClient {
	return &APIClient{Client: apiclient.New(endpoint, token, client)}
}

func (c *APIClient) ListAgents(ctx context.Context) ([]apitypes.Agent, error) {
	var agents []apitypes.Agent
	if err := c.GetJSON(ctx, "/api/v1/agents", &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

func (c *APIClient) GetAgent(ctx context.Context, id string) (apitypes.Agent, error) {
	var got apitypes.Agent
	if err := c.GetJSON(ctx, "/api/v1/agents/"+id, &got); err != nil {
		return apitypes.Agent{}, err
	}
	return got, nil
}

func (c *APIClient) CreateAgent(ctx context.Context, req apitypes.CreateAgentRequest) (apitypes.Agent, error) {
	var created apitypes.Agent
	if err := c.DoJSON(ctx, http.MethodPost, "/api/v1/agents", req, &created); err != nil {
		return apitypes.Agent{}, err
	}
	return created, nil
}

func (c *APIClient) DeleteAgent(ctx context.Context, id string) error {
	return c.DoNoContent(ctx, http.MethodDelete, "/api/v1/agents/"+id)
}

func (c *APIClient) StreamAgentLogs(ctx context.Context, id string, follow bool, lines int, w io.Writer) error {
	values := url.Values{}
	if follow {
		values.Set("follow", "true")
	}
	apiclient.QueryInt(values, "lines", lines)
	return c.Stream(ctx, "/api/v1/agents/"+id+"/logs", values, w)
}

func (c *APIClient) CreateFeishuUser(ctx context.Context, req channel.FeishuCreateUserRequest) (apitypes.User, error) {
	var created apitypes.User
	if err := c.DoJSON(ctx, http.MethodPost, "/api/v1/channels/feishu/users", req, &created); err != nil {
		return apitypes.User{}, err
	}
	return created, nil
}

func (c *APIClient) DeleteUser(ctx context.Context, channel, id string) error {
	return c.Client.DeleteUser(ctx, channel, id)
}

func extractAPIError(resp *http.Response) error {
	return apiclient.ExtractAPIError(resp)
}

func extractAPIErrorMessage(body []byte) string {
	return apiclient.ExtractAPIErrorMessage(body)
}

func writeJSON(w io.Writer, v any) error {
	return command.WriteJSON(w, v)
}

func renderAgentsTable(w io.Writer, agents []apitypes.Agent) error {
	tw := newTableWriter(w)
	fmt.Fprintln(tw, "ID\tNAME\tROLE\tSTATUS\tPROFILE")
	for _, a := range agents {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", a.ID, a.Name, a.Role, a.Status, displayAgentProfile(a.Profile))
	}
	return tw.Flush()
}

func renderBotsTable(w io.Writer, bots []apitypes.Bot) error {
	return command.RenderBotsTable(w, bots)
}

func displayAgentProfile(profile string) string {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return "-"
	}
	return profile
}

func renderRoomsTable(w io.Writer, rooms []apitypes.Room) error {
	return command.RenderRoomsTable(w, rooms)
}

func renderUsersTable(w io.Writer, users []apitypes.User) error {
	return command.RenderUsersTable(w, users)
}

func newTableWriter(w io.Writer) *tabwriter.Writer {
	return command.NewTableWriter(w)
}
