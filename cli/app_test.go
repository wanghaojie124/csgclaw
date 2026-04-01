package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteAgentStatusUsesHTTPClient(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}
			return jsonResponse(http.StatusOK, `[{"id":"u-alice","name":"alice","role":"worker","status":"running","created_at":"2026-04-01T12:00:00Z"}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "agent", "status"})
	if err != nil {
		t.Fatalf("Execute() error = %v; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id": "u-alice"`) {
		t.Fatalf("stdout = %q, want JSON agent payload", stdout.String())
	}
}

func TestExecuteAgentCreateUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}
			if got := req.Header.Get("Authorization"); got != "Bearer secret-token" {
				t.Fatalf("Authorization = %q, want %q", got, "Bearer secret-token")
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["name"] != "alice" {
				t.Fatalf("payload[name] = %#v, want %q", payload["name"], "alice")
			}
			if payload["description"] != "worker" {
				t.Fatalf("payload[description] = %#v, want %q", payload["description"], "worker")
			}
			if payload["model_id"] != "gpt-test" {
				t.Fatalf("payload[model_id] = %#v, want %q", payload["model_id"], "gpt-test")
			}

			return jsonResponse(http.StatusCreated, `{"id":"u-alice","name":"alice","role":"worker","status":"running","created_at":"2026-04-01T12:00:00Z"}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--token", "secret-token", "agent", "create", "--name", "alice", "--description", "worker", "--model-id", "gpt-test"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "u-alice\talice\tworker\trunning") {
		t.Fatalf("stdout = %q, want table row for created agent", stdout.String())
	}
}

func TestExecuteAgentDeleteUsesHTTPClient(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/agents/u-alice" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents/u-alice")
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Status:     http.StatusText(http.StatusNoContent),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "delete", "u-alice"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteAgentStatusByIDUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://example.test/api/v1/agents/u-alice" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents/u-alice")
			}
			return jsonResponse(http.StatusOK, `{"id":"u-alice","name":"alice","role":"worker","status":"running","created_at":"2026-04-01T12:00:00Z"}`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "status", "u-alice"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "u-alice\talice\tworker\trunning") {
		t.Fatalf("stdout = %q, want table row for u-alice", stdout.String())
	}
}

func TestExecuteRoomListUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/rooms" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/rooms")
			}
			return jsonResponse(http.StatusOK, `[{"id":"room-1","title":"alpha","participants":["u-admin","u-alice"],"messages":[{"id":"msg-1"}]}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "room", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "room-1\talpha\t2\t1") {
		t.Fatalf("stdout = %q, want table row for room", stdout.String())
	}
}

func TestExecuteRoomCreateUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/rooms" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/rooms")
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["title"] != "alpha" {
				t.Fatalf("payload[title] = %#v, want %q", payload["title"], "alpha")
			}
			if payload["creator_id"] != "u-admin" {
				t.Fatalf("payload[creator_id] = %#v, want %q", payload["creator_id"], "u-admin")
			}
			participantIDs, ok := payload["participant_ids"].([]any)
			if !ok || len(participantIDs) != 2 || participantIDs[0] != "u-alice" || participantIDs[1] != "u-bob" {
				t.Fatalf("payload[participant_ids] = %#v, want [u-alice u-bob]", payload["participant_ids"])
			}

			return jsonResponse(http.StatusCreated, `{"id":"room-1","title":"alpha","participants":["u-admin","u-alice","u-bob"],"messages":[{"id":"msg-1"}]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "room", "create", "--title", "alpha", "--creator-id", "u-admin", "--participant-ids", "u-alice, u-bob"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"title": "alpha"`) {
		t.Fatalf("stdout = %q, want JSON room payload", stdout.String())
	}
}

func TestExecuteRoomDeleteUsesHTTPClient(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/rooms/room-1" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/rooms/room-1")
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Status:     http.StatusText(http.StatusNoContent),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "room", "delete", "room-1"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteUserListUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/users" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/users")
			}
			return jsonResponse(http.StatusOK, `[{"id":"u-alice","name":"Alice","handle":"alice","role":"Worker","is_online":true}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "user", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "u-alice\tAlice\talice\tWorker\ttrue") {
		t.Fatalf("stdout = %q, want table row for user", stdout.String())
	}
}

func TestExecuteUserKickUsesHTTPClient(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/users/u-alice" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/users/u-alice")
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Status:     http.StatusText(http.StatusNoContent),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "user", "kick", "u-alice"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestUsageIncludesAgentRoomUserTrees(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), nil)
	if err != flag.ErrHelp {
		t.Fatalf("Execute() error = %v, want %v", err, flag.ErrHelp)
	}

	got := stderr.String()
	for _, want := range []string{
		"csgclaw agent create [flags]",
		"csgclaw room list [flags]",
		"csgclaw user kick <id> [flags]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage = %q, want substring %q", got, want)
		}
	}
}

func TestExecuteStartRemainsServeAlias(t *testing.T) {
	called := false
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
		serveFunc: func(_ context.Context, args []string, globals GlobalOptions) error {
			called = true
			if len(args) != 0 {
				t.Fatalf("args = %v, want empty", args)
			}
			if globals.Config != "/tmp/test.toml" {
				t.Fatalf("globals.Config = %q, want /tmp/test.toml", globals.Config)
			}
			return nil
		},
	}

	if err := app.Execute(context.Background(), []string{"--config", "/tmp/test.toml", "start"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("serveFunc was not called for start alias")
	}
}

func TestRunStopRejectsInvalidPIDFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{
		stdout:     &stdout,
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	pidPath := filepath.Join(t.TempDir(), "server.pid")
	if err := os.WriteFile(pidPath, []byte("abc\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	err := app.runStop([]string{"--pid", pidPath}, GlobalOptions{})
	if err == nil {
		t.Fatal("runStop() error = nil, want parse failure")
	}
	if !strings.Contains(err.Error(), "parse pid file") {
		t.Fatalf("runStop() error = %q, want parse pid file", err)
	}
}

func TestExecuteAgentStatusErrorOmitsHTTPStatusCode(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusNotFound, "agent not found\n"), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "status", "missing"})
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "agent not found" {
		t.Fatalf("Execute() error = %q, want %q", err.Error(), "agent not found")
	}
	if strings.Contains(strings.ToLower(err.Error()), "http") || strings.Contains(err.Error(), "404") {
		t.Fatalf("Execute() error = %q, should not include HTTP status details", err.Error())
	}
}

func TestExecuteAgentStatusErrorUsesJSONMessageWithoutHTTPStatusCode(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusBadRequest, `{"error":"invalid agent id"}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "status", "bad/id"})
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "invalid agent id" {
		t.Fatalf("Execute() error = %q, want %q", err.Error(), "invalid agent id")
	}
	if strings.Contains(strings.ToLower(err.Error()), "http") || strings.Contains(err.Error(), "400") {
		t.Fatalf("Execute() error = %q, should not include HTTP status details", err.Error())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
