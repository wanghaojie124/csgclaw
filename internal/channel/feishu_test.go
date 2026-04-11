package channel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/im"
)

func TestFeishuServiceDoesNotPersistState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "channels", "feishu", "state.json")
	svc := NewFeishuService()

	if _, err := svc.CreateUser(FeishuCreateUserRequest{ID: "fsu-alice", Name: "Alice"}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state.json exists after Feishu operation; stat error = %v", err)
	}
}

func TestFeishuServiceKeepsNamedAppConfigs(t *testing.T) {
	svc := NewFeishuService(map[string]FeishuAppConfig{
		"manager": {
			AppID:       "cli_manager",
			AppSecret:   "manager-secret",
			AdminOpenID: "ou_admin",
		},
		"dev": {
			AppID:     "cli_dev",
			AppSecret: "dev-secret",
		},
	})

	apps := svc.AppConfigs()
	if got, want := apps["manager"].AppID, "cli_manager"; got != want {
		t.Fatalf("manager app_id = %q, want %q", got, want)
	}
	if got, want := apps["manager"].AdminOpenID, "ou_admin"; got != want {
		t.Fatalf("manager admin_open_id = %q, want %q", got, want)
	}
	if got, want := apps["dev"].AppSecret, "dev-secret"; got != want {
		t.Fatalf("dev app_secret = %q, want %q", got, want)
	}

	apps["manager"] = FeishuAppConfig{AppID: "mutated"}
	if got, want := svc.AppConfigs()["manager"].AppID, "cli_manager"; got != want {
		t.Fatalf("manager app_id after caller mutation = %q, want %q", got, want)
	}
}

func TestFeishuCreateRoomUsesConfiguredAdminOpenID(t *testing.T) {
	var gotCreatorID string
	svc := NewFeishuServiceWithCreateChat(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(_ context.Context, _ FeishuAppConfig, req FeishuCreateChatRequest) (FeishuCreateChatResponse, error) {
			gotCreatorID = req.CreatorID
			return FeishuCreateChatResponse{ChatID: "oc_alpha", Name: req.Title, Description: req.Description}, nil
		},
	)

	if _, err := svc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-manager"}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	if got, want := gotCreatorID, "ou_admin"; got != want {
		t.Fatalf("create chat creator_id = %q, want %q", got, want)
	}
}

func TestFeishuCreateRoomRequiresAppForCreatorID(t *testing.T) {
	svc := NewFeishuServiceWithCreateChat(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(_ context.Context, _ FeishuAppConfig, _ FeishuCreateChatRequest) (FeishuCreateChatResponse, error) {
			t.Fatal("createChat should not be called when creator app is missing")
			return FeishuCreateChatResponse{}, nil
		},
	)

	_, err := svc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-missing"})
	if err == nil {
		t.Fatal("CreateRoom() error = nil, want missing creator app error")
	}
	if !strings.Contains(err.Error(), `creator_id "u-missing"`) {
		t.Fatalf("CreateRoom() error = %q, want creator_id detail", err)
	}
}
