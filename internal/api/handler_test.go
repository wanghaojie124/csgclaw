package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	boxlite "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
)

func TestParsePicoClawBotPath(t *testing.T) {
	tests := []struct {
		path       string
		wantBotID  string
		wantAction string
		wantOK     bool
	}{
		{path: "/api/bots/u-manager/events", wantBotID: "u-manager", wantAction: "events", wantOK: true},
		{path: "/api/bots/u-manager/messages/send", wantBotID: "u-manager", wantAction: "messages/send", wantOK: true},
		{path: "/api/bots/u-manager", wantOK: false},
		{path: "/api/v1/bots/u-manager/events", wantOK: false},
		{path: "/api/bots//events", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			gotBotID, gotAction, gotOK := parsePicoClawBotPath(tt.path)
			if gotBotID != tt.wantBotID || gotAction != tt.wantAction || gotOK != tt.wantOK {
				t.Fatalf("parsePicoClawBotPath(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.path, gotBotID, gotAction, gotOK, tt.wantBotID, tt.wantAction, tt.wantOK)
			}
		})
	}
}

func TestDeriveAgentHandle(t *testing.T) {
	tests := []struct {
		name  string
		agent agent.Agent
		want  string
	}{
		{
			name:  "plain name",
			agent: agent.Agent{Name: "Alice Smith", ID: "u-alice", Role: agent.RoleWorker},
			want:  "alice-smith",
		},
		{
			name:  "fallback to id",
			agent: agent.Agent{Name: "中文 名字", ID: "u-worker_01", Role: agent.RoleWorker},
			want:  "worker_01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveAgentHandle(tt.agent); got != tt.want {
				t.Fatalf("deriveAgentHandle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnsureWorkerIMStatePublishesBootstrapRoom(t *testing.T) {
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	srv := &Handler{
		im:    im.NewService(),
		imBus: bus,
	}

	created := agent.Agent{
		ID:   "u-alice",
		Name: "Alice",
		Role: agent.RoleWorker,
	}
	if err := srv.ensureWorkerIMState(created); err != nil {
		t.Fatalf("ensureWorkerIMState() error = %v", err)
	}

	first := mustReceiveEvent(t, events)
	if first.Type != im.EventTypeUserCreated {
		t.Fatalf("first event.Type = %q, want %q", first.Type, im.EventTypeUserCreated)
	}
	if first.User == nil || first.User.ID != "u-alice" {
		t.Fatalf("first event.User = %+v, want u-alice", first.User)
	}

	second := mustReceiveEvent(t, events)
	if second.Type != im.EventTypeRoomCreated {
		t.Fatalf("second event.Type = %q, want %q", second.Type, im.EventTypeRoomCreated)
	}
	if second.Room == nil {
		t.Fatal("second event.Room = nil, want bootstrap room")
	}
	if second.Room.Title != "alice" {
		t.Fatalf("second event.Room.Title = %q, want %q", second.Room.Title, "alice")
	}
	if !containsParticipant(second.Room.Participants, "u-admin") || !containsParticipant(second.Room.Participants, "u-alice") {
		t.Fatalf("second event.Room.Participants = %+v, want admin and worker", second.Room.Participants)
	}
}

func TestHandleAgentsListReturnsUnifiedAgents(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{
		{ID: "u-manager", Name: "manager", Role: agent.RoleManager, CreatedAt: time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC)},
		{ID: "u-alice", Name: "alice", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
		{ID: "agent-1", Name: "observer", Role: agent.RoleAgent, CreatedAt: time.Date(2026, 3, 28, 11, 0, 0, 0, time.UTC)},
	})

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(agents) = %d, want 3; body=%s", len(got), rec.Body.String())
	}
	if got[0].ID != "u-manager" || got[1].ID != "u-alice" || got[2].ID != "agent-1" {
		t.Fatalf("agents = %+v, want manager/worker/agent in CreatedAt order", got)
	}
}

func TestHandleAgentsGetByIDReturnsAgent(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{
		{ID: "u-alice", Name: "alice", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	})

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-alice", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "u-alice" || got.Name != "alice" || got.Role != agent.RoleWorker {
		t.Fatalf("agent = %+v, want u-alice/alice/worker", got)
	}
}

func TestHandleAgentsGetByIDNotFound(t *testing.T) {
	svc := mustNewSeededService(t, nil)

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/missing", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleAgentsDeleteRemovesAgent(t *testing.T) {
	agent.SetTestHooks(
		func(_ *agent.Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		nil,
	)
	defer agent.ResetTestHooks()

	svc := mustNewSeededService(t, []agent.Agent{
		{ID: "u-alice", Name: "alice", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	})

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/u-alice", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if _, ok := svc.Agent("u-alice"); ok {
		t.Fatal("Agent() ok = true, want false after delete")
	}
}

func TestHandleAgentsDeleteNotFound(t *testing.T) {
	svc := mustNewSeededService(t, nil)

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/missing", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleAgentsCreateUsesWorkerCompatibilityFlow(t *testing.T) {
	agent.SetTestHooks(
		func(_ *agent.Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		func(_ *agent.Service, _ context.Context, _ *boxlite.Runtime, _ string, name, _, _ string) (*boxlite.Box, *boxlite.BoxInfo, error) {
			return nil, &boxlite.BoxInfo{
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				Name:      name,
				Image:     "test-image",
			}, nil
		},
	)
	defer agent.ResetTestHooks()

	svc := mustNewService(t)
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	srv := &Handler{
		svc:   svc,
		im:    im.NewService(),
		imBus: bus,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"alice","role":"worker"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "u-alice" || got.Role != agent.RoleWorker {
		t.Fatalf("agent = %+v, want worker alias result", got)
	}

	first := mustReceiveEvent(t, events)
	second := mustReceiveEvent(t, events)
	if first.Type != im.EventTypeUserCreated || second.Type != im.EventTypeRoomCreated {
		t.Fatalf("events = [%q, %q], want user_created then room_created", first.Type, second.Type)
	}
}

func TestHandleBootstrapAliasReturnsIMBootstrap(t *testing.T) {
	srv := &Handler{im: im.NewService()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bootstrap", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got struct {
		CurrentUserID string    `json:"current_user_id"`
		Users         []im.User `json:"users"`
		Rooms         []im.Room `json:"rooms"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.CurrentUserID == "" {
		t.Fatal("bootstrap current_user_id is empty")
	}
	if got.Rooms == nil {
		t.Fatal("bootstrap rooms is nil, want room-oriented DTO")
	}
}

func TestHandleRoomsInviteAliasAddsConversationMembers(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin"},
				{ID: "u-manager", Name: "manager", Handle: "manager"},
			},
			Conversations: []im.Conversation{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin"},
				},
			},
		}),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/invite", strings.NewReader(`{"room_id":"room-1","inviter_id":"u-admin","user_ids":["u-manager"],"locale":"en"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got im.Conversation
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "room-1" {
		t.Fatalf("conversation id = %q, want %q", got.ID, "room-1")
	}
	if !containsParticipant(got.Participants, "u-manager") {
		t.Fatalf("participants = %+v, want u-manager to be invited", got.Participants)
	}
}

func TestHandleIMAgentJoinReturnsCompactSuccessPayload(t *testing.T) {
	srv := &Handler{
		svc: mustNewSeededService(t, []agent.Agent{
			{ID: "u-alice", Name: "Alice", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)},
		}),
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin"},
				{ID: "u-alice", Name: "Alice", Handle: "alice"},
			},
			Conversations: []im.Conversation{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin"},
					Messages:     []im.Message{{ID: "msg-1", SenderID: "u-admin", Content: "hello"}},
				},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/im/agents/join", strings.NewReader(`{"agent_id":"u-alice","room_id":"room-1","locale":"en"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"messages"`) {
		t.Fatalf("body = %s, want compact success payload without messages", rec.Body.String())
	}

	var got struct {
		Message string `json:"message"`
		RoomID  string `json:"room_id"`
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Message != "agent joined successfully" {
		t.Fatalf("message = %q, want success message", got.Message)
	}
	if got.RoomID != "room-1" || got.AgentID != "u-alice" {
		t.Fatalf("response = %+v, want room-1/u-alice", got)
	}
	if room, ok := srv.im.Room("room-1"); !ok || !containsParticipant(room.Participants, "u-alice") {
		t.Fatalf("room participants = %+v, want agent joined", room.Participants)
	}
}

func TestHandleRoomsInviteRequiresRoomID(t *testing.T) {
	srv := &Handler{im: im.NewService()}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/invite", strings.NewReader(`{"inviter_id":"u-admin","user_ids":["u-manager"]}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWorkersPostRemainsCreateAlias(t *testing.T) {
	agent.SetTestHooks(
		func(_ *agent.Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		func(_ *agent.Service, _ context.Context, _ *boxlite.Runtime, _ string, name, _, _ string) (*boxlite.Box, *boxlite.BoxInfo, error) {
			return nil, &boxlite.BoxInfo{
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				Name:      name,
				Image:     "test-image",
			}, nil
		},
	)
	defer agent.ResetTestHooks()

	svc := mustNewService(t)
	srv := &Handler{
		svc: svc,
		im:  im.NewService(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workers", strings.NewReader(`{"name":"bob"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "u-bob" || got.Role != agent.RoleWorker {
		t.Fatalf("agent = %+v, want worker alias result", got)
	}
}

func TestHandleRoomsReturnsConversationList(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-alice", Name: "Alice", Handle: "alice"},
			},
			Conversations: []im.Conversation{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin", "u-alice"},
					Messages: []im.Message{{
						ID:        "msg-1",
						SenderID:  "u-admin",
						Content:   "hello",
						CreatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
					}},
				},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []im.Conversation
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "room-1" {
		t.Fatalf("rooms = %+v, want room-1", got)
	}
}

func TestHandleUsersReturnsUserList(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-zed", Name: "Zed", Handle: "zed"},
				{ID: "u-alice", Name: "Alice", Handle: "alice"},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 4 || got[0].Name != "admin" || got[1].Name != "alice" || got[2].Name != "manager" || got[3].Name != "zed" {
		t.Fatalf("users = %+v, want admin/alice/manager/zed", got)
	}
}

func TestHandleMessagesReturnsConversationMessages(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Conversations: []im.Conversation{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin", "u-manager"},
					Messages: []im.Message{{
						ID:        "msg-1",
						SenderID:  "u-admin",
						Content:   "hello",
						CreatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
					}},
				},
			},
		}),
	}

	for _, path := range []string{
		"/api/v1/messages?room_id=room-1",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("path %s status = %d, want %d; body=%s", path, rec.Code, http.StatusOK, rec.Body.String())
		}
		var got []im.Message
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("path %s decode response: %v", path, err)
		}
		if len(got) != 1 || got[0].ID != "msg-1" {
			t.Fatalf("path %s messages = %+v, want msg-1", path, got)
		}
	}
}

func TestHandleMessagesRejectsInvalidQuery(t *testing.T) {
	srv := &Handler{im: im.NewService()}

	for _, path := range []string{
		"/api/v1/messages",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("path %s status = %d, want %d", path, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestHandleMessagesPostCreatesMessage(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin"},
				{ID: "u-manager", Name: "manager", Handle: "manager"},
			},
			Conversations: []im.Conversation{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin", "u-manager"},
				},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"room-1","sender_id":"u-admin","content":"hello @manager"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got im.Message
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.SenderID != "u-admin" || got.Content != "hello @manager" {
		t.Fatalf("message = %+v, want sender/content populated", got)
	}
	if len(got.Mentions) != 1 || got.Mentions[0] != "u-manager" {
		t.Fatalf("mentions = %+v, want u-manager", got.Mentions)
	}
}

func TestHandleMessagesPostRequiresRoomID(t *testing.T) {
	srv := &Handler{im: im.NewService()}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"sender_id":"u-admin","content":"hello"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleIMEventsExposeRoomIDOnly(t *testing.T) {
	bus := im.NewBus()
	srv := &Handler{imBus: bus}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/im/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.Routes().ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	bus.Publish(im.Event{
		Type:   im.EventTypeMessageCreated,
		RoomID: "room-1",
		Message: &im.Message{
			ID:       "msg-1",
			SenderID: "u-admin",
			Content:  "hello",
		},
		Sender: &im.User{ID: "u-admin", Name: "admin", Handle: "admin"},
	})
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, `"room_id":"room-1"`) {
		t.Fatalf("body = %q, want room_id", body)
	}
	if strings.Contains(body, `"conversation_id"`) {
		t.Fatalf("body = %q, want no conversation_id compatibility field", body)
	}
}

func TestHandleRoomsPostCreatesRoom(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin"},
				{ID: "u-alice", Name: "Alice", Handle: "alice"},
				{ID: "u-manager", Name: "manager", Handle: "manager"},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"title":"Launch","description":"coordination","creator_id":"u-admin","participant_ids":["u-alice","u-manager"],"locale":"en"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got im.Conversation
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Title != "Launch" {
		t.Fatalf("conversation.Title = %q, want Launch", got.Title)
	}
	if !containsParticipant(got.Participants, "u-admin") || !containsParticipant(got.Participants, "u-alice") || !containsParticipant(got.Participants, "u-manager") {
		t.Fatalf("participants = %+v, want admin, alice, and manager", got.Participants)
	}
}

func TestHandleUsersDeleteKicksUser(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin", IsOnline: true},
				{ID: "u-alice", Name: "Alice", Handle: "alice", IsOnline: true},
			},
			Conversations: []im.Conversation{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin", "u-alice"},
					Messages:     []im.Message{{ID: "msg-1", SenderID: "u-alice", Content: "hello"}},
				},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/u-alice", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if _, ok := srv.im.User("u-alice"); ok {
		t.Fatal("User() ok = true, want false after delete")
	}
	if _, ok := srv.im.Room("room-1"); ok {
		t.Fatal("Room() ok = true, want false for DM after kicked user")
	}
}

func TestHandleUsersDeleteCurrentUserReturnsConflict(t *testing.T) {
	srv := &Handler{im: im.NewService()}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/u-admin", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleRoomsDeleteRemovesRoom(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Conversations: []im.Conversation{
				{ID: "room-1", Title: "Room One", Participants: []string{"u-admin", "u-manager"}},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/rooms/room-1", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if _, ok := srv.im.Room("room-1"); ok {
		t.Fatal("Room() ok = true, want false after delete")
	}
}

func TestHandlePicoClawRoutesRequireAuthorization(t *testing.T) {
	srv := &Handler{
		im:       im.NewService(),
		picoclaw: im.NewPicoClawBridge(config.PicoClawConfig{AccessToken: "secret"}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/bots/u-manager/messages/send", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandlePicoClawSendMessageRequiresIMService(t *testing.T) {
	srv := &Handler{
		picoclaw: im.NewPicoClawBridge(config.PicoClawConfig{}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/bots/u-manager/messages/send", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func mustNewService(t *testing.T) *agent.Service {
	t.Helper()

	svc, err := agent.NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

func mustNewSeededService(t *testing.T, agents []agent.Agent) *agent.Service {
	t.Helper()

	if agents == nil {
		agents = []agent.Agent{}
	}

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(map[string]any{
		"agents": agents,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc, err := agent.NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

func containsParticipant(participants []string, want string) bool {
	for _, participant := range participants {
		if participant == want {
			return true
		}
	}
	return false
}

func mustReceiveEvent(t *testing.T, events <-chan im.Event) im.Event {
	t.Helper()

	select {
	case evt := <-events:
		return evt
	default:
		t.Fatal("expected event")
		return im.Event{}
	}
}
