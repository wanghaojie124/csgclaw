package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type APIClient struct {
	endpoint string
	token    string
	client   HTTPClient
}

func NewAPIClient(endpoint, token string, client HTTPClient) *APIClient {
	if endpoint == "" {
		endpoint = config.DefaultAPIBaseURL()
	}
	return &APIClient{
		endpoint: strings.TrimRight(endpoint, "/"),
		token:    token,
		client:   client,
	}
}

func (c *APIClient) ListAgents(ctx context.Context) ([]agent.Agent, error) {
	var agents []agent.Agent
	if err := c.getJSON(ctx, "/api/v1/agents", &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

func (c *APIClient) GetAgent(ctx context.Context, id string) (agent.Agent, error) {
	var got agent.Agent
	if err := c.getJSON(ctx, "/api/v1/agents/"+id, &got); err != nil {
		return agent.Agent{}, err
	}
	return got, nil
}

func (c *APIClient) CreateAgent(ctx context.Context, req agent.CreateRequest) (agent.Agent, error) {
	var created agent.Agent
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/agents", req, &created); err != nil {
		return agent.Agent{}, err
	}
	return created, nil
}

func (c *APIClient) DeleteAgent(ctx context.Context, id string) error {
	return c.doNoContent(ctx, http.MethodDelete, "/api/v1/agents/"+id)
}

func (c *APIClient) StreamAgentLogs(ctx context.Context, id string, follow bool, lines int, w io.Writer) error {
	values := url.Values{}
	if follow {
		values.Set("follow", "true")
	}
	if lines > 0 {
		values.Set("lines", strconv.Itoa(lines))
	}

	path := "/api/v1/agents/" + id + "/logs"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path, nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return extractAPIError(resp)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

func (c *APIClient) ListRooms(ctx context.Context) ([]im.Room, error) {
	var rooms []im.Room
	if err := c.getJSON(ctx, "/api/v1/rooms", &rooms); err != nil {
		return nil, err
	}
	return rooms, nil
}

func (c *APIClient) CreateRoom(ctx context.Context, req im.CreateRoomRequest) (im.Room, error) {
	var created im.Room
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/rooms", req, &created); err != nil {
		return im.Room{}, err
	}
	return created, nil
}

func (c *APIClient) DeleteRoom(ctx context.Context, id string) error {
	return c.doNoContent(ctx, http.MethodDelete, "/api/v1/rooms/"+id)
}

func (c *APIClient) ListUsers(ctx context.Context) ([]im.User, error) {
	var users []im.User
	if err := c.getJSON(ctx, "/api/v1/users", &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (c *APIClient) KickUser(ctx context.Context, id string) error {
	return c.doNoContent(ctx, http.MethodDelete, "/api/v1/users/"+id)
}

func (c *APIClient) getJSON(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, out)
}

func (c *APIClient) doNoContent(ctx context.Context, method, path string) error {
	return c.doJSON(ctx, method, path, nil, nil)
}

func (c *APIClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
		reader = &buf
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return extractAPIError(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	return nil
}

func extractAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if msg := extractAPIErrorMessage(body); msg != "" {
		return fmt.Errorf("%s", msg)
	}
	return fmt.Errorf("request failed")
}

func extractAPIErrorMessage(body []byte) string {
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		for _, key := range []string{"error", "message"} {
			if value, ok := payload[key].(string); ok {
				value = strings.TrimSpace(value)
				if value != "" {
					return value
				}
			}
		}
	}

	return msg
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func renderAgentsTable(w io.Writer, agents []agent.Agent) error {
	tw := newTableWriter(w)
	fmt.Fprintln(tw, "ID\tNAME\tROLE\tSTATUS")
	for _, a := range agents {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.ID, a.Name, a.Role, a.Status)
	}
	return tw.Flush()
}

func renderRoomsTable(w io.Writer, rooms []im.Room) error {
	tw := newTableWriter(w)
	fmt.Fprintln(tw, "ID\tTITLE\tPARTICIPANTS\tMESSAGES")
	for _, room := range rooms {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\n", room.ID, room.Title, len(room.Participants), len(room.Messages))
	}
	return tw.Flush()
}

func renderUsersTable(w io.Writer, users []im.User) error {
	tw := newTableWriter(w)
	fmt.Fprintln(tw, "ID\tNAME\tHANDLE\tROLE\tONLINE")
	for _, user := range users {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\n", user.ID, user.Name, user.Handle, user.Role, user.IsOnline)
	}
	return tw.Flush()
}

func newTableWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}
