package channel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/im"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

type FeishuCreateUserRequest struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	Handle string `json:"handle,omitempty"`
	Role   string `json:"role,omitempty"`
	Avatar string `json:"avatar,omitempty"`
}

type FeishuAppConfig struct {
	AppID       string
	AppSecret   string
	AdminOpenID string
}

type FeishuCreateChatRequest struct {
	Title       string
	Description string
	CreatorID   string
	MemberIDs   []string
}

type FeishuCreateChatResponse struct {
	ChatID      string
	Name        string
	Description string
}

type FeishuCreateChatFunc func(context.Context, FeishuAppConfig, FeishuCreateChatRequest) (FeishuCreateChatResponse, error)

type FeishuService struct {
	mu         sync.RWMutex
	users      map[string]im.User
	byHandle   map[string]string
	rooms      map[string]*im.Room
	apps       map[string]FeishuAppConfig
	createChat FeishuCreateChatFunc
}

func NewFeishuService(apps ...map[string]FeishuAppConfig) *FeishuService {
	configuredApps := make(map[string]FeishuAppConfig)
	if len(apps) > 0 {
		for name, app := range apps[0] {
			configuredApps[name] = app
		}
	}
	return &FeishuService{
		users:      make(map[string]im.User),
		byHandle:   make(map[string]string),
		rooms:      make(map[string]*im.Room),
		apps:       configuredApps,
		createChat: defaultFeishuCreateChat,
	}
}

func NewFeishuServiceWithCreateChat(apps map[string]FeishuAppConfig, createChat FeishuCreateChatFunc) *FeishuService {
	svc := NewFeishuService(apps)
	if createChat != nil {
		svc.createChat = createChat
	}
	return svc
}

func (s *FeishuService) AppConfigs() map[string]FeishuAppConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	apps := make(map[string]FeishuAppConfig, len(s.apps))
	for name, app := range s.apps {
		apps[name] = app
	}
	return apps
}

func (s *FeishuService) CreateUser(req FeishuCreateUserRequest) (im.User, error) {
	// Mock implementation. Real Feishu support should call the external Feishu API.
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return im.User{}, fmt.Errorf("name is required")
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = fmt.Sprintf("fsu-%d", time.Now().UnixNano())
	}
	handle := strings.ToLower(strings.TrimSpace(req.Handle))
	if handle == "" {
		handle = deriveHandle(name, id)
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role == "" {
		role = "member"
	}
	avatar := strings.TrimSpace(req.Avatar)
	if avatar == "" {
		avatar = initials(name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[id]; ok {
		return im.User{}, fmt.Errorf("user already exists")
	}
	if existingID, ok := s.byHandle[handle]; ok && existingID != id {
		return im.User{}, fmt.Errorf("handle %q already exists", handle)
	}

	user := im.User{
		ID:        id,
		Name:      name,
		Handle:    handle,
		Role:      role,
		Avatar:    avatar,
		IsOnline:  true,
		AccentHex: accentHexForID(id),
	}
	s.users[id] = user
	s.byHandle[handle] = id
	return user, nil
}

func (s *FeishuService) ListUsers() []im.User {
	// Mock implementation. Real Feishu support should call the external Feishu API.
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]im.User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b im.User) int { return strings.Compare(a.Name, b.Name) })
	return users
}

func (s *FeishuService) CreateRoom(req im.CreateRoomRequest) (im.Room, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return im.Room{}, fmt.Errorf("title is required")
	}
	creatorID := strings.TrimSpace(req.CreatorID)
	if creatorID == "" {
		return im.Room{}, fmt.Errorf("creator_id is required")
	}

	app, err := s.appConfigForCreator(creatorID)
	if err != nil {
		return im.Room{}, err
	}
	adminOpenID := strings.TrimSpace(app.AdminOpenID)
	if adminOpenID == "" {
		return im.Room{}, fmt.Errorf("feishu admin_open_id is required")
	}
	participants := normalizeFeishuParticipants(creatorID, req.ParticipantIDs)
	memberIDs := participants[1:]
	description := strings.TrimSpace(req.Description)

	created, err := s.createChat(context.Background(), app, FeishuCreateChatRequest{
		Title:       title,
		Description: description,
		CreatorID:   adminOpenID, // TODO: use u-manager app_id?
		MemberIDs:   memberIDs,
	})
	if err != nil {
		return im.Room{}, err
	}
	chatID := strings.TrimSpace(created.ChatID)
	if chatID == "" {
		return im.Room{}, fmt.Errorf("create feishu chat: empty chat_id in response")
	}
	if responseName := strings.TrimSpace(created.Name); responseName != "" {
		title = responseName
	}
	if responseDescription := strings.TrimSpace(created.Description); responseDescription != "" {
		description = responseDescription
	}

	room := im.Room{
		ID:           chatID,
		Title:        title,
		Subtitle:     formatMembers(len(participants)),
		Description:  description,
		Participants: participants,
		Messages:     nil,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.rooms[room.ID] = &room
	return cloneRoom(room), nil
}

func defaultFeishuCreateChat(ctx context.Context, app FeishuAppConfig, req FeishuCreateChatRequest) (FeishuCreateChatResponse, error) {
	client := lark.NewClient(app.AppID, app.AppSecret)
	createReq := larkim.NewCreateChatReqBuilder().
		UserIdType("open_id"). // TODO: use app_id?
		SetBotManager(true).
		Uuid(feishuRequestUUID()).
		Body(larkim.NewCreateChatReqBodyBuilder().
			Name(req.Title).
			Description(req.Description).
			OwnerId(req.CreatorID).
			UserIdList(req.MemberIDs).
			BotIdList([]string{}).
			GroupMessageType("chat").
			ChatMode("group").
			ChatType("private").
			JoinMessageVisibility("all_members").
			LeaveMessageVisibility("all_members").
			MembershipApproval("no_approval_required").
			RestrictedModeSetting(larkim.NewRestrictedModeSettingBuilder().Build()).
			UrgentSetting("all_members").
			VideoConferenceSetting("all_members").
			EditPermission("all_members").
			HideMemberCountSetting("all_members").
			Build()).
		Build()

	resp, err := client.Im.V1.Chat.Create(ctx, createReq)
	if err != nil {
		return FeishuCreateChatResponse{}, fmt.Errorf("create feishu chat: %w", err)
	}
	if !resp.Success() {
		return FeishuCreateChatResponse{}, fmt.Errorf("create feishu chat: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
	}
	if resp.Data == nil {
		return FeishuCreateChatResponse{}, fmt.Errorf("create feishu chat: empty response data")
	}
	return FeishuCreateChatResponse{
		ChatID:      larkcore.StringValue(resp.Data.ChatId),
		Name:        larkcore.StringValue(resp.Data.Name),
		Description: larkcore.StringValue(resp.Data.Description),
	}, nil
}

func (s *FeishuService) ListRooms() []im.Room {
	// Mock implementation. Real Feishu support should call the external Feishu API.
	s.mu.RLock()
	defer s.mu.RUnlock()

	rooms := make([]im.Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, cloneRoom(*room))
	}
	slices.SortFunc(rooms, func(a, b im.Room) int { return strings.Compare(a.Title, b.Title) })
	return rooms
}

func (s *FeishuService) AddRoomMembers(req im.AddRoomMembersRequest) (im.Room, error) {
	// Mock implementation. Real Feishu support should call the external Feishu API.
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return im.Room{}, fmt.Errorf("room_id is required")
	}
	if len(req.UserIDs) == 0 {
		return im.Room{}, fmt.Errorf("user_ids is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return im.Room{}, fmt.Errorf("room not found")
	}
	existing := make(map[string]struct{}, len(room.Participants))
	for _, userID := range room.Participants {
		existing[userID] = struct{}{}
	}

	added := 0
	for _, userID := range req.UserIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, ok := s.users[userID]; !ok {
			return im.Room{}, fmt.Errorf("user not found: %s", userID)
		}
		if _, ok := existing[userID]; ok {
			continue
		}
		existing[userID] = struct{}{}
		room.Participants = append(room.Participants, userID)
		added++
	}
	if added == 0 {
		return im.Room{}, fmt.Errorf("no new users to invite")
	}
	room.Subtitle = formatMembers(len(room.Participants))
	return cloneRoom(*room), nil
}

func (s *FeishuService) ListRoomMembers(roomID string) ([]im.User, error) {
	// Mock implementation. Real Feishu support should call the external Feishu API.
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return nil, fmt.Errorf("room_id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return nil, fmt.Errorf("room not found")
	}
	users := make([]im.User, 0, len(room.Participants))
	for _, userID := range room.Participants {
		if user, ok := s.users[userID]; ok {
			users = append(users, user)
		}
	}
	slices.SortFunc(users, func(a, b im.User) int { return strings.Compare(a.Name, b.Name) })
	return users, nil
}

func (s *FeishuService) normalizeParticipantsLocked(creatorID string, participantIDs []string) ([]string, error) {
	seen := map[string]struct{}{creatorID: {}}
	participants := []string{creatorID}
	for _, userID := range participantIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, ok := s.users[userID]; !ok {
			return nil, fmt.Errorf("user not found: %s", userID)
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		participants = append(participants, userID)
	}
	return participants, nil
}

func (s *FeishuService) appConfigForCreator(creatorID string) (FeishuAppConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	app, ok := s.apps[creatorID]
	if !ok {
		return FeishuAppConfig{}, fmt.Errorf("feishu app is not configured for creator_id %q", creatorID)
	}
	if strings.TrimSpace(app.AppID) == "" {
		return FeishuAppConfig{}, fmt.Errorf("feishu app_id is required")
	}
	if strings.TrimSpace(app.AppSecret) == "" {
		return FeishuAppConfig{}, fmt.Errorf("feishu app_secret is required")
	}
	return FeishuAppConfig{
		AppID:       strings.TrimSpace(app.AppID),
		AppSecret:   strings.TrimSpace(app.AppSecret),
		AdminOpenID: strings.TrimSpace(app.AdminOpenID),
	}, nil
}

func normalizeFeishuParticipants(creatorID string, participantIDs []string) []string {
	seen := map[string]struct{}{creatorID: {}}
	participants := []string{creatorID}
	for _, userID := range participantIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		participants = append(participants, userID)
	}
	return participants
}

func feishuRequestUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("csgclaw-%d", time.Now().UnixNano())
	}
	return "csgclaw-" + hex.EncodeToString(b[:])
}

func normalizeFeishuUser(user im.User) im.User {
	user.ID = strings.TrimSpace(user.ID)
	user.Name = strings.TrimSpace(user.Name)
	user.Handle = strings.ToLower(strings.TrimSpace(user.Handle))
	user.Role = strings.ToLower(strings.TrimSpace(user.Role))
	if user.Handle == "" {
		user.Handle = deriveHandle(user.Name, user.ID)
	}
	if user.Role == "" {
		user.Role = "member"
	}
	if user.Avatar == "" {
		user.Avatar = initials(user.Name)
	}
	if user.AccentHex == "" {
		user.AccentHex = accentHexForID(user.ID)
	}
	return user
}

func cloneRoom(room im.Room) im.Room {
	room.Participants = append([]string(nil), room.Participants...)
	room.Messages = append([]im.Message(nil), room.Messages...)
	return room
}

func deriveHandle(name, fallback string) string {
	source := strings.ToLower(strings.TrimSpace(name))
	if source == "" {
		source = strings.ToLower(strings.TrimSpace(fallback))
	}
	var b strings.Builder
	for _, r := range source {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	handle := strings.Trim(b.String(), "-._")
	if handle == "" {
		return strings.ToLower(strings.TrimSpace(fallback))
	}
	return handle
}

func initials(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "FS"
	}
	parts := strings.Fields(name)
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		r := []rune(part)[0]
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		b.WriteRune(r)
		if b.Len() >= 2 {
			break
		}
	}
	if b.Len() == 0 {
		return "FS"
	}
	return b.String()
}

func accentHexForID(id string) string {
	palette := []string{"#0f766e", "#2563eb", "#7c3aed", "#dc2626", "#ca8a04", "#16a34a"}
	sum := 0
	for _, r := range id {
		sum += int(r)
	}
	return palette[sum%len(palette)]
}

func formatMembers(n int) string {
	if n == 1 {
		return "1 member"
	}
	return fmt.Sprintf("%d members", n)
}
