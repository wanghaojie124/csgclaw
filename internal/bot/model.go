package bot

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

type Role string

const (
	RoleManager Role = "manager"
	RoleWorker  Role = "worker"
)

type Channel string

const (
	ChannelCSGClaw Channel = "csgclaw"
	ChannelFeishu  Channel = "feishu"
)

type Bot struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Channel   string    `json:"channel"`
	AgentID   string    `json:"agent_id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateRequest struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Role        string `json:"role"`
	Channel     string `json:"channel,omitempty"`
	ModelID     string `json:"model_id,omitempty"`
}

func NormalizeCreateRequest(req CreateRequest) (CreateRequest, error) {
	req.ID = strings.TrimSpace(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.ModelID = strings.TrimSpace(req.ModelID)
	if req.Name == "" {
		return CreateRequest{}, fmt.Errorf("name is required")
	}

	role, err := NormalizeRole(req.Role)
	if err != nil {
		return CreateRequest{}, err
	}
	channel, err := NormalizeChannel(req.Channel)
	if err != nil {
		return CreateRequest{}, err
	}
	req.Role = string(role)
	req.Channel = string(channel)
	return req, nil
}

func ValidateCreateRequest(req CreateRequest) error {
	_, err := NormalizeCreateRequest(req)
	return err
}

func NormalizeBot(b Bot) (Bot, error) {
	b.ID = strings.TrimSpace(b.ID)
	b.Name = strings.TrimSpace(b.Name)
	b.AgentID = strings.TrimSpace(b.AgentID)
	b.UserID = strings.TrimSpace(b.UserID)
	if b.ID == "" {
		return Bot{}, fmt.Errorf("id is required")
	}
	if b.Name == "" {
		return Bot{}, fmt.Errorf("name is required")
	}

	role, err := NormalizeRole(b.Role)
	if err != nil {
		return Bot{}, err
	}
	channel, err := NormalizeChannel(b.Channel)
	if err != nil {
		return Bot{}, err
	}
	b.Role = string(role)
	b.Channel = string(channel)
	return b, nil
}

func ValidateBot(b Bot) error {
	_, err := NormalizeBot(b)
	return err
}

func NormalizeRole(role string) (Role, error) {
	switch Role(strings.ToLower(strings.TrimSpace(role))) {
	case RoleManager:
		return RoleManager, nil
	case RoleWorker:
		return RoleWorker, nil
	default:
		return "", fmt.Errorf("role must be one of %q or %q", RoleManager, RoleWorker)
	}
}

func NormalizeChannel(channel string) (Channel, error) {
	switch Channel(strings.ToLower(strings.TrimSpace(channel))) {
	case "", ChannelCSGClaw:
		return ChannelCSGClaw, nil
	case ChannelFeishu:
		return ChannelFeishu, nil
	default:
		return "", fmt.Errorf("channel must be one of %q or %q", ChannelCSGClaw, ChannelFeishu)
	}
}

func sortedBotsFromMap(items map[string]Bot) []Bot {
	bots := make([]Bot, 0, len(items))
	for _, b := range items {
		bots = append(bots, b)
	}
	slices.SortFunc(bots, func(a, b Bot) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			switch {
			case a.ID < b.ID:
				return -1
			case a.ID > b.ID:
				return 1
			default:
				return 0
			}
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return bots
}
