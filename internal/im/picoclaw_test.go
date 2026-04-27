package im

import (
	"testing"
	"time"
)

func TestChatTypeForRoomRespectsIsDirect(t *testing.T) {
	tests := []struct {
		name string
		room Room
		want string
	}{
		{
			name: "direct room stays direct",
			room: Room{
				ID:       "room-direct",
				IsDirect: true,
				Members:  []string{"u-admin", "u-bot"},
			},
			want: "direct",
		},
		{
			name: "two member group stays group",
			room: Room{
				ID:       "room-group",
				IsDirect: false,
				Members:  []string{"u-admin", "u-bot"},
			},
			want: "group",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := chatTypeForRoom(tc.room); got != tc.want {
				t.Fatalf("chatTypeForRoom() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShouldNotifyBotPushesForTwoMemberGroupWithoutMention(t *testing.T) {
	room := Room{
		ID:       "room-group",
		IsDirect: false,
		Members:  []string{"u-admin", "u-bot"},
	}

	message := Message{
		ID:        "msg-1",
		SenderID:  "u-admin",
		Content:   "hello",
		CreatedAt: time.Now().UTC(),
	}

	if !shouldNotifyBot(room, message, "u-bot") {
		t.Fatal("shouldNotifyBot() = false, want true for room member without mention")
	}
}

func TestPublishMessageEventUsesGroupChatTypeForTwoMemberGroup(t *testing.T) {
	bridge := NewPicoClawBridge("")
	events, cancel := bridge.Subscribe("u-bot")
	defer cancel()

	room := Room{
		ID:       "room-group",
		IsDirect: false,
		Members:  []string{"u-admin", "u-bot"},
	}
	sender := User{ID: "u-admin", Name: "Admin", Handle: "admin"}
	message := Message{
		ID:        "msg-1",
		SenderID:  "u-admin",
		Content:   "hello",
		CreatedAt: time.Now().UTC(),
	}

	bridge.PublishMessageEvent(room, sender, message)

	select {
	case evt := <-events:
		if evt.ChatType != "group" {
			t.Fatalf("PublishMessageEvent() chat_type = %q, want group", evt.ChatType)
		}
	case <-time.After(time.Second):
		t.Fatal("PublishMessageEvent() timed out waiting for event")
	}
}
