package csgcli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"strings"
	"testing"

	appversion "csgclaw/internal/version"
)

func TestExecuteExposesOnlyLiteCommands(t *testing.T) {
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
		"bot      Manage bots",
		"room     Manage IM rooms",
		"member   Manage IM room members",
		"message  Send an IM message.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage = %q, want substring %q", got, want)
		}
	}
	for _, notWant := range []string{"  agent", "  serve", "  onboard", "  user"} {
		if strings.Contains(got, notWant) {
			t.Fatalf("usage = %q, should not include %q", got, notWant)
		}
	}
}

func TestExecuteRejectsFullCsgclawCommands(t *testing.T) {
	for _, command := range []string{"agent", "serve", "onboard", "user"} {
		t.Run(command, func(t *testing.T) {
			app := &App{
				stdout: &bytes.Buffer{},
				stderr: &bytes.Buffer{},
				httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
					return nil, nil
				}),
			}

			err := app.Execute(context.Background(), []string{command})
			if err == nil || !strings.Contains(err.Error(), `unknown command "`+command+`"`) {
				t.Fatalf("Execute(%q) error = %v, want unknown command", command, err)
			}
		})
	}
}

func TestExecuteBotListUsesAPIClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/bots?channel=feishu" {
				t.Fatalf("url = %q, want feishu bot list route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"bot-feishu","name":"feishu","role":"manager","channel":"feishu","agent_id":"u-manager","user_id":"fsu-manager","created_at":"2026-04-12T09:00:00Z"}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "bot", "list", "--channel", "feishu"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "bot-feishu"`) || !strings.Contains(stdout.String(), `"channel": "feishu"`) {
		t.Fatalf("stdout = %q, want JSON bot payload", stdout.String())
	}
}

func TestExecuteRoomCreateUsesChannelRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/rooms" {
				t.Fatalf("url = %q, want feishu room create route", req.URL.String())
			}
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["title"] != "alpha" || payload["creator_id"] != "ou_admin" {
				t.Fatalf("payload = %#v, want title and creator", payload)
			}
			return jsonResponse(http.StatusCreated, `{"id":"oc_alpha","title":"alpha","participants":["ou_admin"],"messages":[]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "room", "create", "--channel", "feishu", "--title", "alpha", "--creator-id", "ou_admin"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "oc_alpha", "alpha", "1", "0")
}

func TestExecuteMessageUsesChannelRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/messages" {
				t.Fatalf("url = %q, want feishu messages route", req.URL.String())
			}
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["room_id"] != "oc_alpha" || payload["sender_id"] != "u-manager" || payload["content"] != "hello" {
				t.Fatalf("payload = %#v, want room/sender/content", payload)
			}
			return jsonResponse(http.StatusCreated, `{"id":"om_1","sender_id":"u-manager","kind":"message","content":"hello","created_at":"2026-04-12T09:00:00Z","mentions":[]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "message", "--channel", "feishu", "--room-id", "oc_alpha", "--sender-id", "u-manager", "--content", "hello"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "om_1", "u-manager", "message", "hello")
}

func TestExecuteMemberListUsesFeishuDefault(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/rooms/oc_alpha/members" {
				t.Fatalf("url = %q, want feishu room members route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"ou_alice","name":"Alice","handle":"alice","role":"worker","is_online":true}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "member", "list", "--room-id", "oc_alpha"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "ou_alice", "Alice", "alice", "worker", "true")
}

func TestExecuteVersionFlagPrintsCsgcliVersion(t *testing.T) {
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
	if got, want := strings.TrimSpace(stdout.String()), "csgcli version 1.2.3-test"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
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
