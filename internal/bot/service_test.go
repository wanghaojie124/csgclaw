package bot

import (
	"strings"
	"testing"
	"time"
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

	got, err := svc.List("")
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

	got, err := svc.List(" FEISHU ")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "bot-feishu" {
		t.Fatalf("List(FEISHU) = %+v, want only bot-feishu", got)
	}
}

func TestServiceListRejectsInvalidChannel(t *testing.T) {
	svc := mustNewBotService(t, nil)

	_, err := svc.List("slack")
	if err == nil || !strings.Contains(err.Error(), "channel must be one of") {
		t.Fatalf("List(slack) error = %v, want channel validation error", err)
	}
}

func TestNewServiceRequiresStore(t *testing.T) {
	_, err := NewService(nil)
	if err == nil || !strings.Contains(err.Error(), "bot store is required") {
		t.Fatalf("NewService(nil) error = %v, want store required", err)
	}
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
