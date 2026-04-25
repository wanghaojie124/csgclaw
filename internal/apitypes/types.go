package apitypes

import (
	"encoding/json"
	"time"
)

type Bot struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Role        string    `json:"role"`
	Channel     string    `json:"channel"`
	AgentID     string    `json:"agent_id"`
	UserID      string    `json:"user_id"`
	Available   bool      `json:"available"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateBotRequest struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Role        string `json:"role"`
	Channel     string `json:"channel,omitempty"`
	ModelID     string `json:"model_id,omitempty"`
}

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Handle    string    `json:"handle"`
	Role      string    `json:"role"`
	Avatar    string    `json:"avatar"`
	IsOnline  bool      `json:"is_online"`
	LastSeen  string    `json:"last_seen,omitempty"`
	AccentHex string    `json:"accent_hex"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type CreateUserRequest struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	Handle string `json:"handle,omitempty"`
	Role   string `json:"role,omitempty"`
}

type Message struct {
	ID        string        `json:"id"`
	SenderID  string        `json:"sender_id"`
	Kind      string        `json:"kind,omitempty"`
	Content   string        `json:"content"`
	Event     *EventPayload `json:"event,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	Mentions  []string      `json:"mentions"`
}

type CreateMessageRequest struct {
	RoomID    string `json:"room_id"`
	SenderID  string `json:"sender_id"`
	Content   string `json:"content"`
	MentionID string `json:"mention_id,omitempty"`
}

type EventPayload struct {
	Key       string   `json:"key"`
	ActorID   string   `json:"actor_id,omitempty"`
	Title     string   `json:"title,omitempty"`
	TargetIDs []string `json:"target_ids,omitempty"`
}

type Room struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Subtitle    string    `json:"subtitle"`
	Description string    `json:"description,omitempty"`
	IsDirect    bool      `json:"is_direct,omitempty"`
	Members     []string  `json:"members"`
	Messages    []Message `json:"messages"`
}

type CreateRoomRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	CreatorID   string   `json:"creator_id"`
	MemberIDs   []string `json:"member_ids"`
	Locale      string   `json:"locale"`
}

type AddRoomMembersRequest struct {
	RoomID    string   `json:"room_id,omitempty"`
	InviterID string   `json:"inviter_id"`
	UserIDs   []string `json:"user_ids"`
	Locale    string   `json:"locale"`
}

// UnmarshalJSON keeps room payload decoding backward-compatible with legacy participants fields.
func (r *Room) UnmarshalJSON(data []byte) error {
	type roomAlias Room
	type roomJSON struct {
		roomAlias
		Participants []string `json:"participants"`
	}

	var decoded roomJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = Room(decoded.roomAlias)
	if len(r.Members) == 0 && len(decoded.Participants) > 0 {
		r.Members = append([]string(nil), decoded.Participants...)
	}
	return nil
}

// UnmarshalJSON keeps create-room request decoding backward-compatible with legacy participant_ids.
func (r *CreateRoomRequest) UnmarshalJSON(data []byte) error {
	type createRoomAlias CreateRoomRequest
	type createRoomJSON struct {
		createRoomAlias
		ParticipantIDs []string `json:"participant_ids"`
	}

	var decoded createRoomJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = CreateRoomRequest(decoded.createRoomAlias)
	if len(r.MemberIDs) == 0 && len(decoded.ParticipantIDs) > 0 {
		r.MemberIDs = append([]string(nil), decoded.ParticipantIDs...)
	}
	return nil
}
