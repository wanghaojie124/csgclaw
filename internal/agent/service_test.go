package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	boxlite "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/config"
)

func TestCreateWorkerRejectsReservedManagerName(t *testing.T) {
	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "")
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
	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "")
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

func TestCreateWorkerRejectsInvalidRuntime(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return &boxlite.Runtime{}, nil },
		nil,
	)
	defer ResetTestHooks()

	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateRequest{Name: "alice"})
	if err == nil {
		t.Fatal("CreateWorker() error = nil, want invalid runtime error")
	}
	if !strings.Contains(err.Error(), "invalid boxlite runtime") {
		t.Fatalf("CreateWorker() error = %q, want invalid runtime error", err)
	}
}

func TestRuntimeValidRejectsNilAndZeroValue(t *testing.T) {
	var nilRT *boxlite.Runtime
	if runtimeValid(nilRT) {
		t.Fatal("runtimeValid(nil) = true, want false")
	}
	if runtimeValid(&boxlite.Runtime{}) {
		t.Fatal("runtimeValid(zero runtime) = true, want false")
	}
}

func TestListWorkersFiltersUnifiedAgents(t *testing.T) {
	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "")
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

	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", statePath)
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

func TestDeleteRejectsManagerAgent(t *testing.T) {
	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents[ManagerUserID] = Agent{
		ID:        ManagerUserID,
		Name:      ManagerName,
		Role:      RoleManager,
		CreatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	}

	err = svc.Delete(context.Background(), ManagerUserID)
	if err == nil {
		t.Fatal("Delete() error = nil, want reserved error")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("Delete() error = %q, want reserved error", err)
	}
}

func TestDeleteRemovesAgentFromState(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		nil,
	)
	defer ResetTestHooks()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		Role:      RoleWorker,
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}
	if err := svc.saveLocked(); err != nil {
		t.Fatalf("saveLocked() error = %v", err)
	}

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := svc.Agent("u-alice"); ok {
		t.Fatal("Agent() ok = true, want false after delete")
	}

	reloaded, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() reload error = %v", err)
	}
	if _, ok := reloaded.Agent("u-alice"); ok {
		t.Fatal("reloaded Agent() ok = true, want false after delete")
	}
}

func TestDeleteRemovesAgentHomeDirectory(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		nil,
	)
	defer ResetTestHooks()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		Role:      RoleWorker,
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	agentHome, err := agentHomeDir("alice")
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(agentHome, config.RuntimeHomeDirName), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(agent runtime) error = %v", err)
	}
	sharedProjects, err := ensureAgentProjectsRoot()
	if err != nil {
		t.Fatalf("ensureAgentProjectsRoot() error = %v", err)
	}

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := os.Stat(agentHome); !os.IsNotExist(err) {
		t.Fatalf("os.Stat(agentHome) error = %v, want not exist", err)
	}
	if info, err := os.Stat(sharedProjects); err != nil {
		t.Fatalf("os.Stat(sharedProjects) error = %v", err)
	} else if !info.IsDir() {
		t.Fatalf("shared projects path is not a directory: %q", sharedProjects)
	}
}

func TestDeletePrefersBoxIDOverName(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return &boxlite.Runtime{}, nil },
		nil,
	)
	defer ResetTestHooks()

	var removed string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, idOrName string) error {
		removed = idOrName
		return nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string) (*boxlite.Box, error) {
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	}
	defer func() {
		testForceRemoveBoxHook = nil
	}()

	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		BoxID:     "box-123",
		Role:      RoleWorker,
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if removed != "box-123" {
		t.Fatalf("ForceRemove() target = %q, want %q", removed, "box-123")
	}
}

func TestDeleteRemovesRuntimeCacheByHomeDir(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return &boxlite.Runtime{}, nil },
		nil,
	)
	defer ResetTestHooks()
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string) error {
		return nil
	}
	defer func() {
		testForceRemoveBoxHook = nil
	}()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		Role:      RoleWorker,
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	runtimeHome, err := boxRuntimeHome("alice")
	if err != nil {
		t.Fatalf("boxRuntimeHome() error = %v", err)
	}
	svc.runtimes[runtimeHome] = &boxlite.Runtime{}

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := svc.runtimes[runtimeHome]; ok {
		t.Fatalf("Delete() kept runtime cache for %q", runtimeHome)
	}
}

func TestCreateWorkerStoresBoxID(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string, name, _, _ string) (*boxlite.Box, *boxlite.BoxInfo, error) {
			return nil, &boxlite.BoxInfo{
				ID:        "box-" + name,
				Name:      name,
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				Image:     "test-image",
			}, nil
		},
	)
	defer ResetTestHooks()

	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateRequest{Name: "alice"})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.BoxID != "box-alice" {
		t.Fatalf("CreateWorker().BoxID = %q, want %q", got.BoxID, "box-alice")
	}
}

func TestEnsureBootstrapStateForceRecreatePrefersStoredManagerBoxID(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return &boxlite.Runtime{}, nil },
		func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string, name, _, _ string) (*boxlite.Box, *boxlite.BoxInfo, error) {
			return &boxlite.Box{}, &boxlite.BoxInfo{
				ID:        "box-new",
				Name:      name,
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
				Image:     "test-image",
			}, nil
		},
	)
	defer ResetTestHooks()

	var removed string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, idOrName string) error {
		removed = idOrName
		return nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string) (*boxlite.Box, error) {
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	}
	defer func() {
		testForceRemoveBoxHook = nil
	}()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []Agent{
			{
				ID:        ManagerUserID,
				Name:      ManagerName,
				Role:      RoleManager,
				BoxID:     "box-old",
				Status:    "running",
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, config.LLMConfig{}, config.PicoClawConfig{}, "", true); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if removed != "box-old" {
		t.Fatalf("ForceRemove() target = %q, want %q", removed, "box-old")
	}

	reloaded, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() reload error = %v", err)
	}
	got, ok := reloaded.Agent(ManagerUserID)
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if got.BoxID != "box-new" {
		t.Fatalf("Agent().BoxID = %q, want %q", got.BoxID, "box-new")
	}
}

func TestEnsureBootstrapStateReusesStoredManagerBoxIDWithoutForce(t *testing.T) {
	SetTestHooks(nil, nil)
	defer ResetTestHooks()

	primaryRT := &boxlite.Runtime{}
	legacyRT := &boxlite.Runtime{}
	testEnsureRuntimeAtHomeHook = func(_ *Service, home string) (*boxlite.Runtime, error) {
		if strings.HasSuffix(home, filepath.Join(config.AppDirName, config.RuntimeHomeDirName)) {
			return primaryRT, nil
		}
		return legacyRT, nil
	}

	var created bool
	testCreateGatewayBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string, _ string, _, _ string) (*boxlite.Box, *boxlite.BoxInfo, error) {
		created = true
		return nil, nil, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, rt *boxlite.Runtime, idOrName string) (*boxlite.Box, error) {
		if rt == primaryRT {
			return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing in primary"}
		}
		if rt == legacyRT && idOrName == "box-old" {
			return &boxlite.Box{}, nil
		}
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	}
	testStartBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Box) error { return nil }
	testBoxInfoHook = func(_ *Service, _ context.Context, _ *boxlite.Box) (*boxlite.BoxInfo, error) {
		return &boxlite.BoxInfo{
			ID:        "box-old",
			Name:      ManagerName,
			State:     boxlite.StateRunning,
			CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
			Image:     "test-image",
		}, nil
	}

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []Agent{
			{
				ID:        ManagerUserID,
				Name:      ManagerName,
				Role:      RoleManager,
				BoxID:     "box-old",
				Status:    "running",
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, config.LLMConfig{}, config.PicoClawConfig{}, "", false); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if created {
		t.Fatal("createGatewayBox() called, want existing manager box to be reused")
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

func TestLookupBootstrapManagerUsesPerAgentHome(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	var gotHome string
	testEnsureRuntimeAtHomeHook = func(_ *Service, homeDir string) (*boxlite.Runtime, error) {
		gotHome = homeDir
		return &boxlite.Runtime{}, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string) (*boxlite.Box, error) {
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	}
	defer func() {
		testEnsureRuntimeAtHomeHook = nil
		testGetBoxHook = nil
	}()

	svc, err := NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	rt, box, err := svc.lookupBootstrapManager(context.Background())
	if err != nil {
		t.Fatalf("lookupBootstrapManager() error = %v", err)
	}
	if box != nil {
		t.Fatalf("lookupBootstrapManager() box = %#v, want nil", box)
	}
	wantHome := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, ManagerName, config.RuntimeHomeDirName)
	if rt == nil {
		t.Fatal("lookupBootstrapManager() runtime = nil, want non-nil")
	}
	if info, err := os.Stat(wantHome); err != nil {
		t.Fatalf("os.Stat(runtime home) error = %v", err)
	} else if !info.IsDir() {
		t.Fatalf("runtime home is not a directory: %q", wantHome)
	}
	if got, want := len(svc.runtimes), 0; got != want {
		t.Fatalf("len(svc.runtimes) = %d, want %d when runtime creation is hooked", got, want)
	}
	if got, want := gotHome, wantHome; got != want {
		t.Fatalf("resolved manager runtime home = %q, want %q", got, want)
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

func TestResolveManagerBaseURLPrefersLocalIP(t *testing.T) {
	orig := localIPv4Resolver
	localIPv4Resolver = func() string { return "10.0.0.8" }
	t.Cleanup(func() {
		localIPv4Resolver = orig
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
