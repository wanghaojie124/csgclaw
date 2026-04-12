package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/config"
)

func TestRunOnboardRequiresLLMFlagsForFirstTimeSetup(t *testing.T) {
	origEnsureBootstrapState := agentEnsureBootstrapState
	origEnsureIMBootstrapState := imEnsureBootstrapState
	t.Cleanup(func() {
		agentEnsureBootstrapState = origEnsureBootstrapState
		imEnsureBootstrapState = origEnsureIMBootstrapState
	})

	agentEnsureBootstrapState = func(context.Context, string, config.ServerConfig, config.ModelConfig, string, bool) error {
		t.Fatal("agent bootstrap should not run when config is incomplete")
		return nil
	}
	imEnsureBootstrapState = func(string) error {
		t.Fatal("IM bootstrap should not run when config is incomplete")
		return nil
	}

	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
	}

	err := app.runOnboard(nil, GlobalOptions{Config: filepath.Join(t.TempDir(), "config.toml")})
	if err == nil {
		t.Fatal("runOnboard() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "--base-url") || !strings.Contains(err.Error(), "--api-key") || !strings.Contains(err.Error(), "--model-id") {
		t.Fatalf("runOnboard() error = %q, want all required LLM flags", err)
	}
}

func TestRunOnboardReusesExistingLLMConfig(t *testing.T) {
	origEnsureBootstrapState := agentEnsureBootstrapState
	origEnsureIMBootstrapState := imEnsureBootstrapState
	t.Cleanup(func() {
		agentEnsureBootstrapState = origEnsureBootstrapState
		imEnsureBootstrapState = origEnsureIMBootstrapState
	})

	callCount := 0
	agentEnsureBootstrapState = func(_ context.Context, _ string, _ config.ServerConfig, model config.ModelConfig, _ string, _ bool) error {
		callCount++
		if model.BaseURL != "http://llm.test" || model.APIKey != "secret" || model.ModelID != "gpt-test" {
			t.Fatalf("model config = %#v, want preserved values", model)
		}
		return nil
	}
	imEnsureBootstrapState = func(string) error { return nil }

	configPath := filepath.Join(t.TempDir(), "config.toml")
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
	}

	if err := app.runOnboard([]string{"--base-url", "http://llm.test", "--api-key", "secret", "--model-id", "gpt-test"}, GlobalOptions{Config: configPath}); err != nil {
		t.Fatalf("initial runOnboard() error = %v", err)
	}

	if err := app.runOnboard(nil, GlobalOptions{Config: configPath}); err != nil {
		t.Fatalf("second runOnboard() error = %v", err)
	}

	if callCount != 2 {
		t.Fatalf("agent bootstrap call count = %d, want 2", callCount)
	}
}
