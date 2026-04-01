package im

import (
	"testing"
	"time"
)

func TestEnsureWorkerUserCreatesUserAndBootstrapRoom(t *testing.T) {
	svc := NewService()

	user, room, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID:     "u-alice",
		Name:   "Alice",
		Handle: "alice",
		Role:   "Worker",
	})
	if err != nil {
		t.Fatalf("EnsureWorkerUser() error = %v", err)
	}
	if user.ID != "u-alice" || user.Handle != "alice" {
		t.Fatalf("EnsureWorkerUser() user = %+v, want id/handle set", user)
	}
	if room == nil {
		t.Fatal("EnsureWorkerUser() room = nil, want bootstrap room")
	}
	if len(room.Participants) != 2 || !containsUserIDInConversation(*room, "u-admin") || !containsUserIDInConversation(*room, "u-alice") {
		t.Fatalf("EnsureWorkerUser() room participants = %+v, want admin and worker", room.Participants)
	}
}

func TestEnsureWorkerUserRejectsDuplicateHandle(t *testing.T) {
	svc := NewService()
	_, _, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID:     "u-alice",
		Name:   "Alice",
		Handle: "alice",
	})
	if err != nil {
		t.Fatalf("EnsureWorkerUser() first call error = %v", err)
	}

	_, _, err = svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID:     "u-bob",
		Name:   "Bob",
		Handle: "alice",
	})
	if err == nil {
		t.Fatal("EnsureWorkerUser() duplicate handle error = nil, want error")
	}
}

func TestAddAgentToConversationSupportsRoomID(t *testing.T) {
	svc := NewService()

	_, _, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID:     "u-alice",
		Name:   "Alice",
		Handle: "alice",
		Role:   "Worker",
	})
	if err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}

	conversation, err := svc.CreateConversation(CreateConversationRequest{
		Title:          "Ops",
		CreatorID:      "u-admin",
		ParticipantIDs: []string{"u-manager"},
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}

	updated, err := svc.AddAgentToConversation(AddAgentToConversationRequest{
		AgentID:   "u-alice",
		RoomID:    conversation.ID,
		InviterID: "u-admin",
	})
	if err != nil {
		t.Fatalf("AddAgentToConversation() error = %v", err)
	}
	if !containsUserIDInConversation(updated, "u-alice") {
		t.Fatalf("AddAgentToConversation() participants = %+v, want agent joined", updated.Participants)
	}
}

func TestListRoomsUsersAndMessages(t *testing.T) {
	earlier := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	later := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-zed", Name: "Zed", Handle: "zed", Role: "Worker"},
			{ID: "u-alice", Name: "Alice", Handle: "alice", Role: "Worker"},
		},
		Conversations: []Conversation{
			{
				ID:           "room-1",
				Title:        "First",
				Participants: []string{"u-admin", "u-alice"},
				Messages:     []Message{{ID: "msg-1", SenderID: "u-admin", Content: "first", CreatedAt: earlier}},
			},
			{
				ID:           "room-2",
				Title:        "Second",
				Participants: []string{"u-admin", "u-zed"},
				Messages:     []Message{{ID: "msg-2", SenderID: "u-zed", Content: "second", CreatedAt: later}},
			},
		},
	})

	rooms := svc.ListRooms()
	if len(rooms) != 2 {
		t.Fatalf("len(ListRooms()) = %d, want 2", len(rooms))
	}
	if rooms[0].ID != "room-2" || rooms[1].ID != "room-1" {
		t.Fatalf("ListRooms() order = [%s, %s], want room-2 then room-1", rooms[0].ID, rooms[1].ID)
	}

	users := svc.ListUsers()
	if len(users) != 4 {
		t.Fatalf("len(ListUsers()) = %d, want 4 including ensured admin/manager", len(users))
	}
	if users[0].Name != "Admin" || users[1].Name != "Alice" {
		t.Fatalf("ListUsers() leading order = [%s, %s], want Admin then Alice", users[0].Name, users[1].Name)
	}

	gotMessages, err := svc.ListMessages("room-1")
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(gotMessages) != 1 || gotMessages[0].ID != "msg-1" {
		t.Fatalf("ListMessages() = %+v, want msg-1", gotMessages)
	}

	if _, err := svc.ListMessages(""); err == nil {
		t.Fatal("ListMessages(\"\") error = nil, want error")
	}
	if _, err := svc.ListMessages("missing"); err == nil {
		t.Fatal("ListMessages(\"missing\") error = nil, want error")
	}
}
