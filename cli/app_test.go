package cli

import (
	"bytes"
	"context"
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
