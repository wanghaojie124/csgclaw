package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"csgclaw/internal/agent"
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
		endpoint = "http://127.0.0.1:18080"
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

func (c *APIClient) getJSON(ctx context.Context, path string, out any) error {
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("http %s: %s", resp.Status, msg)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func renderAgentsTable(w io.Writer, agents []agent.Agent) error {
	var buf bytes.Buffer
	fmt.Fprintln(&buf, "ID\tNAME\tROLE\tSTATUS")
	for _, a := range agents {
		fmt.Fprintf(&buf, "%s\t%s\t%s\t%s\n", a.ID, a.Name, a.Role, a.Status)
	}
	_, err := w.Write(buf.Bytes())
	return err
}
