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
	if len(room.Participants) != 2 || !containsUserIDInRoom(*room, "u-admin") || !containsUserIDInRoom(*room, "u-alice") {
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

func TestAddAgentToRoomSupportsRoomID(t *testing.T) {
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

	room, err := svc.CreateRoom(CreateRoomRequest{
		Title:          "Ops",
		CreatorID:      "u-admin",
		ParticipantIDs: []string{"u-manager"},
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	updated, err := svc.AddAgentToRoom(AddAgentToConversationRequest{
		AgentID:   "u-alice",
		RoomID:    room.ID,
		InviterID: "u-admin",
	})
	if err != nil {
		t.Fatalf("AddAgentToRoom() error = %v", err)
	}
	if !containsUserIDInRoom(updated, "u-alice") {
		t.Fatalf("AddAgentToRoom() participants = %+v, want agent joined", updated.Participants)
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

func TestDeleteRoomRemovesRoom(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Conversations: []Conversation{
			{ID: "room-1", Title: "Room One", Participants: []string{"u-admin", "u-manager"}},
		},
	})

	if err := svc.DeleteRoom("room-1"); err != nil {
		t.Fatalf("DeleteRoom() error = %v", err)
	}
	if _, ok := svc.Room("room-1"); ok {
		t.Fatal("Room() ok = true, want false after delete")
	}
}

func TestKickUserRemovesUserFromStateConversationsAndMessages(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-admin", Name: "Admin", Handle: "admin"},
			{ID: "u-alice", Name: "Alice", Handle: "alice"},
			{ID: "u-bob", Name: "Bob", Handle: "bob"},
		},
		Conversations: []Conversation{
			{
				ID:           "room-group",
				Title:        "Group",
				Participants: []string{"u-admin", "u-alice", "u-bob"},
				Messages: []Message{
					{ID: "msg-1", SenderID: "u-alice", Content: "hello"},
					{ID: "msg-2", SenderID: "u-bob", Content: "world"},
				},
			},
			{
				ID:           "room-dm",
				Title:        "Alice",
				Participants: []string{"u-admin", "u-alice"},
				Messages:     []Message{{ID: "msg-3", SenderID: "u-alice", Content: "ping"}},
			},
		},
	})

	if err := svc.KickUser("u-alice"); err != nil {
		t.Fatalf("KickUser() error = %v", err)
	}
	if _, ok := svc.User("u-alice"); ok {
		t.Fatal("User() ok = true, want false after kick")
	}

	group, ok := svc.Room("room-group")
	if !ok {
		t.Fatal("Room(room-group) ok = false, want true")
	}
	if containsUserIDInRoom(group, "u-alice") {
		t.Fatalf("group participants = %+v, want u-alice removed", group.Participants)
	}
	if len(group.Messages) != 1 || group.Messages[0].SenderID != "u-bob" {
		t.Fatalf("group messages = %+v, want only u-bob message", group.Messages)
	}

	if _, ok := svc.Room("room-dm"); ok {
		t.Fatal("Room(room-dm) ok = true, want DM deleted after kick")
	}
}

func TestKickUserRejectsCurrentUser(t *testing.T) {
	svc := NewService()

	if err := svc.KickUser("u-admin"); err == nil {
		t.Fatal("KickUser(current user) error = nil, want error")
	}
}
