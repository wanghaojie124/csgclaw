package im

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

type User struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Handle    string `json:"handle"`
	Role      string `json:"role"`
	Avatar    string `json:"avatar"`
	IsOnline  bool   `json:"is_online"`
	LastSeen  string `json:"last_seen,omitempty"`
	AccentHex string `json:"accent_hex"`
}

type Message struct {
	ID        string    `json:"id"`
	SenderID  string    `json:"sender_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	Mentions  []string  `json:"mentions"`
}

type Conversation struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Subtitle     string    `json:"subtitle"`
	Description  string    `json:"description,omitempty"`
	Participants []string  `json:"participants"`
	Messages     []Message `json:"messages"`
}

type Bootstrap struct {
	CurrentUserID      string         `json:"current_user_id"`
	Users              []User         `json:"users"`
	Conversations      []Conversation `json:"conversations"`
	InviteDraftUserIDs []string       `json:"invite_draft_user_ids,omitempty"`
}

type CreateMessageRequest struct {
	ConversationID string `json:"conversation_id"`
	SenderID       string `json:"sender_id"`
	Content        string `json:"content"`
}

type DeliverMessageRequest struct {
	ChatID    string `json:"chat_id"`
	SenderID  string `json:"sender_id,omitempty"`
	Content   string `json:"text"`
	MessageID string `json:"message_id,omitempty"`
}

type CreateConversationRequest struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	CreatorID      string   `json:"creator_id"`
	ParticipantIDs []string `json:"participant_ids"`
	Locale         string   `json:"locale"`
}

type AddConversationMembersRequest struct {
	ConversationID string   `json:"conversation_id"`
	InviterID      string   `json:"inviter_id"`
	UserIDs        []string `json:"user_ids"`
	Locale         string   `json:"locale"`
}

type EnsureAgentUserRequest struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Handle string `json:"handle"`
	Role   string `json:"role"`
}

type EnsureWorkerUserRequest = EnsureAgentUserRequest

type AddAgentToConversationRequest struct {
	AgentID        string `json:"agent_id"`
	ConversationID string `json:"conversation_id,omitempty"`
	RoomID         string `json:"room_id,omitempty"`
	InviterID      string `json:"inviter_id"`
	Locale         string `json:"locale"`
}

type Service struct {
	mu            sync.RWMutex
	statePath     string
	currentUserID string
	users         map[string]User
	byHandle      map[string]string
	conversations map[string]*Conversation
}

var mentionPattern = regexp.MustCompile(`(^|[^\w])@([a-zA-Z0-9._-]+)`)

func NewService() *Service {
	return NewServiceFromBootstrap(DefaultBootstrap())
}

func NewServiceFromPath(path string) (*Service, error) {
	state, err := LoadBootstrap(path)
	if err != nil {
		return nil, err
	}
	svc := NewServiceFromBootstrap(state)
	svc.statePath = path
	return svc, nil
}

func NewServiceFromBootstrap(state Bootstrap) *Service {
	state = normalizeBootstrap(state)

	users := state.Users
	conversations := state.Conversations
	svc := &Service{
		currentUserID: state.CurrentUserID,
		users:         make(map[string]User, len(users)),
		byHandle:      make(map[string]string, len(users)),
		conversations: make(map[string]*Conversation, len(conversations)),
	}
	for _, user := range users {
		svc.users[user.ID] = user
		svc.byHandle[strings.ToLower(user.Handle)] = user.ID
	}
	for i := range conversations {
		conv := conversations[i]
		svc.conversations[conv.ID] = &conv
	}
	return svc
}

func DefaultBootstrap() Bootstrap {
	return Bootstrap{
		CurrentUserID: "u-admin",
		Users:         nil,
		Conversations: nil,
	}
}

func LoadBootstrap(path string) (Bootstrap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultBootstrap(), nil
		}
		return Bootstrap{}, fmt.Errorf("read im bootstrap: %w", err)
	}

	var state Bootstrap
	if err := json.Unmarshal(data, &state); err != nil {
		return Bootstrap{}, fmt.Errorf("decode im bootstrap: %w", err)
	}
	return normalizeBootstrap(state), nil
}

func SaveBootstrap(path string, state Bootstrap) error {
	state = normalizeBootstrap(state)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create im state dir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode im bootstrap: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write im bootstrap: %w", err)
	}
	return nil
}

func EnsureBootstrapState(path string) error {
	state, err := LoadBootstrap(path)
	if err != nil {
		return err
	}
	state = normalizeBootstrap(state)
	state.Conversations = ensureAdminManagerRoom(state.Conversations)
	state.InviteDraftUserIDs = nil
	return SaveBootstrap(path, state)
}

func normalizeBootstrap(state Bootstrap) Bootstrap {
	if state.CurrentUserID == "" {
		state.CurrentUserID = DefaultBootstrap().CurrentUserID
	}
	state.Users = ensureUsers(state.Users)
	state.Conversations = cloneConversations(state.Conversations)
	if !containsUserID(state.Users, state.CurrentUserID) {
		state.CurrentUserID = defaultCurrentUserID(state.Users)
	}
	return state
}

func ensureUsers(users []User) []User {
	result := append([]User(nil), users...)
	if !hasUserHandle(result, "admin") {
		result = append(result, User{
			ID:        "u-admin",
			Name:      "Admin",
			Handle:    "admin",
			Role:      "Admin",
			Avatar:    "AD",
			IsOnline:  true,
			AccentHex: "#dc2626",
		})
	}
	if !hasUserHandle(result, "manager") {
		result = append(result, User{
			ID:        "u-manager",
			Name:      "Manager",
			Handle:    "manager",
			Role:      "Manager",
			Avatar:    "MG",
			IsOnline:  true,
			AccentHex: "#0f766e",
		})
	}
	return result
}

func hasUserHandle(users []User, handle string) bool {
	for _, user := range users {
		if strings.EqualFold(strings.TrimSpace(user.Handle), handle) {
			return true
		}
	}
	return false
}

func containsUserID(users []User, userID string) bool {
	for _, user := range users {
		if user.ID == userID {
			return true
		}
	}
	return false
}

func defaultCurrentUserID(users []User) string {
	for _, preferred := range []string{"u-admin", "u-manager"} {
		if containsUserID(users, preferred) {
			return preferred
		}
	}
	if len(users) > 0 {
		return users[0].ID
	}
	return ""
}

func cloneConversations(conversations []Conversation) []Conversation {
	cloned := make([]Conversation, 0, len(conversations))
	for _, conv := range conversations {
		cloned = append(cloned, cloneConversation(conv))
	}
	return cloned
}

func ensureAdminManagerRoom(conversations []Conversation) []Conversation {
	for _, conv := range conversations {
		if containsUserIDInConversation(conv, "u-admin") && containsUserIDInConversation(conv, "u-manager") {
			return conversations
		}
	}

	now := time.Now().UTC()
	room := Conversation{
		ID:           fmt.Sprintf("room-%d", now.UnixNano()),
		Title:        "Admin & Manager",
		Subtitle:     formatConversationSubtitle(2),
		Description:  "Bootstrap room for Admin and Manager.",
		Participants: []string{"u-admin", "u-manager"},
		Messages: []Message{
			{
				ID:        fmt.Sprintf("msg-%d", now.UnixNano()+1),
				SenderID:  "u-manager",
				Content:   "Bootstrap room created for Admin and Manager.",
				CreatedAt: now,
			},
		},
	}
	return append(cloneConversations(conversations), room)
}

func containsUserIDInConversation(conv Conversation, userID string) bool {
	for _, participantID := range conv.Participants {
		if participantID == userID {
			return true
		}
	}
	return false
}

func (s *Service) Bootstrap() Bootstrap {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b User) int { return strings.Compare(a.Name, b.Name) })

	conversations := make([]Conversation, 0, len(s.conversations))
	for _, conv := range s.conversations {
		conversations = append(conversations, s.presentConversationLocked(*conv))
	}
	slices.SortFunc(conversations, func(a, b Conversation) int {
		return latestMessageAt(b).Compare(latestMessageAt(a))
	})

	return Bootstrap{
		CurrentUserID: s.currentUserID,
		Users:         users,
		Conversations: conversations,
	}
}

func (s *Service) ListRooms() []Conversation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conversations := make([]Conversation, 0, len(s.conversations))
	for _, conv := range s.conversations {
		conversations = append(conversations, s.presentConversationLocked(*conv))
	}
	slices.SortFunc(conversations, func(a, b Conversation) int {
		return latestMessageAt(b).Compare(latestMessageAt(a))
	})
	return conversations
}

func (s *Service) ListUsers() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b User) int { return strings.Compare(a.Name, b.Name) })
	return users
}

func (s *Service) ListMessages(conversationID string) ([]Message, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, fmt.Errorf("conversation_id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	conv, ok := s.conversations[conversationID]
	if !ok {
		return nil, fmt.Errorf("conversation not found")
	}
	return append([]Message(nil), conv.Messages...), nil
}

func (s *Service) EnsureAgentUser(req EnsureAgentUserRequest) (User, *Conversation, error) {
	id := strings.TrimSpace(req.ID)
	name := strings.TrimSpace(req.Name)
	handle := strings.TrimSpace(req.Handle)
	role := strings.TrimSpace(req.Role)
	switch {
	case id == "":
		return User{}, nil, fmt.Errorf("id is required")
	case name == "":
		return User{}, nil, fmt.Errorf("name is required")
	case handle == "":
		return User{}, nil, fmt.Errorf("handle is required")
	}
	if role == "" {
		role = "Worker"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.users[id]; ok {
		room, _ := s.ensureAdminAgentRoomLocked(id, existing.Name)
		if err := s.saveLocked(); err != nil {
			return User{}, nil, err
		}
		return existing, room, nil
	}
	if existingID, ok := s.byHandle[strings.ToLower(handle)]; ok && existingID != id {
		return User{}, nil, fmt.Errorf("handle %q already exists", handle)
	}

	user := User{
		ID:        id,
		Name:      name,
		Handle:    handle,
		Role:      role,
		Avatar:    initials(name),
		IsOnline:  true,
		AccentHex: accentHexForID(id),
	}
	s.users[id] = user
	s.byHandle[strings.ToLower(handle)] = id
	room, roomCreated := s.ensureAdminAgentRoomLocked(id, name)
	if err := s.saveLocked(); err != nil {
		delete(s.users, id)
		delete(s.byHandle, strings.ToLower(handle))
		if roomCreated && room != nil {
			delete(s.conversations, room.ID)
		}
		return User{}, nil, err
	}
	return user, room, nil
}

func (s *Service) EnsureWorkerUser(req EnsureWorkerUserRequest) (User, *Conversation, error) {
	return s.EnsureAgentUser(req)
}

func (s *Service) CreateMessage(req CreateMessageRequest) (Message, error) {
	content := strings.TrimSpace(req.Content)
	if req.ConversationID == "" {
		return Message{}, fmt.Errorf("conversation_id is required")
	}
	if req.SenderID == "" {
		return Message{}, fmt.Errorf("sender_id is required")
	}
	if content == "" {
		return Message{}, fmt.Errorf("content is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[req.SenderID]; !ok {
		return Message{}, fmt.Errorf("sender not found")
	}

	conv, ok := s.conversations[req.ConversationID]
	if !ok {
		return Message{}, fmt.Errorf("conversation not found")
	}

	message := s.newMessage("", req.SenderID, content)
	conv.Messages = append(conv.Messages, message)
	if err := s.saveLocked(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (s *Service) DeliverMessage(req DeliverMessageRequest) (Message, error) {
	chatID := strings.TrimSpace(req.ChatID)
	senderID := strings.TrimSpace(req.SenderID)
	content := strings.TrimSpace(req.Content)
	if chatID == "" {
		return Message{}, fmt.Errorf("chat_id is required")
	}
	if content == "" {
		return Message{}, fmt.Errorf("text is required")
	}
	if senderID == "" {
		senderID = s.currentUserID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[senderID]; !ok {
		return Message{}, fmt.Errorf("sender not found")
	}
	conv, ok := s.conversations[chatID]
	if !ok {
		return Message{}, fmt.Errorf("conversation not found")
	}

	message := s.newMessage(req.MessageID, senderID, content)
	conv.Messages = append(conv.Messages, message)
	if err := s.saveLocked(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (s *Service) CreateConversation(req CreateConversationRequest) (Conversation, error) {
	title := strings.TrimSpace(req.Title)
	description := strings.TrimSpace(req.Description)
	if title == "" {
		return Conversation{}, fmt.Errorf("title is required")
	}
	if req.CreatorID == "" {
		return Conversation{}, fmt.Errorf("creator_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[req.CreatorID]; !ok {
		return Conversation{}, fmt.Errorf("creator not found")
	}

	participants, err := s.normalizeParticipants(req.CreatorID, req.ParticipantIDs)
	if err != nil {
		return Conversation{}, err
	}

	conv := Conversation{
		ID:           fmt.Sprintf("room-%d", time.Now().UnixNano()),
		Title:        title,
		Subtitle:     formatConversationSubtitle(len(participants)),
		Description:  description,
		Participants: participants,
		Messages: []Message{
			{
				ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
				SenderID:  req.CreatorID,
				Content:   s.localizeSystemText(req.Locale, "conversation_created", title, nil),
				CreatedAt: time.Now().UTC(),
			},
		},
	}

	s.conversations[conv.ID] = &conv
	if err := s.saveLocked(); err != nil {
		return Conversation{}, err
	}
	return s.presentConversationLocked(conv), nil
}

func (s *Service) AddConversationMembers(req AddConversationMembersRequest) (Conversation, error) {
	if req.ConversationID == "" {
		return Conversation{}, fmt.Errorf("conversation_id is required")
	}
	if req.InviterID == "" {
		return Conversation{}, fmt.Errorf("inviter_id is required")
	}
	if len(req.UserIDs) == 0 {
		return Conversation{}, fmt.Errorf("user_ids is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.conversations[req.ConversationID]
	if !ok {
		return Conversation{}, fmt.Errorf("conversation not found")
	}
	if _, ok := s.users[req.InviterID]; !ok {
		return Conversation{}, fmt.Errorf("inviter not found")
	}
	if !slices.Contains(conv.Participants, req.InviterID) {
		return Conversation{}, fmt.Errorf("inviter is not a conversation member")
	}

	existing := make(map[string]struct{}, len(conv.Participants))
	for _, id := range conv.Participants {
		existing[id] = struct{}{}
	}

	addedIDs := make([]string, 0, len(req.UserIDs))
	for _, userID := range req.UserIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, ok := s.users[userID]; !ok {
			return Conversation{}, fmt.Errorf("user not found: %s", userID)
		}
		if _, ok := existing[userID]; ok {
			continue
		}
		existing[userID] = struct{}{}
		conv.Participants = append(conv.Participants, userID)
		addedIDs = append(addedIDs, userID)
	}
	if len(addedIDs) == 0 {
		return Conversation{}, fmt.Errorf("no new users to invite")
	}

	conv.Subtitle = formatConversationSubtitle(len(conv.Participants))
	conv.Messages = append(conv.Messages, Message{
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		SenderID:  req.InviterID,
		Content:   s.localizeSystemText(req.Locale, "members_added", "", addedIDs),
		CreatedAt: time.Now().UTC(),
		Mentions:  append([]string(nil), addedIDs...),
	})
	if err := s.saveLocked(); err != nil {
		return Conversation{}, err
	}

	return s.presentConversationLocked(*conv), nil
}

func (s *Service) AddAgentToConversation(req AddAgentToConversationRequest) (Conversation, error) {
	conversationID := strings.TrimSpace(req.ConversationID)
	roomID := strings.TrimSpace(req.RoomID)
	switch {
	case conversationID == "" && roomID == "":
		return Conversation{}, fmt.Errorf("conversation_id or room_id is required")
	case conversationID != "" && roomID != "":
		return Conversation{}, fmt.Errorf("conversation_id and room_id cannot both be set")
	}
	if conversationID == "" {
		conversationID = roomID
	}

	return s.AddConversationMembers(AddConversationMembersRequest{
		ConversationID: conversationID,
		InviterID:      strings.TrimSpace(req.InviterID),
		UserIDs:        []string{strings.TrimSpace(req.AgentID)},
		Locale:         req.Locale,
	})
}

func (s *Service) Conversation(conversationID string) (Conversation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conv, ok := s.conversations[conversationID]
	if !ok {
		return Conversation{}, false
	}
	return s.presentConversationLocked(*conv), true
}

func (s *Service) User(userID string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	return user, ok
}

func (s *Service) extractMentions(content string) []string {
	matches := mentionPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	mentions := make([]string, 0, len(matches))
	for _, match := range matches {
		handle := strings.ToLower(match[2])
		if userID, ok := s.byHandle[handle]; ok {
			if _, exists := seen[userID]; exists {
				continue
			}
			seen[userID] = struct{}{}
			mentions = append(mentions, userID)
		}
	}
	return mentions
}

func (s *Service) normalizeParticipants(creatorID string, participantIDs []string) ([]string, error) {
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

func (s *Service) formatUserList(userIDs []string) string {
	names := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		if user, ok := s.users[userID]; ok {
			names = append(names, "@"+user.Handle)
		}
	}
	return strings.Join(names, "、")
}

func (s *Service) localizeSystemText(locale, key, title string, userIDs []string) string {
	switch normalizeLocale(locale) {
	case "en":
		switch key {
		case "conversation_created":
			return fmt.Sprintf("Created conversation \"%s\". Welcome everyone.", title)
		case "members_added":
			return fmt.Sprintf("Added %s to the conversation.", strings.Join(s.formatHandles(userIDs), ", "))
		}
	default:
		switch key {
		case "conversation_created":
			return fmt.Sprintf("已创建会话“%s”，欢迎大家加入。", title)
		case "members_added":
			return fmt.Sprintf("已将 %s 添加到会话。", s.formatUserList(userIDs))
		}
	}
	return ""
}

func (s *Service) formatHandles(userIDs []string) []string {
	handles := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		if user, ok := s.users[userID]; ok {
			handles = append(handles, "@"+user.Handle)
		}
	}
	return handles
}

func normalizeLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if strings.HasPrefix(locale, "en") {
		return "en"
	}
	return "zh"
}

func formatConversationSubtitle(count int) string {
	return fmt.Sprintf("%d members", count)
}

func (s *Service) presentConversationLocked(conv Conversation) Conversation {
	cloned := cloneConversation(conv)
	if len(cloned.Participants) != 2 {
		return cloned
	}

	otherID := cloned.Participants[0]
	if otherID == s.currentUserID {
		otherID = cloned.Participants[1]
	}
	if user, ok := s.users[otherID]; ok && strings.TrimSpace(user.Name) != "" {
		cloned.Title = user.Name
	}
	return cloned
}

func cloneConversation(conv Conversation) Conversation {
	cloned := conv
	cloned.Participants = append([]string(nil), conv.Participants...)
	cloned.Messages = append([]Message(nil), conv.Messages...)
	return cloned
}

func (s *Service) newMessage(messageID, senderID, content string) Message {
	if strings.TrimSpace(messageID) == "" {
		messageID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}
	return Message{
		ID:        messageID,
		SenderID:  senderID,
		Content:   content,
		CreatedAt: time.Now().UTC(),
		Mentions:  s.extractMentions(content),
	}
}

func (s *Service) saveLocked() error {
	if s.statePath == "" {
		return nil
	}
	return SaveBootstrap(s.statePath, s.bootstrapLocked())
}

func (s *Service) bootstrapLocked() Bootstrap {
	users := make([]User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b User) int { return strings.Compare(a.Name, b.Name) })

	conversations := make([]Conversation, 0, len(s.conversations))
	for _, conv := range s.conversations {
		conversations = append(conversations, cloneConversation(*conv))
	}
	slices.SortFunc(conversations, func(a, b Conversation) int {
		return latestMessageAt(b).Compare(latestMessageAt(a))
	})

	return Bootstrap{
		CurrentUserID: s.currentUserID,
		Users:         users,
		Conversations: conversations,
	}
}

func (s *Service) ensureAdminAgentRoomLocked(agentID, agentName string) (*Conversation, bool) {
	for _, conv := range s.conversations {
		if len(conv.Participants) != 2 {
			continue
		}
		if containsUserIDInConversation(*conv, "u-admin") && containsUserIDInConversation(*conv, agentID) {
			presented := s.presentConversationLocked(*conv)
			return &presented, false
		}
	}

	now := time.Now().UTC()
	room := Conversation{
		ID:           fmt.Sprintf("room-%d", now.UnixNano()),
		Title:        agentName,
		Subtitle:     formatConversationSubtitle(2),
		Description:  fmt.Sprintf("Bootstrap room for Admin and %s.", agentName),
		Participants: []string{"u-admin", agentID},
		Messages: []Message{
			{
				ID:        fmt.Sprintf("msg-%d", now.UnixNano()+1),
				SenderID:  agentID,
				Content:   fmt.Sprintf("Bootstrap room created for Admin and %s.", agentName),
				CreatedAt: now,
			},
		},
	}
	s.conversations[room.ID] = &room
	presented := s.presentConversationLocked(room)
	return &presented, true
}

func initials(name string) string {
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) == 0 {
		return "WK"
	}
	var b strings.Builder
	for _, field := range fields {
		for _, r := range field {
			if r == '-' || r == '_' {
				continue
			}
			b.WriteRune(r)
			if b.Len() >= 2 {
				return strings.ToUpper(b.String())
			}
			break
		}
	}
	if b.Len() == 0 {
		return "WK"
	}
	return strings.ToUpper(b.String())
}

func accentHexForID(id string) string {
	palette := []string{
		"#2563eb",
		"#7c3aed",
		"#0891b2",
		"#059669",
		"#ea580c",
		"#db2777",
	}
	sum := 0
	for _, r := range id {
		sum += int(r)
	}
	return palette[sum%len(palette)]
}

func latestMessageAt(conv Conversation) time.Time {
	if len(conv.Messages) == 0 {
		return time.Time{}
	}
	return conv.Messages[len(conv.Messages)-1].CreatedAt
}

func seedTime(hour, minute int) time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
}
