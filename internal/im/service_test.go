package im

import "testing"

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
