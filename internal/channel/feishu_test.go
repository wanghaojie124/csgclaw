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
	svc := NewFeishuServiceWithCreateChatAndAddMembers(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(_ context.Context, _ FeishuAppConfig, req FeishuCreateChatRequest) (FeishuCreateChatResponse, error) {
			gotCreatorID = req.CreatorID
			return FeishuCreateChatResponse{ChatID: "oc_alpha", Name: req.Title, Description: req.Description}, nil
		},
		func(context.Context, FeishuAppConfig, FeishuAddChatMembersRequest) error { return nil },
	)

	if _, err := svc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-manager"}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	if got, want := gotCreatorID, "ou_admin"; got != want {
		t.Fatalf("create chat creator_id = %q, want %q", got, want)
	}
}

func TestFeishuSendMessageUsesSenderAppAndStoresLocalMessage(t *testing.T) {
	var gotApp FeishuAppConfig
	var gotReq FeishuSendMessageRequest
	svc := NewFeishuServiceWithSendMessage(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(_ context.Context, app FeishuAppConfig, req FeishuSendMessageRequest) (FeishuSendMessageResponse, error) {
			gotApp = app
			gotReq = req
			return FeishuSendMessageResponse{MessageID: "om_1"}, nil
		},
	)
	svc.rooms["oc_alpha"] = &im.Room{ID: "oc_alpha", Title: "alpha", Participants: []string{"u-manager"}}

	message, err := svc.SendMessage(im.CreateMessageRequest{
		RoomID:   "oc_alpha",
		SenderID: "u-manager",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if gotApp.AppID != "cli_manager" {
		t.Fatalf("send app = %+v, want manager app", gotApp)
	}
	if gotReq.ChatID != "oc_alpha" || gotReq.Content != "hello" || gotReq.UUID == "" {
		t.Fatalf("send request = %+v, want chat/content/uuid", gotReq)
	}
	if message.ID != "om_1" || message.SenderID != "u-manager" || message.Content != "hello" {
		t.Fatalf("message = %+v, want sent message", message)
	}
	if len(svc.rooms["oc_alpha"].Messages) != 1 || svc.rooms["oc_alpha"].Messages[0].ID != "om_1" {
		t.Fatalf("stored messages = %+v, want om_1", svc.rooms["oc_alpha"].Messages)
	}
}

func TestFeishuSendMessageResolvesMentionApp(t *testing.T) {
	var gotReq FeishuSendMessageRequest
	svc := NewFeishuServiceWithSendMessage(
		map[string]FeishuAppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
			"u-dev":     {AppID: "cli_dev", AppSecret: "dev-secret"},
		},
		func(_ context.Context, _ FeishuAppConfig, req FeishuSendMessageRequest) (FeishuSendMessageResponse, error) {
			gotReq = req
			return FeishuSendMessageResponse{MessageID: "om_mention"}, nil
		},
	)
	svc.rooms["oc_alpha"] = &im.Room{ID: "oc_alpha", Title: "alpha", Participants: []string{"u-manager", "u-dev"}}

	message, err := svc.SendMessage(im.CreateMessageRequest{
		RoomID:    "oc_alpha",
		SenderID:  "u-manager",
		Content:   "hello",
		MentionID: "u-dev",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if gotReq.MentionID != "u-dev" || gotReq.MentionAppConfig.AppID != "cli_dev" || gotReq.MentionAppConfig.AppSecret != "dev-secret" {
		t.Fatalf("send request = %+v, want mention app config", gotReq)
	}
	if len(message.Mentions) != 1 || message.Mentions[0] != "u-dev" {
		t.Fatalf("message mentions = %+v, want u-dev", message.Mentions)
	}
}

func TestFeishuSendMessageRequiresMentionApp(t *testing.T) {
	svc := NewFeishuServiceWithSendMessage(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(context.Context, FeishuAppConfig, FeishuSendMessageRequest) (FeishuSendMessageResponse, error) {
			t.Fatal("sendMessage should not be called without mention app config")
			return FeishuSendMessageResponse{}, nil
		},
	)

	_, err := svc.SendMessage(im.CreateMessageRequest{
		RoomID:    "oc_alpha",
		SenderID:  "u-manager",
		Content:   "hello",
		MentionID: "u-dev",
	})
	if err == nil || !strings.Contains(err.Error(), `feishu app is not configured for mention "u-dev"`) {
		t.Fatalf("SendMessage() error = %v, want mention app config error", err)
	}
}

func TestFeishuCreateRoomUsesManagerAppRegardlessOfCreatorID(t *testing.T) {
	svc := NewFeishuServiceWithCreateChatAndAddMembers(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(_ context.Context, app FeishuAppConfig, _ FeishuCreateChatRequest) (FeishuCreateChatResponse, error) {
			if got, want := app.AppID, "cli_manager"; got != want {
				t.Fatalf("create chat app_id = %q, want %q", got, want)
			}
			return FeishuCreateChatResponse{ChatID: "oc_alpha"}, nil
		},
		func(context.Context, FeishuAppConfig, FeishuAddChatMembersRequest) error { return nil },
	)

	room, err := svc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-missing"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if got, want := room.ID, "oc_alpha"; got != want {
		t.Fatalf("room id = %q, want %q", got, want)
	}
}

func TestFeishuListRoomsCallsConfiguredApp(t *testing.T) {
	var gotApp FeishuAppConfig
	svc := NewFeishuServiceWithCreateChatAndAddMembers(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(_ context.Context, _ FeishuAppConfig, req FeishuCreateChatRequest) (FeishuCreateChatResponse, error) {
			return FeishuCreateChatResponse{ChatID: "oc_alpha", Name: req.Title, Description: req.Description}, nil
		},
		func(context.Context, FeishuAppConfig, FeishuAddChatMembersRequest) error { return nil },
	)
	svc.listChats = func(_ context.Context, app FeishuAppConfig) ([]im.Room, error) {
		gotApp = app
		return []im.Room{
			{ID: "oc_beta", Title: "beta"},
			{ID: "oc_alpha", Title: "alpha"},
		}, nil
	}

	if _, err := svc.CreateRoom(im.CreateRoomRequest{
		Title:          "alpha",
		CreatorID:      "u-manager",
		ParticipantIDs: []string{"ou_alice"},
	}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	rooms, err := svc.ListRooms()
	if err != nil {
		t.Fatalf("ListRooms() error = %v", err)
	}

	if got, want := gotApp.AppID, "cli_manager"; got != want {
		t.Fatalf("list rooms app_id = %q, want %q", got, want)
	}
	if len(rooms) != 2 {
		t.Fatalf("rooms len = %d, want 2", len(rooms))
	}
	if got, want := rooms[0].ID, "oc_alpha"; got != want {
		t.Fatalf("first room id = %q, want %q", got, want)
	}
	if len(rooms[0].Participants) != 2 || rooms[0].Participants[0] != "u-manager" || rooms[0].Participants[1] != "ou_alice" {
		t.Fatalf("first room participants = %+v, want local participants", rooms[0].Participants)
	}
}

func TestFeishuAddRoomMembersCallsConfiguredApp(t *testing.T) {
	var gotApp FeishuAppConfig
	var gotReq FeishuAddChatMembersRequest
	svc := NewFeishuServiceWithCreateChatAndAddMembers(
		map[string]FeishuAppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"},
			"ou_alice":  {AppID: "cli_alice", AppSecret: "alice-secret"},
		},
		func(_ context.Context, _ FeishuAppConfig, req FeishuCreateChatRequest) (FeishuCreateChatResponse, error) {
			return FeishuCreateChatResponse{ChatID: "oc_alpha", Name: req.Title, Description: req.Description}, nil
		},
		func(_ context.Context, app FeishuAppConfig, req FeishuAddChatMembersRequest) error {
			gotApp = app
			gotReq = req
			return nil
		},
	)

	if _, err := svc.CreateUser(FeishuCreateUserRequest{ID: "u-manager", Name: "Manager"}); err != nil {
		t.Fatalf("CreateUser(manager) error = %v", err)
	}
	if _, err := svc.CreateUser(FeishuCreateUserRequest{ID: "ou_alice", Name: "Alice"}); err != nil {
		t.Fatalf("CreateUser(alice) error = %v", err)
	}
	if _, err := svc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-manager"}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	room, err := svc.AddRoomMembers(im.AddRoomMembersRequest{
		RoomID:  "oc_alpha",
		UserIDs: []string{"ou_alice"},
	})
	if err != nil {
		t.Fatalf("AddRoomMembers() error = %v", err)
	}

	if got, want := gotApp.AppID, "cli_manager"; got != want {
		t.Fatalf("add members app_id = %q, want %q", got, want)
	}
	if got, want := gotReq.ChatID, "oc_alpha"; got != want {
		t.Fatalf("add members chat_id = %q, want %q", got, want)
	}
	if len(gotReq.MemberIDs) != 1 || gotReq.MemberIDs[0] != "ou_alice" {
		t.Fatalf("add members ids = %+v, want [ou_alice]", gotReq.MemberIDs)
	}
	if len(gotReq.MemberAppIDs) != 1 || gotReq.MemberAppIDs[0] != "cli_alice" {
		t.Fatalf("add members app_ids = %+v, want [cli_alice]", gotReq.MemberAppIDs)
	}
	if len(room.Participants) != 2 {
		t.Fatalf("participants = %+v, want two users", room.Participants)
	}
}

func TestFeishuAddRoomMembersForwardsUnconfiguredMemberToFeishu(t *testing.T) {
	var gotReq FeishuAddChatMembersRequest
	svc := NewFeishuServiceWithCreateChatAndAddMembers(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(_ context.Context, _ FeishuAppConfig, req FeishuCreateChatRequest) (FeishuCreateChatResponse, error) {
			return FeishuCreateChatResponse{ChatID: "oc_alpha", Name: req.Title, Description: req.Description}, nil
		},
		func(_ context.Context, _ FeishuAppConfig, req FeishuAddChatMembersRequest) error {
			gotReq = req
			return nil
		},
	)

	if _, err := svc.CreateUser(FeishuCreateUserRequest{ID: "u-manager", Name: "Manager"}); err != nil {
		t.Fatalf("CreateUser(manager) error = %v", err)
	}
	if _, err := svc.CreateUser(FeishuCreateUserRequest{ID: "ou_alice", Name: "Alice"}); err != nil {
		t.Fatalf("CreateUser(alice) error = %v", err)
	}
	if _, err := svc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-manager"}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	_, err := svc.AddRoomMembers(im.AddRoomMembersRequest{
		RoomID:  "oc_alpha",
		UserIDs: []string{"ou_alice"},
	})
	if err != nil {
		t.Fatalf("AddRoomMembers() error = %v", err)
	}
	if len(gotReq.MemberAppIDs) != 1 || gotReq.MemberAppIDs[0] != "ou_alice" {
		t.Fatalf("add members app_ids = %+v, want [ou_alice]", gotReq.MemberAppIDs)
	}
}

func TestFeishuAddRoomMembersLetsFeishuValidateRoomID(t *testing.T) {
	var gotReq FeishuAddChatMembersRequest
	svc := NewFeishuServiceWithCreateChatAndAddMembers(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(context.Context, FeishuAppConfig, FeishuCreateChatRequest) (FeishuCreateChatResponse, error) {
			t.Fatal("createChat should not be called")
			return FeishuCreateChatResponse{}, nil
		},
		func(_ context.Context, _ FeishuAppConfig, req FeishuAddChatMembersRequest) error {
			gotReq = req
			return nil
		},
	)

	room, err := svc.AddRoomMembers(im.AddRoomMembersRequest{
		RoomID:    "oc_external",
		InviterID: "u-manager",
		UserIDs:   []string{"ou_alice"},
	})
	if err != nil {
		t.Fatalf("AddRoomMembers() error = %v", err)
	}
	if got, want := gotReq.ChatID, "oc_external"; got != want {
		t.Fatalf("add members chat_id = %q, want %q", got, want)
	}
	if got, want := room.ID, "oc_external"; got != want {
		t.Fatalf("room id = %q, want %q", got, want)
	}
	if len(room.Participants) != 1 || room.Participants[0] != "ou_alice" {
		t.Fatalf("participants = %+v, want [ou_alice]", room.Participants)
	}
}

func TestFeishuListRoomMembersCallsConfiguredApp(t *testing.T) {
	var gotApp FeishuAppConfig
	var gotRoomID string
	svc := NewFeishuServiceWithCreateChatAndAddMembers(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(_ context.Context, _ FeishuAppConfig, req FeishuCreateChatRequest) (FeishuCreateChatResponse, error) {
			return FeishuCreateChatResponse{ChatID: "oc_alpha", Name: req.Title, Description: req.Description}, nil
		},
		func(context.Context, FeishuAppConfig, FeishuAddChatMembersRequest) error { return nil },
	)
	svc.listChatMembers = func(_ context.Context, app FeishuAppConfig, roomID string) ([]im.User, error) {
		gotApp = app
		gotRoomID = roomID
		return []im.User{{ID: "ou_alice", Name: "Alice"}}, nil
	}

	if _, err := svc.CreateUser(FeishuCreateUserRequest{ID: "u-manager", Name: "Manager"}); err != nil {
		t.Fatalf("CreateUser(manager) error = %v", err)
	}
	if _, err := svc.CreateUser(FeishuCreateUserRequest{ID: "ou_alice", Name: "Alice Local", Handle: "alice-local", Role: "worker", Avatar: "AL"}); err != nil {
		t.Fatalf("CreateUser(alice) error = %v", err)
	}
	if _, err := svc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-manager"}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	members, err := svc.ListRoomMembers("oc_alpha")
	if err != nil {
		t.Fatalf("ListRoomMembers() error = %v", err)
	}
	if got, want := gotApp.AppID, "cli_manager"; got != want {
		t.Fatalf("list members app_id = %q, want %q", got, want)
	}
	if got, want := gotRoomID, "oc_alpha"; got != want {
		t.Fatalf("list members room_id = %q, want %q", got, want)
	}
	if len(members) != 1 {
		t.Fatalf("members len = %d, want 1", len(members))
	}
	if got, want := members[0].Name, "Alice"; got != want {
		t.Fatalf("member name = %q, want %q", got, want)
	}
	if got, want := members[0].Handle, "alice-local"; got != want {
		t.Fatalf("member handle = %q, want %q", got, want)
	}
	if got, want := members[0].Role, "worker"; got != want {
		t.Fatalf("member role = %q, want %q", got, want)
	}
}

func TestFeishuListRoomMembersLetsFeishuValidateExternalRoomID(t *testing.T) {
	var gotRoomID string
	svc := NewFeishuService(map[string]FeishuAppConfig{
		"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"},
	})
	svc.listChatMembers = func(_ context.Context, app FeishuAppConfig, roomID string) ([]im.User, error) {
		if got, want := app.AppID, "cli_manager"; got != want {
			t.Fatalf("list members app_id = %q, want %q", got, want)
		}
		gotRoomID = roomID
		return []im.User{{ID: "ou_alice", Name: "Alice"}}, nil
	}

	members, err := svc.ListRoomMembers("oc_external")
	if err != nil {
		t.Fatalf("ListRoomMembers() error = %v", err)
	}
	if got, want := gotRoomID, "oc_external"; got != want {
		t.Fatalf("list members room_id = %q, want %q", got, want)
	}
	if len(members) != 1 || members[0].ID != "ou_alice" {
		t.Fatalf("members = %+v, want ou_alice", members)
	}
}
