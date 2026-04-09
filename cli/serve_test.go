package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/server"
)

func TestServeForegroundPassesContextToServer(t *testing.T) {
	origRunServer := runServer
	origNewAgentService := newAgentServiceFn
	origNewIMService := newIMServiceFn
	t.Cleanup(func() {
		runServer = origRunServer
		newAgentServiceFn = origNewAgentService
		newIMServiceFn = origNewIMService
	})

	ctx := context.WithValue(context.Background(), struct{}{}, "serve-context")

	newAgentServiceFn = func(config.Config) (*agent.Service, error) {
		return nil, nil
	}
	newIMServiceFn = func() (*im.Service, error) {
		return nil, nil
	}

	called := false
	runServer = func(opts server.Options) error {
		called = true
		if opts.Context != ctx {
			t.Fatalf("Context = %v, want %v", opts.Context, ctx)
		}
		return nil
	}

	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
	}
	cfg := config.Config{
		Server: config.ServerConfig{
			AdvertiseBaseURL: "http://example.test",
		},
	}

	if err := app.serveForeground(ctx, cfg); err != nil {
		t.Fatalf("serveForeground() error = %v", err)
	}
	if !called {
		t.Fatal("runServer was not called")
	}
}

func TestAPIBaseURLDefaultsToLocalhost(t *testing.T) {
	got := apiBaseURL(config.ServerConfig{ListenAddr: "0.0.0.0:19090"})
	want := "http://127.0.0.1:19090"
	if got != want {
		t.Fatalf("apiBaseURL() = %q, want %q", got, want)
	}
}

func TestAPIBaseURLPrefersAdvertiseBaseURL(t *testing.T) {
	got := apiBaseURL(config.ServerConfig{
		ListenAddr:       "0.0.0.0:19090",
		AdvertiseBaseURL: "http://example.test/base/",
	})
	want := "http://example.test/base"
	if got != want {
		t.Fatalf("apiBaseURL() = %q, want %q", got, want)
	}
}

func TestAPIBaseURLFallsBackToSharedDefault(t *testing.T) {
	got := apiBaseURL(config.ServerConfig{})
	if got != config.DefaultAPIBaseURL() {
		t.Fatalf("apiBaseURL() = %q, want %q", got, config.DefaultAPIBaseURL())
	}
}

func TestValidateModelConfigRequiresOnboardWhenIncomplete(t *testing.T) {
	err := validateModelConfig(config.Config{})
	if err == nil {
		t.Fatal("validateModelConfig() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "csgclaw onboard") {
		t.Fatalf("validateModelConfig() error = %q, want onboard guidance", err)
	}
	if !strings.Contains(err.Error(), "--base-url") || !strings.Contains(err.Error(), "--api-key") || !strings.Contains(err.Error(), "--model-id") {
		t.Fatalf("validateModelConfig() error = %q, want missing model flags", err)
	}
}
