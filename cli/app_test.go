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
	"regexp"
	"strings"
	"testing"

	"csgclaw/internal/agent"
	appversion "csgclaw/internal/version"
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

func TestExecuteAgentListUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}
			return jsonResponse(http.StatusOK, `[{"id":"u-alice","name":"alice","role":"worker","status":"running","created_at":"2026-04-01T12:00:00Z"}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "running")
}

func TestRenderAgentsTableAlignsLongColumns(t *testing.T) {
	var buf bytes.Buffer
	agents := []agent.Agent{
		{ID: "u-manager", Name: "manager", Role: "manager", Status: "running"},
		{ID: "u-dev", Name: "dev", Role: "worker", Status: "running"},
		{ID: "u-alex", Name: "alex", Role: "worker", Status: "running"},
	}

	if err := renderAgentsTable(&buf, agents); err != nil {
		t.Fatalf("renderAgentsTable() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("line count = %d, want 4; output=%q", len(lines), buf.String())
	}

	re := regexp.MustCompile(`^(\S+)(\s{2,})(\S+)(\s{2,})(\S+)(\s{2,})(\S+)$`)
	if re.FindStringSubmatchIndex(lines[0]) == nil {
		t.Fatalf("header not aligned: %q", lines[0])
	}
	wantStarts := fieldStartIndexes(lines[0])
	for i, line := range lines[1:] {
		if re.FindStringSubmatchIndex(line) == nil {
			t.Fatalf("row %d not aligned: %q", i+1, line)
		}
		if gotStarts := fieldStartIndexes(line); !slicesEqualInts(gotStarts, wantStarts) {
			t.Fatalf("row %d columns not aligned with header:\nheader=%q\nrow=%q", i+1, lines[0], line)
		}
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
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "running")
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
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "running")
}

func TestExecuteAgentLogsUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/agents/u-alice/logs?follow=true&lines=80" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents/u-alice/logs?follow=true&lines=80")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("line-1\nline-2\n")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "logs", "u-alice", "-f", "-n", "80"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.String() != "line-1\nline-2\n" {
		t.Fatalf("stdout = %q, want streamed logs", stdout.String())
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
	assertTableHasRow(t, stdout.String(), "room-1", "alpha", "2", "1")
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
	assertTableHasRow(t, stdout.String(), "u-alice", "Alice", "alice", "Worker", "true")
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

func TestUsageIncludesTopLevelCommandIndex(t *testing.T) {
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
		"Available Commands:",
		"agent    Manage agents",
		"room     Manage IM rooms",
		"user     Manage IM users",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage = %q, want substring %q", got, want)
		}
	}
}

func TestRootHelpIncludesAvailableCommands(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"-h"})
	if err != flag.ErrHelp {
		t.Fatalf("Execute() error = %v, want %v", err, flag.ErrHelp)
	}

	got := stderr.String()
	for _, want := range []string{
		"Available Commands:",
		"agent    Manage agents",
		"room     Manage IM rooms",
		"user     Manage IM users",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help = %q, want substring %q", got, want)
		}
	}
}

func TestExecuteVersionFlagPrintsVersion(t *testing.T) {
	originalVersion := appversion.Version
	appversion.Version = "1.2.3-test"
	t.Cleanup(func() { appversion.Version = originalVersion })

	var stdout bytes.Buffer
	app := &App{
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	if err := app.Execute(context.Background(), []string{"--version"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := strings.TrimSpace(stdout.String()), "csgclaw version 1.2.3-test"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestExecuteVersionFlagShortFormPrintsVersion(t *testing.T) {
	originalVersion := appversion.Version
	appversion.Version = "1.2.3-test"
	t.Cleanup(func() { appversion.Version = originalVersion })

	var stdout bytes.Buffer
	app := &App{
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	if err := app.Execute(context.Background(), []string{"-V"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := strings.TrimSpace(stdout.String()), "csgclaw version 1.2.3-test"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestAgentHelpIncludesSubcommands(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"agent", "-h"})
	if err != flag.ErrHelp {
		t.Fatalf("Execute() error = %v, want %v", err, flag.ErrHelp)
	}

	got := stderr.String()
	for _, want := range []string{
		"Manage agents.",
		"csgclaw agent <subcommand> [flags]",
		"list               List agents",
		"create             Create an agent",
		"delete <id>        Delete an agent",
		"status [id]        Show one agent or list all agents",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help = %q, want substring %q", got, want)
		}
	}
}

func TestAgentSubcommandHelpIncludesUsageAndFlags(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"agent", "list", "-h"})
	if err != flag.ErrHelp {
		t.Fatalf("Execute() error = %v, want %v", err, flag.ErrHelp)
	}

	got := stderr.String()
	for _, want := range []string{
		"List agents.",
		"csgclaw agent list [flags]",
		"Flags:",
		"-filter string",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help = %q, want substring %q", got, want)
		}
	}
}

func TestExecuteStartIsRejected(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"--config", "/tmp/test.toml", "start"})
	if err == nil {
		t.Fatal("Execute() error = nil, want unknown command")
	}
	if !strings.Contains(err.Error(), `unknown command "start"`) {
		t.Fatalf("Execute() error = %v, want unknown command start", err)
	}
	if !strings.Contains(stderr.String(), "  serve    Start the local HTTP server") {
		t.Fatalf("stderr = %q, want serve command in usage", stderr.String())
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

func assertTableHasRow(t *testing.T, table string, want ...string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(table), "\n") {
		if slicesEqual(strings.Fields(line), want) {
			return
		}
	}
	t.Fatalf("table = %q, want row %v", table, want)
}

func slicesEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func fieldStartIndexes(line string) []int {
	var starts []int
	inField := false
	for i, r := range line {
		if r != ' ' && !inField {
			starts = append(starts, i)
			inField = true
			continue
		}
		if r == ' ' {
			inField = false
		}
	}
	return starts
}

func slicesEqualInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
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
