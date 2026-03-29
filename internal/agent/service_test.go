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

func TestCSGClawBoxEnvVars(t *testing.T) {
	got := csgclawBoxEnvVars("http://127.0.0.1:18080", "shared-token")

	if got["CSGCLAW_BASE_URL"] != "http://127.0.0.1:18080" {
		t.Fatalf("CSGCLAW_BASE_URL = %q, want %q", got["CSGCLAW_BASE_URL"], "http://127.0.0.1:18080")
	}
	if got["CSGCLAW_ACCESS_TOKEN"] != "shared-token" {
		t.Fatalf("CSGCLAW_ACCESS_TOKEN = %q, want %q", got["CSGCLAW_ACCESS_TOKEN"], "shared-token")
	}
}
