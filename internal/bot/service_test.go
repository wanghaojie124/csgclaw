package bot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	boxlite "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/agent"
	"csgclaw/internal/channel"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
)

func TestServiceListReturnsAllWhenChannelEmpty(t *testing.T) {
	svc := mustNewBotService(t, []Bot{
		{
			ID:        "bot-csgclaw",
			Name:      "CSGClaw Bot",
			Role:      string(RoleWorker),
			Channel:   string(ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-feishu",
			Name:      "Feishu Bot",
			Role:      string(RoleWorker),
			Channel:   string(ChannelFeishu),
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})

	got, err := svc.List("", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() len = %d, want 2", len(got))
	}
	if got[0].ID != "bot-csgclaw" || got[1].ID != "bot-feishu" {
		t.Fatalf("List() = %+v, want all bots in store order", got)
	}
}

func TestServiceListFiltersByNormalizedChannel(t *testing.T) {
	svc := mustNewBotService(t, []Bot{
		{
			ID:        "bot-csgclaw",
			Name:      "CSGClaw Bot",
			Role:      string(RoleWorker),
			Channel:   string(ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-feishu",
			Name:      "Feishu Bot",
			Role:      string(RoleWorker),
			Channel:   string(ChannelFeishu),
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})

	got, err := svc.List(" FEISHU ", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "bot-feishu" {
		t.Fatalf("List(FEISHU) = %+v, want only bot-feishu", got)
	}
}

func TestServiceListFiltersByNormalizedRole(t *testing.T) {
	svc := mustNewBotService(t, []Bot{
		{
			ID:        "bot-manager",
			Name:      "Manager Bot",
			Role:      string(RoleManager),
			Channel:   string(ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-worker",
			Name:      "Worker Bot",
			Role:      string(RoleWorker),
			Channel:   string(ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})

	got, err := svc.List("", " WORKER ")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "bot-worker" {
		t.Fatalf("List(role=WORKER) = %+v, want only bot-worker", got)
	}
}

func TestServiceListFiltersByChannelAndRole(t *testing.T) {
	svc := mustNewBotService(t, []Bot{
		{
			ID:        "bot-csgclaw-manager",
			Name:      "CSGClaw Manager",
			Role:      string(RoleManager),
			Channel:   string(ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-feishu-manager",
			Name:      "Feishu Manager",
			Role:      string(RoleManager),
			Channel:   string(ChannelFeishu),
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-feishu-worker",
			Name:      "Feishu Worker",
			Role:      string(RoleWorker),
			Channel:   string(ChannelFeishu),
			CreatedAt: time.Date(2026, 4, 12, 11, 0, 0, 0, time.UTC),
		},
	})

	got, err := svc.List("feishu", "manager")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "bot-feishu-manager" {
		t.Fatalf("List(feishu, manager) = %+v, want only bot-feishu-manager", got)
	}
}

func TestServiceListFeishuIncludesConfiguredUnavailableBots(t *testing.T) {
	store, err := NewMemoryStore([]Bot{
		{
			ID:        "u-worker",
			Name:      "Worker",
			Role:      string(RoleWorker),
			Channel:   string(ChannelFeishu),
			AgentID:   "u-worker",
			UserID:    "ou_worker",
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	feishuSvc := channel.NewFeishuServiceWithBotOpenIDResolver(
		map[string]channel.FeishuAppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
			"u-worker":  {AppID: "cli_worker", AppSecret: "worker-secret"},
		},
		func(_ context.Context, app channel.FeishuAppConfig) (channel.FeishuBotInfo, error) {
			switch app.AppID {
			case "cli_manager":
				return channel.FeishuBotInfo{OpenID: "ou_manager", AppName: "Manager Bot"}, nil
			case "cli_worker":
				return channel.FeishuBotInfo{OpenID: "ou_worker", AppName: "Worker Bot"}, nil
			default:
				t.Fatalf("unexpected app_id %q", app.AppID)
				return channel.FeishuBotInfo{}, nil
			}
		},
	)
	svc, err := NewServiceWithDependencies(store, nil, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.List(string(ChannelFeishu), "")
	if err != nil {
		t.Fatalf("List(feishu) error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List(feishu) = %+v, want stored bot plus configured manager", got)
	}
	if got[0].ID != "u-worker" || !got[0].Available {
		t.Fatalf("stored bot = %+v, want available u-worker", got[0])
	}
	if got[1].ID != "u-manager" || got[1].Name != "Manager Bot" || got[1].Role != string(RoleManager) || got[1].AgentID != "" || got[1].UserID != "ou_manager" || got[1].Available {
		t.Fatalf("configured bot = %+v, want unavailable manager with configured id and open_id", got[1])
	}
}

func TestServiceListFeishuConfiguredBotUsesMatchingAgent(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:        "u-manager",
			Name:      "manager",
			Role:      agent.RoleManager,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
			ModelID:   "default-model",
		},
	})
	feishuSvc := channel.NewFeishuServiceWithBotOpenIDResolver(
		map[string]channel.FeishuAppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
		},
		func(_ context.Context, app channel.FeishuAppConfig) (channel.FeishuBotInfo, error) {
			if app.AppID != "cli_manager" {
				t.Fatalf("unexpected app_id %q", app.AppID)
			}
			return channel.FeishuBotInfo{OpenID: "ou_manager", AppName: "Manager Bot"}, nil
		},
	)
	svc, err := NewServiceWithDependencies(store, agentSvc, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.List(string(ChannelFeishu), "")
	if err != nil {
		t.Fatalf("List(feishu) error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List(feishu) = %+v, want configured manager", got)
	}
	if got[0].ID != "u-manager" || got[0].AgentID != "u-manager" || !got[0].Available {
		t.Fatalf("configured bot = %+v, want configured id u-manager bound to available u-manager agent", got[0])
	}
}

func TestServiceListFeishuConfiguredBotsRespectRoleFilter(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	feishuSvc := channel.NewFeishuServiceWithBotOpenIDResolver(
		map[string]channel.FeishuAppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
			"u-worker":  {AppID: "cli_worker", AppSecret: "worker-secret"},
		},
		func(_ context.Context, app channel.FeishuAppConfig) (channel.FeishuBotInfo, error) {
			switch app.AppID {
			case "cli_manager":
				return channel.FeishuBotInfo{OpenID: "ou_manager", AppName: "Manager Bot"}, nil
			case "cli_worker":
				return channel.FeishuBotInfo{OpenID: "ou_worker", AppName: "Worker Bot"}, nil
			default:
				t.Fatalf("unexpected app_id %q", app.AppID)
				return channel.FeishuBotInfo{}, nil
			}
		},
	)
	svc, err := NewServiceWithDependencies(store, nil, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.List(string(ChannelFeishu), string(RoleManager))
	if err != nil {
		t.Fatalf("List(feishu, manager) error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "u-manager" || got[0].Name != "Manager Bot" || got[0].UserID != "ou_manager" || got[0].AgentID != "" || got[0].Available {
		t.Fatalf("List(feishu, manager) = %+v, want unavailable configured manager only", got)
	}
}

func TestServiceListRejectsInvalidChannel(t *testing.T) {
	svc := mustNewBotService(t, nil)

	_, err := svc.List("slack", "")
	if err == nil || !strings.Contains(err.Error(), "channel must be one of") {
		t.Fatalf("List(slack) error = %v, want channel validation error", err)
	}
}

func TestServiceListRejectsInvalidRole(t *testing.T) {
	svc := mustNewBotService(t, nil)

	_, err := svc.List("", "agent")
	if err == nil || !strings.Contains(err.Error(), "role must be one of") {
		t.Fatalf("List(role=agent) error = %v, want role validation error", err)
	}
}

func TestServiceDeleteRemovesBotForChannel(t *testing.T) {
	svc := mustNewBotService(t, []Bot{
		{
			ID:        "u-alice",
			Name:      "Alice",
			Role:      string(RoleWorker),
			Channel:   string(ChannelCSGClaw),
			AgentID:   "u-alice",
			UserID:    "u-alice",
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "u-alice",
			Name:      "Alice",
			Role:      string(RoleWorker),
			Channel:   string(ChannelFeishu),
			AgentID:   "u-alice",
			UserID:    "u-alice",
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})

	if err := svc.Delete(context.Background(), "feishu", "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	feishuBots, err := svc.List("feishu", "")
	if err != nil {
		t.Fatalf("List(feishu) error = %v", err)
	}
	if len(feishuBots) != 0 {
		t.Fatalf("List(feishu) = %+v, want deleted", feishuBots)
	}
	csgclawBots, err := svc.List("csgclaw", "")
	if err != nil {
		t.Fatalf("List(csgclaw) error = %v", err)
	}
	if len(csgclawBots) != 1 || csgclawBots[0].ID != "u-alice" {
		t.Fatalf("List(csgclaw) = %+v, want retained u-alice", csgclawBots)
	}
}

func TestServiceDeleteRemovesBackingAgentWhenLastReference(t *testing.T) {
	agent.SetTestHooks(
		func(_ *agent.Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		nil,
	)
	defer agent.ResetTestHooks()

	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:        "u-alice",
			Name:      "alice",
			Role:      agent.RoleWorker,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
	})
	store, err := NewMemoryStore([]Bot{
		{
			ID:        "u-alice",
			Name:      "Alice",
			Role:      string(RoleWorker),
			Channel:   string(ChannelFeishu),
			AgentID:   "u-alice",
			UserID:    "u-alice",
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, nil)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	if err := svc.Delete(context.Background(), "feishu", "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := agentSvc.Agent("u-alice"); ok {
		t.Fatal("Agent() ok = true, want false after deleting last bot reference")
	}
}

func TestNewServiceRequiresStore(t *testing.T) {
	_, err := NewService(nil)
	if err == nil || !strings.Contains(err.Error(), "bot store is required") {
		t.Fatalf("NewService(nil) error = %v, want store required", err)
	}
}

func TestServiceCreateCSGClawWorkerCreatesAgentUserAndBot(t *testing.T) {
	agent.SetTestHooks(
		func(_ *agent.Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		func(_ *agent.Service, _ context.Context, _ *boxlite.Runtime, _ string, name, botID string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
			if name != "alice" {
				t.Fatalf("create gateway name = %q, want alice", name)
			}
			if botID != "u-alice" {
				t.Fatalf("create gateway botID = %q, want u-alice", botID)
			}
			return nil, &boxlite.BoxInfo{
				ID:        "box-alice",
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
				Name:      name,
				Image:     "test-image",
			}, nil
		},
	)
	defer agent.ResetTestHooks()

	agentSvc, err := agent.NewService(config.ModelConfig{ModelID: "default-model"}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	imSvc := im.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:        "alice",
		Description: "test lead",
		Role:        string(RoleWorker),
		Channel:     string(ChannelCSGClaw),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != "u-alice" || got.AgentID != "u-alice" || got.UserID != "u-alice" {
		t.Fatalf("Create() = %+v, want u-alice bot/agent/user IDs", got)
	}
	if got.Role != string(RoleWorker) || got.Channel != string(ChannelCSGClaw) {
		t.Fatalf("Create() = %+v, want worker csgclaw", got)
	}
	if got.Description != "test lead" {
		t.Fatalf("Create().Description = %q, want test lead", got.Description)
	}
	if _, ok := agentSvc.Agent("u-alice"); !ok {
		t.Fatal("agent u-alice not created")
	}
	users := imSvc.ListUsers()
	if !containsUser(users, "u-alice") {
		t.Fatalf("users = %+v, want u-alice", users)
	}
	listed, err := svc.List(string(ChannelCSGClaw), "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "u-alice" {
		t.Fatalf("List(csgclaw) = %+v, want u-alice", listed)
	}
	if listed[0].Description != "test lead" {
		t.Fatalf("List(csgclaw)[0].Description = %q, want test lead", listed[0].Description)
	}
}

func TestServiceCreateFeishuWorkerCreatesAgentUserAndBot(t *testing.T) {
	agent.SetTestHooks(
		func(_ *agent.Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		func(_ *agent.Service, _ context.Context, _ *boxlite.Runtime, _ string, name, botID string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
			if name != "alice" {
				t.Fatalf("create gateway name = %q, want alice", name)
			}
			if botID != "u-alice" {
				t.Fatalf("create gateway botID = %q, want u-alice", botID)
			}
			return nil, &boxlite.BoxInfo{
				ID:        "box-alice",
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
				Name:      name,
				Image:     "test-image",
			}, nil
		},
	)
	defer agent.ResetTestHooks()

	agentSvc, err := agent.NewService(config.ModelConfig{ModelID: "default-model"}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	feishuSvc := channel.NewFeishuService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:        "alice",
		Description: "test lead",
		Role:        string(RoleWorker),
		Channel:     string(ChannelFeishu),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != "u-alice" || got.AgentID != "u-alice" || got.UserID != "u-alice" {
		t.Fatalf("Create() = %+v, want u-alice bot/agent/user IDs", got)
	}
	if got.Role != string(RoleWorker) || got.Channel != string(ChannelFeishu) {
		t.Fatalf("Create() = %+v, want worker feishu", got)
	}
	if _, ok := agentSvc.Agent("u-alice"); !ok {
		t.Fatal("agent u-alice not created")
	}
	if !containsUser(feishuSvc.ListUsers(), "u-alice") {
		t.Fatalf("feishu users = %+v, want u-alice", feishuSvc.ListUsers())
	}
	listed, err := svc.List(string(ChannelFeishu), "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "u-alice" {
		t.Fatalf("List(feishu) = %+v, want u-alice", listed)
	}
}

func TestServiceCreateWorkerReusesAgentAcrossChannels(t *testing.T) {
	createCalls := 0
	agent.SetTestHooks(
		func(_ *agent.Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		func(_ *agent.Service, _ context.Context, _ *boxlite.Runtime, _ string, name, botID string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
			createCalls++
			if name != "alice" {
				t.Fatalf("create gateway name = %q, want alice", name)
			}
			if botID != "u-alice" {
				t.Fatalf("create gateway botID = %q, want u-alice", botID)
			}
			return nil, &boxlite.BoxInfo{
				ID:        "box-alice",
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
				Name:      name,
				Image:     "test-image",
			}, nil
		},
	)
	defer agent.ResetTestHooks()

	agentSvc, err := agent.NewService(config.ModelConfig{ModelID: "default-model"}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	imSvc := im.NewService()
	feishuSvc := channel.NewFeishuService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	csgclawBot, err := svc.Create(context.Background(), CreateRequest{
		Name:    "alice",
		Role:    string(RoleWorker),
		Channel: string(ChannelCSGClaw),
	})
	if err != nil {
		t.Fatalf("Create(csgclaw worker) error = %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	feishuBot, err := svc.Create(context.Background(), CreateRequest{
		Name:    "alice",
		Role:    string(RoleWorker),
		Channel: string(ChannelFeishu),
	})
	if err != nil {
		t.Fatalf("Create(feishu worker) error = %v", err)
	}
	if createCalls != 1 {
		t.Fatalf("create gateway calls = %d, want 1", createCalls)
	}
	if csgclawBot.ID != "u-alice" || feishuBot.ID != "u-alice" || csgclawBot.AgentID != feishuBot.AgentID {
		t.Fatalf("created bots = %+v / %+v, want shared u-alice agent", csgclawBot, feishuBot)
	}
	if !feishuBot.CreatedAt.After(csgclawBot.CreatedAt) {
		t.Fatalf("created_at = %v / %v, want feishu bot time after csgclaw channel user time", csgclawBot.CreatedAt, feishuBot.CreatedAt)
	}
	if !containsUser(imSvc.ListUsers(), "u-alice") {
		t.Fatalf("im users = %+v, want u-alice", imSvc.ListUsers())
	}
	if !containsUser(feishuSvc.ListUsers(), "u-alice") {
		t.Fatalf("feishu users = %+v, want u-alice", feishuSvc.ListUsers())
	}
	all, err := svc.List("", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List() = %+v, want two channel bindings", all)
	}
}

func TestServiceCreateCSGClawManagerBindsBootstrappedAgent(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			Role:      agent.RoleManager,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
			ModelID:   "default-model",
		},
	})
	imSvc := im.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	})
	if err != nil {
		t.Fatalf("Create(manager) error = %v", err)
	}
	if got.ID != agent.ManagerUserID || got.AgentID != agent.ManagerUserID || got.UserID != agent.ManagerUserID {
		t.Fatalf("Create(manager) = %+v, want u-manager IDs", got)
	}
	if got.Role != string(RoleManager) || got.Channel != string(ChannelCSGClaw) {
		t.Fatalf("Create(manager) = %+v, want manager csgclaw", got)
	}
	if !containsUser(imSvc.ListUsers(), agent.ManagerUserID) {
		t.Fatalf("users = %+v, want u-manager", imSvc.ListUsers())
	}
}

func TestServiceCreateManagerBindsSameAgentAcrossChannels(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			Role:      agent.RoleManager,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
			ModelID:   "default-model",
		},
	})
	imSvc := im.NewService()
	feishuSvc := channel.NewFeishuService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	csgclawBot, err := svc.Create(context.Background(), CreateRequest{
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	})
	if err != nil {
		t.Fatalf("Create(csgclaw manager) error = %v", err)
	}
	feishuBot, err := svc.Create(context.Background(), CreateRequest{
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelFeishu),
	})
	if err != nil {
		t.Fatalf("Create(feishu manager) error = %v", err)
	}
	if csgclawBot.ID != agent.ManagerUserID || feishuBot.ID != agent.ManagerUserID || csgclawBot.AgentID != feishuBot.AgentID {
		t.Fatalf("created managers = %+v / %+v, want shared manager agent", csgclawBot, feishuBot)
	}
	all, err := svc.List("", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List() = %+v, want two channel bindings", all)
	}
}

func TestServiceCreateCSGClawManagerReusesExistingBotAndRestoresMissingUser(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			Role:      agent.RoleManager,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
	})
	imSvc := im.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	if _, err := svc.Create(context.Background(), CreateRequest{
		ID:      agent.ManagerUserID,
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	}); err != nil {
		t.Fatalf("first Create(manager) error = %v", err)
	}
	if err := imSvc.KickUser(agent.ManagerUserID); err != nil {
		t.Fatalf("KickUser(manager) error = %v", err)
	}
	if containsUser(imSvc.ListUsers(), agent.ManagerUserID) {
		t.Fatalf("users = %+v, want u-manager removed before second create", imSvc.ListUsers())
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		ID:      agent.ManagerUserID,
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	})
	if err != nil {
		t.Fatalf("second Create(manager) error = %v", err)
	}
	if got.ID != agent.ManagerUserID || got.UserID != agent.ManagerUserID {
		t.Fatalf("second Create(manager) = %+v, want u-manager", got)
	}
	if !containsUser(imSvc.ListUsers(), agent.ManagerUserID) {
		t.Fatalf("users = %+v, want u-manager restored", imSvc.ListUsers())
	}
}

func TestServiceCreateFeishuManagerEnsuresExistingUser(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			Role:      agent.RoleManager,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
	})
	feishuSvc := channel.NewFeishuService()
	if _, err := feishuSvc.CreateUser(channel.FeishuCreateUserRequest{ID: agent.ManagerUserID, Name: "manager"}); err != nil {
		t.Fatalf("CreateUser(manager) error = %v", err)
	}
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelFeishu),
	})
	if err != nil {
		t.Fatalf("Create(feishu manager) error = %v", err)
	}
	if got.ID != agent.ManagerUserID || got.UserID != agent.ManagerUserID || got.Channel != string(ChannelFeishu) {
		t.Fatalf("Create(feishu manager) = %+v, want u-manager feishu", got)
	}
}

func TestServiceCreateFeishuManagerUsesConfiguredOpenID(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			Role:      agent.RoleManager,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
	})
	feishuSvc := channel.NewFeishuServiceWithBotOpenIDResolver(
		map[string]channel.FeishuAppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
		},
		func(_ context.Context, app channel.FeishuAppConfig) (channel.FeishuBotInfo, error) {
			if got, want := app.AppID, "cli_manager"; got != want {
				t.Fatalf("resolve app_id = %q, want %q", got, want)
			}
			return channel.FeishuBotInfo{OpenID: "ou_manager", AppName: "Manager Bot"}, nil
		},
	)
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelFeishu),
	})
	if err != nil {
		t.Fatalf("Create(feishu manager) error = %v", err)
	}
	if got.ID != agent.ManagerUserID || got.AgentID != agent.ManagerUserID || got.UserID != "ou_manager" || got.Channel != string(ChannelFeishu) {
		t.Fatalf("Create(feishu manager) = %+v, want u-manager agent with ou_manager user", got)
	}

	bots, err := svc.List(string(ChannelFeishu), "")
	if err != nil {
		t.Fatalf("List(feishu) error = %v", err)
	}
	if len(bots) != 1 || bots[0].UserID != "ou_manager" {
		t.Fatalf("List(feishu) = %+v, want manager user_id ou_manager", bots)
	}
}

func TestServiceCreateManagerBootstrapsMissingAgent(t *testing.T) {
	agent.SetTestHooks(
		func(_ *agent.Service, _ string) (*boxlite.Runtime, error) { return &boxlite.Runtime{}, nil },
		func(_ *agent.Service, _ context.Context, _ *boxlite.Runtime, _ string, name, botID string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
			if name != agent.ManagerName {
				t.Fatalf("create gateway name = %q, want manager", name)
			}
			if botID != agent.ManagerUserID {
				t.Fatalf("create gateway botID = %q, want u-manager", botID)
			}
			return &boxlite.Box{}, &boxlite.BoxInfo{
				ID:        "box-manager",
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
				Name:      name,
				Image:     "test-image",
			}, nil
		},
	)
	defer agent.ResetTestHooks()
	agent.TestOnlySetGetBoxHook(func(_ *agent.Service, _ context.Context, _ *boxlite.Runtime, _ string) (*boxlite.Box, error) {
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	})

	agentSvc := mustNewSeededAgentService(t, nil)
	imSvc := im.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	})
	if err != nil {
		t.Fatalf("Create(manager) error = %v", err)
	}
	if got.ID != agent.ManagerUserID || got.AgentID != agent.ManagerUserID || got.UserID != agent.ManagerUserID {
		t.Fatalf("Create(manager) = %+v, want u-manager IDs", got)
	}
	if _, ok := agentSvc.Agent(agent.ManagerUserID); !ok {
		t.Fatal("manager agent was not bootstrapped")
	}
}

func TestServiceCreateRejectsUnsupportedCombination(t *testing.T) {
	svc := mustNewBotService(t, nil)

	_, err := svc.Create(context.Background(), CreateRequest{
		ID:      "custom-manager",
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	})
	if err == nil || !strings.Contains(err.Error(), "agent service is required") {
		t.Fatalf("Create(manager without agent service) error = %v, want agent service error", err)
	}

	_, err = svc.Create(context.Background(), CreateRequest{
		Name:    "alice",
		Role:    string(RoleWorker),
		Channel: "slack",
	})
	if err == nil || !strings.Contains(err.Error(), "channel must be one of") {
		t.Fatalf("Create(feishu) error = %v, want unsupported channel", err)
	}
}

func TestServiceCreateManagerRejectsCustomID(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{ID: agent.ManagerUserID, Name: agent.ManagerName, Role: agent.RoleManager},
	})
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, im.NewService())
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	_, err = svc.Create(context.Background(), CreateRequest{
		ID:      "bot-manager",
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	})
	if err == nil || !strings.Contains(err.Error(), `manager bot id must be "u-manager"`) {
		t.Fatalf("Create(manager custom ID) error = %v, want manager ID error", err)
	}
}

func containsUser(users []im.User, id string) bool {
	for _, user := range users {
		if user.ID == id {
			return true
		}
	}
	return false
}

func mustNewBotService(t *testing.T, bots []Bot) *Service {
	t.Helper()
	store, err := NewMemoryStore(bots)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewService(store)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

func mustNewSeededAgentService(t *testing.T, agents []agent.Agent) *agent.Service {
	t.Helper()

	if agents == nil {
		agents = []agent.Agent{}
	}
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(map[string]any{"agents": agents})
	if err != nil {
		t.Fatalf("marshal agents: %v", err)
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write agents: %v", err)
	}
	svc, err := agent.NewService(config.ModelConfig{ModelID: "default-model"}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	return svc
}
