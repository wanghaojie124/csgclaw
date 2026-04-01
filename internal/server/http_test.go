package server

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

func TestEnsureWorkerIMStatePublishesBootstrapConversation(t *testing.T) {
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	srv := &HTTPServer{
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
	if second.Type != im.EventTypeConversationCreated {
		t.Fatalf("second event.Type = %q, want %q", second.Type, im.EventTypeConversationCreated)
	}
	if second.Conversation == nil {
		t.Fatal("second event.Conversation = nil, want bootstrap conversation")
	}
	if second.Conversation.Title != "Alice" {
		t.Fatalf("second event.Conversation.Title = %q, want %q", second.Conversation.Title, "Alice")
	}
	if !containsParticipant(second.Conversation.Participants, "u-admin") || !containsParticipant(second.Conversation.Participants, "u-alice") {
		t.Fatalf("second event.Conversation.Participants = %+v, want admin and worker", second.Conversation.Participants)
	}
}

func TestHandleAgentsListReturnsUnifiedAgents(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{
		{ID: "u-manager", Name: "manager", Role: agent.RoleManager, CreatedAt: time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC)},
		{ID: "u-alice", Name: "alice", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
		{ID: "agent-1", Name: "observer", Role: agent.RoleAgent, CreatedAt: time.Date(2026, 3, 28, 11, 0, 0, 0, time.UTC)},
	})

	srv := &HTTPServer{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

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

	srv := &HTTPServer{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-alice", nil)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

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

	srv := &HTTPServer{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/missing", nil)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

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

	srv := &HTTPServer{svc: svc}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/u-alice", nil)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if _, ok := svc.Agent("u-alice"); ok {
		t.Fatal("Agent() ok = true, want false after delete")
	}
}

func TestHandleAgentsDeleteNotFound(t *testing.T) {
	svc := mustNewSeededService(t, nil)

	srv := &HTTPServer{svc: svc}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/missing", nil)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

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

	srv := &HTTPServer{
		svc:   svc,
		im:    im.NewService(),
		imBus: bus,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"alice","role":"worker"}`))
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

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
	if first.Type != im.EventTypeUserCreated || second.Type != im.EventTypeConversationCreated {
		t.Fatalf("events = [%q, %q], want user_created then conversation_created", first.Type, second.Type)
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
	srv := &HTTPServer{
		svc: svc,
		im:  im.NewService(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workers", strings.NewReader(`{"name":"bob"}`))
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

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
	srv := &HTTPServer{
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
	srv.routes().ServeHTTP(rec, req)

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
	srv := &HTTPServer{
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
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) < 2 || got[0].Name != "Admin" || got[1].Name != "Alice" {
		t.Fatalf("users = %+v, want sorted users starting with Admin/Alice", got)
	}
}

func TestHandleMessagesReturnsConversationMessages(t *testing.T) {
	srv := &HTTPServer{
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
		"/api/v1/messages?conversation_id=room-1",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, req)

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
	srv := &HTTPServer{im: im.NewService()}

	for _, path := range []string{
		"/api/v1/messages",
		"/api/v1/messages?room_id=room-1&conversation_id=room-1",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("path %s status = %d, want %d", path, rec.Code, http.StatusBadRequest)
		}
	}
}

func mustNewService(t *testing.T) *agent.Service {
	t.Helper()

	svc, err := agent.NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", "", "")
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

	svc, err := agent.NewService(config.LLMConfig{}, config.ServerConfig{}, config.PicoClawConfig{}, "", statePath, "")
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
