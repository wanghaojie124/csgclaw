package im

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"csgclaw/internal/config"
)

type PicoClawBridge struct {
	cfg         config.PicoClawConfig
	mu          sync.Mutex
	subscribers map[string]map[chan PicoClawEvent]struct{}
}

type PicoClawEvent struct {
	MessageID string         `json:"message_id"`
	ChatID    string         `json:"chat_id"`
	ChatType  string         `json:"chat_type"`
	Sender    PicoClawSender `json:"sender"`
	Text      string         `json:"text"`
	Timestamp string         `json:"timestamp"`
	Mentions  []string       `json:"mentions,omitempty"`
}

type PicoClawSender struct {
	ID          string `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type PicoClawSendMessageRequest struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

func NewPicoClawBridge(cfg config.PicoClawConfig) *PicoClawBridge {
	return &PicoClawBridge{
		cfg:         cfg,
		subscribers: make(map[string]map[chan PicoClawEvent]struct{}),
	}
}

func (b *PicoClawBridge) ValidateAccessToken(authHeader string) bool {
	if strings.TrimSpace(b.cfg.AccessToken) == "" {
		return true
	}
	return authHeader == "Bearer "+b.cfg.AccessToken
}

func (b *PicoClawBridge) Subscribe(botID string) (<-chan PicoClawEvent, func()) {
	ch := make(chan PicoClawEvent, 16)

	b.mu.Lock()
	if b.subscribers[botID] == nil {
		b.subscribers[botID] = make(map[chan PicoClawEvent]struct{})
	}
	b.subscribers[botID][ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if subs, ok := b.subscribers[botID]; ok {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(b.subscribers, botID)
			}
		}
		b.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

func (b *PicoClawBridge) PublishMessageEvent(room Room, sender User, message Message) {
	b.mu.Lock()
	targets := make(map[string][]chan PicoClawEvent, len(b.subscribers))
	for botID, subs := range b.subscribers {
		if !shouldNotifyBot(room, message, botID) {
			continue
		}
		for ch := range subs {
			targets[botID] = append(targets[botID], ch)
		}
	}
	b.mu.Unlock()

	for botID, subs := range targets {
		evt := PicoClawEvent{
			MessageID: message.ID,
			ChatID:    room.ID,
			ChatType:  chatTypeForRoom(room),
			Sender: PicoClawSender{
				ID:          sender.ID,
				Username:    sender.Handle,
				DisplayName: sender.Name,
			},
			Text:      message.Content,
			Timestamp: fmt.Sprintf("%d", message.CreatedAt.UnixMilli()),
			Mentions:  mentionsForBot(message.Mentions, botID),
		}
		for _, ch := range subs {
			select {
			case ch <- evt:
			default:
			}
		}
	}
}

func (e PicoClawEvent) MarshalJSONLine() ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func shouldNotifyBot(room Room, message Message, botID string) bool {
	if message.SenderID == botID {
		return false
	}
	if !containsUserIDInRoom(room, botID) {
		return false
	}
	if chatTypeForRoom(room) == "direct" {
		return true
	}
	for _, mentionedID := range message.Mentions {
		if mentionedID == botID {
			return true
		}
	}
	return false
}

func mentionsForBot(mentions []string, botID string) []string {
	if len(mentions) == 0 {
		return nil
	}
	result := make([]string, 0, len(mentions))
	for _, mentionedID := range mentions {
		if mentionedID == botID {
			result = append(result, mentionedID)
		}
	}
	return result
}

func chatTypeForRoom(room Room) string {
	if len(room.Participants) <= 2 {
		return "direct"
	}
	return "group"
}

func chatTypeForConversation(conv Conversation) string {
	return chatTypeForRoom(conv)
}
