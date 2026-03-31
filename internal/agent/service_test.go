package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/config"
)

func TestCreateWorkerRejectsReservedManagerName(t *testing.T) {
	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateRequest{
		ID:   "worker-1",
		Name: "manager",
	})
	if err == nil {
		t.Fatal("CreateWorker() error = nil, want reserved-name error")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("CreateWorker() error = %q, want reserved-name error", err)
	}
}

func TestCreateWorkerRejectsDuplicateName(t *testing.T) {
	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents["worker-1"] = Agent{
		ID:        "worker-1",
		Name:      "alice",
		Status:    "active",
		CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		Role:      RoleWorker,
	}

	_, err = svc.CreateWorker(context.Background(), CreateRequest{
		ID:   "worker-2",
		Name: "Alice",
	})
	if err == nil {
		t.Fatal("CreateWorker() duplicate error = nil, want duplicate-name error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("CreateWorker() duplicate error = %q, want duplicate-name error", err)
	}
}

func TestListWorkersFiltersUnifiedAgents(t *testing.T) {
	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents["u-manager"] = Agent{ID: "u-manager", Name: "manager", Role: RoleManager, CreatedAt: time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC)}
	svc.agents["worker-1"] = Agent{ID: "worker-1", Name: "alice", Role: RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)}
	svc.agents["agent-1"] = Agent{ID: "agent-1", Name: "observer", Role: RoleAgent, CreatedAt: time.Date(2026, 3, 28, 11, 0, 0, 0, time.UTC)}

	workers := svc.ListWorkers()
	if len(workers) != 1 || workers[0].ID != "worker-1" {
		t.Fatalf("ListWorkers() = %+v, want only worker agent", workers)
	}
}

func TestLoadMigratesLegacyWorkersIntoAgents(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Workers: []legacyWorker{
			{
				ID:        "worker-1",
				Name:      "alice",
				Status:    "running",
				CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", statePath, "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, ok := svc.Agent("worker-1")
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if got.Role != RoleWorker {
		t.Fatalf("Agent().Role = %q, want %q", got.Role, RoleWorker)
	}
}

func TestBoxRuntimeHomeUsesPerAgentDirectory(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	got, err := boxRuntimeHome("alice")
	if err != nil {
		t.Fatalf("boxRuntimeHome() error = %v", err)
	}

	want := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "alice", config.RuntimeHomeDirName)
	if got != want {
		t.Fatalf("boxRuntimeHome() = %q, want %q", got, want)
	}
}

func TestEnsureAgentProjectsRootUsesHomeProjectsDir(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	got, err := ensureAgentProjectsRoot()
	if err != nil {
		t.Fatalf("ensureAgentProjectsRoot() error = %v", err)
	}

	want := filepath.Join(homeDir, config.AppDirName, hostProjectsDir)
	if got != want {
		t.Fatalf("ensureAgentProjectsRoot() = %q, want %q", got, want)
	}

	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("ensureAgentProjectsRoot() path is not a directory: %q", got)
	}
}

func TestGatewayStartCommandUsesTiniForNormalMode(t *testing.T) {
	entrypoint, cmd := gatewayStartCommand(false)

	if strings.Join(entrypoint, " ") != "tini" {
		t.Fatalf("gatewayStartCommand(false) entrypoint = %q, want %q", entrypoint, []string{"tini"})
	}
	if strings.Join(cmd, " ") != "-- picoclaw gateway -d" {
		t.Fatalf("gatewayStartCommand(false) cmd = %q, want %q", cmd, []string{"--", "picoclaw", "gateway", "-d"})
	}
}

func TestGatewayStartCommandKeepsDebugSleepMode(t *testing.T) {
	entrypoint, cmd := gatewayStartCommand(true)

	if strings.Join(entrypoint, " ") != "sleep" {
		t.Fatalf("gatewayStartCommand(true) entrypoint = %q, want %q", entrypoint, []string{"sleep"})
	}
	if strings.Join(cmd, " ") != "infinity" {
		t.Fatalf("gatewayStartCommand(true) cmd = %q, want %q", cmd, []string{"infinity"})
	}
}

func TestPicoclawBoxEnvVars(t *testing.T) {
	got := picoclawBoxEnvVars(
		"http://10.0.0.8:18080",
		"shared-token",
		"u-worker-1",
		config.LLMConfig{
			BaseURL: "http://127.0.0.1:4000",
			APIKey:  "sk-test",
			ModelID: "minimax-m2.7",
		},
	)

	wants := map[string]string{
		"CSGCLAW_BASE_URL":                       "http://10.0.0.8:18080",
		"CSGCLAW_ACCESS_TOKEN":                   "shared-token",
		"PICOCLAW_CHANNELS_CSGCLAW_BASE_URL":     "http://10.0.0.8:18080",
		"PICOCLAW_CHANNELS_CSGCLAW_ACCESS_TOKEN": "shared-token",
		"PICOCLAW_CHANNELS_CSGCLAW_BOT_ID":       "u-worker-1",
		"PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME":    "minimax-m2.7",
		"PICOCLAW_CUSTOM_MODEL_NAME":             "minimax-m2.7",
		"PICOCLAW_CUSTOM_MODEL_ID":               "minimax-m2.7",
		"PICOCLAW_CUSTOM_MODEL_API_KEY":          "sk-test",
		"PICOCLAW_CUSTOM_MODEL_BASE_URL":         "http://127.0.0.1:4000",
	}
	for key, want := range wants {
		if got[key] != want {
			t.Fatalf("%s = %q, want %q", key, got[key], want)
		}
	}
}

func TestResolveManagerBaseURLPrefersEn0IP(t *testing.T) {
	orig := en0IPv4Resolver
	en0IPv4Resolver = func() string { return "10.0.0.8" }
	t.Cleanup(func() {
		en0IPv4Resolver = orig
	})

	got := resolveManagerBaseURL(config.ServerConfig{
		ListenAddr: "0.0.0.0:19090",
		APIBaseURL: "http://127.0.0.1:18080",
	})

	want := "http://10.0.0.8:19090"
	if got != want {
		t.Fatalf("resolveManagerBaseURL() = %q, want %q", got, want)
	}
}
