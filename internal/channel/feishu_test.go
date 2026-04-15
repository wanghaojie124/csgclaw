package channel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestFeishuServiceInitializesMessageBus(t *testing.T) {
	svc := NewFeishuService()

	if svc.MessageBus() == nil {
		t.Fatal("MessageBus() = nil, want initialized bus")
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

func TestFeishuListUsersUsesConfiguredAppsAndOpenIDs(t *testing.T) {
	svc := NewFeishuServiceWithBotOpenIDResolver(
		map[string]FeishuAppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
			"u-dev":     {AppID: "cli_dev", AppSecret: "dev-secret"},
		},
		func(_ context.Context, app FeishuAppConfig) (string, error) {
			switch app.AppID {
			case "cli_manager":
				return "ou_manager", nil
			case "cli_dev":
				return "ou_dev", nil
			default:
				return "", nil
			}
		},
	)

	users := svc.ListUsers()
	if len(users) != 2 {
		t.Fatalf("len(ListUsers()) = %d, want 2", len(users))
	}
	if got, want := users[0].ID, "ou_dev"; got != want {
		t.Fatalf("users[0].ID = %q, want %q", got, want)
	}
	if got, want := users[0].Name, "u-dev"; got != want {
		t.Fatalf("users[0].Name = %q, want %q", got, want)
	}
	if got, want := users[1].ID, "ou_manager"; got != want {
		t.Fatalf("users[1].ID = %q, want %q", got, want)
	}
	if got, want := users[1].Name, "u-manager"; got != want {
		t.Fatalf("users[1].Name = %q, want %q", got, want)
	}
}

func TestFeishuResolveBotUserUsesConfiguredOpenID(t *testing.T) {
	svc := NewFeishuServiceWithBotOpenIDResolver(
		map[string]FeishuAppConfig{
			"u-alice": {AppID: "cli_alice", AppSecret: "alice-secret"},
		},
		func(_ context.Context, app FeishuAppConfig) (string, error) {
			if got, want := app.AppID, "cli_alice"; got != want {
				t.Fatalf("resolve app_id = %q, want %q", got, want)
			}
			return "ou_alice", nil
		},
	)

	user, ok, err := svc.ResolveBotUser(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("ResolveBotUser() error = %v", err)
	}
	if !ok {
		t.Fatal("ResolveBotUser() ok = false, want true")
	}
	if got, want := user.ID, "ou_alice"; got != want {
		t.Fatalf("user.ID = %q, want %q", got, want)
	}
	if got, want := user.Name, "u-alice"; got != want {
		t.Fatalf("user.Name = %q, want %q", got, want)
	}
}

func TestFeishuEnsureUserUsesConfiguredOpenID(t *testing.T) {
	svc := NewFeishuServiceWithBotOpenIDResolver(
		map[string]FeishuAppConfig{
			"u-alice": {AppID: "cli_alice", AppSecret: "alice-secret"},
		},
		func(_ context.Context, app FeishuAppConfig) (string, error) {
			if got, want := app.AppID, "cli_alice"; got != want {
				t.Fatalf("resolve app_id = %q, want %q", got, want)
			}
			return "ou_alice", nil
		},
	)

	user, err := svc.EnsureUser(FeishuCreateUserRequest{
		ID:     "u-alice",
		Name:   "alice",
		Handle: "alice",
		Role:   "worker",
	})
	if err != nil {
		t.Fatalf("EnsureUser() error = %v", err)
	}
	if got, want := user.ID, "ou_alice"; got != want {
		t.Fatalf("user.ID = %q, want %q", got, want)
	}
	if got, want := user.Name, "u-alice"; got != want {
		t.Fatalf("user.Name = %q, want %q", got, want)
	}
}

func TestFeishuBotMembersInChatWithResolversIncludesConfiguredBots(t *testing.T) {
	apps := map[string]FeishuAppConfig{
		"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
		"u-dev":     {AppID: "cli_dev", AppSecret: "dev-secret"},
		"u-qa":      {AppID: "cli_qa", AppSecret: "qa-secret"},
	}
	seenChecks := make([]string, 0)
	members, err := feishuBotMembersInChatWithResolvers(
		context.Background(),
		apps,
		"oc_alpha",
		map[string]struct{}{"ou_existing": {}},
		func(_ context.Context, app FeishuAppConfig) (string, error) {
			switch app.AppID {
			case "cli_manager":
				return "ou_manager", nil
			case "cli_dev":
				return "ou_existing", nil
			case "cli_qa":
				return "ou_qa", nil
			default:
				return "", nil
			}
		},
		func(_ context.Context, app FeishuAppConfig, chatID string) (bool, error) {
			if got, want := chatID, "oc_alpha"; got != want {
				t.Fatalf("chat_id = %q, want %q", got, want)
			}
			seenChecks = append(seenChecks, app.AppID)
			return app.AppID != "cli_qa", nil
		},
	)
	if err != nil {
		t.Fatalf("feishuBotMembersInChatWithResolvers() error = %v", err)
	}
	if len(seenChecks) != 3 {
		t.Fatalf("checked apps = %+v, want all configured apps", seenChecks)
	}
	if len(members) != 1 {
		t.Fatalf("members len = %d, want 1", len(members))
	}
	if got, want := members[0].ID, "ou_manager"; got != want {
		t.Fatalf("member id = %q, want %q", got, want)
	}
	if got, want := members[0].Name, "u-manager"; got != want {
		t.Fatalf("member name = %q, want %q", got, want)
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
			return FeishuSendMessageResponse{MessageID: "om_1", SenderOpenID: "ou_manager"}, nil
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
	if message.ID != "om_1" || message.SenderID != "ou_manager" || message.Content != "hello" {
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
			return FeishuSendMessageResponse{MessageID: "om_mention", SenderOpenID: "ou_manager", MentionOpenID: "ou_dev"}, nil
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
	if message.SenderID != "ou_manager" {
		t.Fatalf("message sender_id = %q, want ou_manager", message.SenderID)
	}
	if len(message.Mentions) != 1 || message.Mentions[0] != "ou_dev" {
		t.Fatalf("message mentions = %+v, want ou_dev", message.Mentions)
	}
}

func TestFeishuSendMessageWithMentionPublishesMessageEvent(t *testing.T) {
	svc := NewFeishuServiceWithSendMessage(
		map[string]FeishuAppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
			"u-dev":     {AppID: "cli_dev", AppSecret: "dev-secret"},
		},
		func(_ context.Context, _ FeishuAppConfig, _ FeishuSendMessageRequest) (FeishuSendMessageResponse, error) {
			return FeishuSendMessageResponse{MessageID: "om_mention", SenderOpenID: "ou_manager", MentionOpenID: "ou_dev"}, nil
		},
	)
	svc.rooms["oc_alpha"] = &im.Room{ID: "oc_alpha", Title: "alpha", Participants: []string{"u-manager", "u-dev"}}
	events, cancel := svc.MessageBus().Subscribe()
	defer cancel()

	message, err := svc.SendMessage(im.CreateMessageRequest{
		RoomID:    "oc_alpha",
		SenderID:  "u-manager",
		Content:   "hello",
		MentionID: "u-dev",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	select {
	case evt := <-events:
		if evt.Type != FeishuMessageEventTypeMessageCreated {
			t.Fatalf("event type = %q, want %q", evt.Type, FeishuMessageEventTypeMessageCreated)
		}
		if evt.RoomID != "oc_alpha" {
			t.Fatalf("event room_id = %q, want oc_alpha", evt.RoomID)
		}
		if evt.Message == nil || evt.Message.ID != message.ID {
			t.Fatalf("event message = %+v, want message %q", evt.Message, message.ID)
		}
		if evt.Message.SenderID != "ou_manager" {
			t.Fatalf("event sender_id = %q, want ou_manager", evt.Message.SenderID)
		}
		if len(evt.Message.Mentions) != 1 || evt.Message.Mentions[0] != "ou_dev" {
			t.Fatalf("event mentions = %+v, want ou_dev", evt.Message.Mentions)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for feishu message event")
	}
}

func TestFeishuSendMessageWithoutMentionDoesNotPublishMessageEvent(t *testing.T) {
	svc := NewFeishuServiceWithSendMessage(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(_ context.Context, _ FeishuAppConfig, _ FeishuSendMessageRequest) (FeishuSendMessageResponse, error) {
			return FeishuSendMessageResponse{MessageID: "om_plain", SenderOpenID: "ou_manager"}, nil
		},
	)
	svc.rooms["oc_alpha"] = &im.Room{ID: "oc_alpha", Title: "alpha", Participants: []string{"u-manager"}}
	events, cancel := svc.MessageBus().Subscribe()
	defer cancel()

	if _, err := svc.SendMessage(im.CreateMessageRequest{
		RoomID:   "oc_alpha",
		SenderID: "u-manager",
		Content:  "hello",
	}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	select {
	case evt := <-events:
		t.Fatalf("unexpected event = %+v", evt)
	case <-time.After(20 * time.Millisecond):
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

func TestFeishuListRoomMessagesFetchesAllMessagesAndUpdatesCache(t *testing.T) {
	var gotApp FeishuAppConfig
	var gotRoomID string
	fetchedAt := time.Unix(5, 0).UTC()
	svc := NewFeishuServiceWithListRoomMessages(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(_ context.Context, app FeishuAppConfig, roomID string) ([]im.Message, error) {
			gotApp = app
			gotRoomID = roomID
			return []im.Message{
				{ID: "om_1", SenderID: "ou_manager", Kind: im.MessageKindMessage, Content: "hello", CreatedAt: fetchedAt},
				{ID: "om_2", SenderID: "ou_alice", Kind: im.MessageKindMessage, Content: "world", CreatedAt: fetchedAt.Add(time.Second)},
			}, nil
		},
	)
	svc.rooms["oc_alpha"] = &im.Room{
		ID:       "oc_alpha",
		Title:    "alpha",
		Messages: []im.Message{{ID: "om_old", Content: "old"}},
	}

	messages, err := svc.ListRoomMessages("oc_alpha")
	if err != nil {
		t.Fatalf("ListRoomMessages() error = %v", err)
	}

	if gotApp.AppID != "cli_manager" {
		t.Fatalf("list messages app = %+v, want manager app", gotApp)
	}
	if gotRoomID != "oc_alpha" {
		t.Fatalf("list messages room_id = %q, want oc_alpha", gotRoomID)
	}
	if len(messages) != 2 || messages[0].ID != "om_1" || messages[1].ID != "om_2" {
		t.Fatalf("messages = %+v, want fetched messages", messages)
	}
	if len(svc.rooms["oc_alpha"].Messages) != 2 || svc.rooms["oc_alpha"].Messages[0].ID != "om_1" {
		t.Fatalf("cached messages = %+v, want fetched messages", svc.rooms["oc_alpha"].Messages)
	}
	messages[0].ID = "mutated"
	if got, want := svc.rooms["oc_alpha"].Messages[0].ID, "om_1"; got != want {
		t.Fatalf("cached message id after caller mutation = %q, want %q", got, want)
	}
}

func TestFeishuListRoomMessagesRequestsAPIWithoutLocalRoomValidation(t *testing.T) {
	var gotRoomIDs []string
	svc := NewFeishuServiceWithListRoomMessages(
		map[string]FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(_ context.Context, _ FeishuAppConfig, roomID string) ([]im.Message, error) {
			gotRoomIDs = append(gotRoomIDs, roomID)
			return []im.Message{{ID: "om_1"}}, nil
		},
	)

	if _, err := svc.ListRoomMessages(" "); err != nil {
		t.Fatalf("ListRoomMessages() with blank room_id error = %v", err)
	}
	if _, err := svc.ListRoomMessages("missing"); err != nil {
		t.Fatalf("ListRoomMessages() with missing local room error = %v", err)
	}

	if len(gotRoomIDs) != 2 || gotRoomIDs[0] != " " || gotRoomIDs[1] != "missing" {
		t.Fatalf("list messages room_ids = %+v, want blank and missing room ids passed through", gotRoomIDs)
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
	svc.listChatMembers = func(_ context.Context, app FeishuAppConfig, apps map[string]FeishuAppConfig, roomID string) ([]im.User, error) {
		gotApp = app
		gotRoomID = roomID
		if got, want := apps["u-manager"].AppID, "cli_manager"; got != want {
			t.Fatalf("list members apps manager app_id = %q, want %q", got, want)
		}
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
	svc.listChatMembers = func(_ context.Context, app FeishuAppConfig, _ map[string]FeishuAppConfig, roomID string) ([]im.User, error) {
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
