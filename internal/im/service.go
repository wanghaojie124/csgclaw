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

type Room struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Subtitle     string    `json:"subtitle"`
	Description  string    `json:"description,omitempty"`
	Participants []string  `json:"participants"`
	Messages     []Message `json:"messages"`
}

type Conversation = Room

type Bootstrap struct {
	CurrentUserID      string   `json:"current_user_id"`
	Users              []User   `json:"users"`
	Rooms              []Room   `json:"rooms,omitempty"`
	Conversations      []Room   `json:"conversations,omitempty"`
	InviteDraftUserIDs []string `json:"invite_draft_user_ids,omitempty"`
}

type CreateMessageRequest struct {
	RoomID   string `json:"room_id,omitempty"`
	SenderID string `json:"sender_id"`
	Content  string `json:"content"`
}

type DeliverMessageRequest struct {
	ChatID    string `json:"chat_id"`
	SenderID  string `json:"sender_id,omitempty"`
	Content   string `json:"text"`
	MessageID string `json:"message_id,omitempty"`
}

type CreateRoomRequest struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	CreatorID      string   `json:"creator_id"`
	ParticipantIDs []string `json:"participant_ids"`
	Locale         string   `json:"locale"`
}

type CreateConversationRequest = CreateRoomRequest

type AddRoomMembersRequest struct {
	RoomID    string   `json:"room_id,omitempty"`
	InviterID string   `json:"inviter_id"`
	UserIDs   []string `json:"user_ids"`
	Locale    string   `json:"locale"`
}

type AddConversationMembersRequest = AddRoomMembersRequest

type EnsureAgentUserRequest struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Handle string `json:"handle"`
	Role   string `json:"role"`
}

type EnsureWorkerUserRequest = EnsureAgentUserRequest

type AddAgentToConversationRequest struct {
	AgentID   string `json:"agent_id"`
	RoomID    string `json:"room_id,omitempty"`
	InviterID string `json:"inviter_id"`
	Locale    string `json:"locale"`
}

type Service struct {
	mu            sync.RWMutex
	statePath     string
	currentUserID string
	users         map[string]User
	byHandle      map[string]string
	rooms         map[string]*Room
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
	rooms := state.Rooms
	svc := &Service{
		currentUserID: state.CurrentUserID,
		users:         make(map[string]User, len(users)),
		byHandle:      make(map[string]string, len(users)),
		rooms:         make(map[string]*Room, len(rooms)),
	}
	for _, user := range users {
		svc.users[user.ID] = user
		svc.byHandle[strings.ToLower(user.Handle)] = user.ID
	}
	for i := range rooms {
		room := rooms[i]
		svc.rooms[room.ID] = &room
	}
	return svc
}

func DefaultBootstrap() Bootstrap {
	return Bootstrap{
		CurrentUserID: "u-admin",
		Users:         nil,
		Rooms:         nil,
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
	state.Rooms = ensureAdminManagerRoom(state.Rooms)
	state.InviteDraftUserIDs = nil
	return SaveBootstrap(path, state)
}

func normalizeBootstrap(state Bootstrap) Bootstrap {
	if state.CurrentUserID == "" {
		state.CurrentUserID = DefaultBootstrap().CurrentUserID
	}
	state.Users = ensureUsers(state.Users)
	state.Rooms = normalizeRooms(state.Rooms, state.Conversations)
	if !containsUserID(state.Users, state.CurrentUserID) {
		state.CurrentUserID = defaultCurrentUserID(state.Users)
	}
	return state
}

func normalizeRooms(rooms, conversations []Room) []Room {
	switch {
	case len(rooms) > 0:
		return cloneRooms(rooms)
	case len(conversations) > 0:
		return cloneRooms(conversations)
	default:
		return nil
	}
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

func cloneRooms(rooms []Room) []Room {
	cloned := make([]Room, 0, len(rooms))
	for _, room := range rooms {
		cloned = append(cloned, cloneRoom(room))
	}
	return cloned
}

func ensureAdminManagerRoom(rooms []Room) []Room {
	for _, room := range rooms {
		if containsUserIDInRoom(room, "u-admin") && containsUserIDInRoom(room, "u-manager") {
			return rooms
		}
	}

	now := time.Now().UTC()
	room := Room{
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
	return append(cloneRooms(rooms), room)
}

func containsUserIDInRoom(room Room, userID string) bool {
	for _, participantID := range room.Participants {
		if participantID == userID {
			return true
		}
	}
	return false
}

func containsUserIDInConversation(conv Conversation, userID string) bool {
	return containsUserIDInRoom(conv, userID)
}

func (s *Service) Bootstrap() Bootstrap {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b User) int { return strings.Compare(a.Name, b.Name) })

	rooms := make([]Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, s.presentRoomLocked(*room))
	}
	slices.SortFunc(rooms, func(a, b Room) int {
		return latestRoomMessageAt(b).Compare(latestRoomMessageAt(a))
	})

	return Bootstrap{
		CurrentUserID: s.currentUserID,
		Users:         users,
		Rooms:         rooms,
	}
}

func (s *Service) ListRooms() []Room {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rooms := make([]Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, s.presentRoomLocked(*room))
	}
	slices.SortFunc(rooms, func(a, b Room) int {
		return latestRoomMessageAt(b).Compare(latestRoomMessageAt(a))
	})
	return rooms
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

func (s *Service) ListMessages(roomID string) ([]Message, error) {
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
	return append([]Message(nil), room.Messages...), nil
}

func (s *Service) DeleteRoom(roomID string) error {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return fmt.Errorf("room_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.rooms[roomID]; !ok {
		return fmt.Errorf("room not found")
	}
	delete(s.rooms, roomID)
	return s.saveLocked()
}

func (s *Service) DeleteConversation(conversationID string) error {
	return s.DeleteRoom(conversationID)
}

func (s *Service) KickUser(userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[userID]
	if !ok {
		return fmt.Errorf("user not found")
	}
	if userID == s.currentUserID {
		return fmt.Errorf("cannot kick current user")
	}

	delete(s.users, userID)
	delete(s.byHandle, strings.ToLower(user.Handle))

	for id, room := range s.rooms {
		participants := make([]string, 0, len(room.Participants))
		for _, participantID := range room.Participants {
			if participantID != userID {
				participants = append(participants, participantID)
			}
		}

		messages := make([]Message, 0, len(room.Messages))
		for _, message := range room.Messages {
			if message.SenderID != userID {
				messages = append(messages, message)
			}
		}

		if len(participants) < 2 {
			delete(s.rooms, id)
			continue
		}

		room.Participants = participants
		room.Messages = messages
		room.Subtitle = formatRoomSubtitle(len(participants))
	}

	return s.saveLocked()
}

func (s *Service) EnsureAgentUser(req EnsureAgentUserRequest) (User, *Room, error) {
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
			delete(s.rooms, room.ID)
		}
		return User{}, nil, err
	}
	return user, room, nil
}

func (s *Service) EnsureWorkerUser(req EnsureWorkerUserRequest) (User, *Room, error) {
	return s.EnsureAgentUser(req)
}

func (s *Service) CreateMessage(req CreateMessageRequest) (Message, error) {
	content := strings.TrimSpace(req.Content)
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return Message{}, fmt.Errorf("room_id is required")
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

	room, ok := s.rooms[roomID]
	if !ok {
		return Message{}, fmt.Errorf("room not found")
	}

	message := s.newMessage("", req.SenderID, content)
	room.Messages = append(room.Messages, message)
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
	room, ok := s.rooms[chatID]
	if !ok {
		return Message{}, fmt.Errorf("room not found")
	}

	message := s.newMessage(req.MessageID, senderID, content)
	room.Messages = append(room.Messages, message)
	if err := s.saveLocked(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (s *Service) CreateRoom(req CreateRoomRequest) (Room, error) {
	title := strings.TrimSpace(req.Title)
	description := strings.TrimSpace(req.Description)
	if title == "" {
		return Room{}, fmt.Errorf("title is required")
	}
	if req.CreatorID == "" {
		return Room{}, fmt.Errorf("creator_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[req.CreatorID]; !ok {
		return Room{}, fmt.Errorf("creator not found")
	}

	participants, err := s.normalizeParticipants(req.CreatorID, req.ParticipantIDs)
	if err != nil {
		return Room{}, err
	}

	room := Room{
		ID:           fmt.Sprintf("room-%d", time.Now().UnixNano()),
		Title:        title,
		Subtitle:     formatRoomSubtitle(len(participants)),
		Description:  description,
		Participants: participants,
		Messages: []Message{
			{
				ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
				SenderID:  req.CreatorID,
				Content:   s.localizeSystemText(req.Locale, "room_created", title, nil),
				CreatedAt: time.Now().UTC(),
			},
		},
	}

	s.rooms[room.ID] = &room
	if err := s.saveLocked(); err != nil {
		return Room{}, err
	}
	return s.presentRoomLocked(room), nil
}

func (s *Service) CreateConversation(req CreateConversationRequest) (Conversation, error) {
	return s.CreateRoom(req)
}

func (s *Service) AddRoomMembers(req AddRoomMembersRequest) (Room, error) {
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return Room{}, fmt.Errorf("room_id is required")
	}
	if req.InviterID == "" {
		return Room{}, fmt.Errorf("inviter_id is required")
	}
	if len(req.UserIDs) == 0 {
		return Room{}, fmt.Errorf("user_ids is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return Room{}, fmt.Errorf("room not found")
	}
	if _, ok := s.users[req.InviterID]; !ok {
		return Room{}, fmt.Errorf("inviter not found")
	}
	if !slices.Contains(room.Participants, req.InviterID) {
		return Room{}, fmt.Errorf("inviter is not a room member")
	}

	existing := make(map[string]struct{}, len(room.Participants))
	for _, id := range room.Participants {
		existing[id] = struct{}{}
	}

	addedIDs := make([]string, 0, len(req.UserIDs))
	for _, userID := range req.UserIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, ok := s.users[userID]; !ok {
			return Room{}, fmt.Errorf("user not found: %s", userID)
		}
		if _, ok := existing[userID]; ok {
			continue
		}
		existing[userID] = struct{}{}
		room.Participants = append(room.Participants, userID)
		addedIDs = append(addedIDs, userID)
	}
	if len(addedIDs) == 0 {
		return Room{}, fmt.Errorf("no new users to invite")
	}

	room.Subtitle = formatRoomSubtitle(len(room.Participants))
	room.Messages = append(room.Messages, Message{
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		SenderID:  req.InviterID,
		Content:   s.localizeSystemText(req.Locale, "room_members_added", "", addedIDs),
		CreatedAt: time.Now().UTC(),
		Mentions:  append([]string(nil), addedIDs...),
	})
	if err := s.saveLocked(); err != nil {
		return Room{}, err
	}

	return s.presentRoomLocked(*room), nil
}

func (s *Service) AddConversationMembers(req AddConversationMembersRequest) (Conversation, error) {
	return s.AddRoomMembers(req)
}

func (s *Service) AddAgentToRoom(req AddAgentToConversationRequest) (Room, error) {
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return Room{}, fmt.Errorf("room_id is required")
	}

	return s.AddRoomMembers(AddRoomMembersRequest{
		RoomID:    roomID,
		InviterID: strings.TrimSpace(req.InviterID),
		UserIDs:   []string{strings.TrimSpace(req.AgentID)},
		Locale:    req.Locale,
	})
}

func (s *Service) AddAgentToConversation(req AddAgentToConversationRequest) (Conversation, error) {
	return s.AddAgentToRoom(req)
}

func (s *Service) Room(roomID string) (Room, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return Room{}, false
	}
	return s.presentRoomLocked(*room), true
}

func (s *Service) Conversation(conversationID string) (Conversation, bool) {
	return s.Room(conversationID)
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
		case "room_created":
			return fmt.Sprintf("Created room \"%s\". Welcome everyone.", title)
		case "room_members_added":
			return fmt.Sprintf("Added %s to the room.", strings.Join(s.formatHandles(userIDs), ", "))
		}
	default:
		switch key {
		case "room_created":
			return fmt.Sprintf("已创建房间“%s”，欢迎大家加入。", title)
		case "room_members_added":
			return fmt.Sprintf("已将 %s 添加到房间。", s.formatUserList(userIDs))
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

func formatRoomSubtitle(count int) string {
	return fmt.Sprintf("%d members", count)
}

func formatConversationSubtitle(count int) string {
	return formatRoomSubtitle(count)
}

func (s *Service) presentRoomLocked(room Room) Room {
	cloned := cloneRoom(room)
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

func (s *Service) presentConversationLocked(conv Conversation) Conversation {
	return s.presentRoomLocked(conv)
}

func cloneRoom(room Room) Room {
	cloned := room
	cloned.Participants = append([]string(nil), room.Participants...)
	cloned.Messages = append([]Message(nil), room.Messages...)
	return cloned
}

func cloneConversation(conv Conversation) Conversation {
	return cloneRoom(conv)
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

	rooms := make([]Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, cloneRoom(*room))
	}
	slices.SortFunc(rooms, func(a, b Room) int {
		return latestRoomMessageAt(b).Compare(latestRoomMessageAt(a))
	})

	return Bootstrap{
		CurrentUserID: s.currentUserID,
		Users:         users,
		Rooms:         rooms,
	}
}

func (s *Service) ensureAdminAgentRoomLocked(agentID, agentName string) (*Room, bool) {
	for _, room := range s.rooms {
		if len(room.Participants) != 2 {
			continue
		}
		if containsUserIDInRoom(*room, "u-admin") && containsUserIDInRoom(*room, agentID) {
			presented := s.presentRoomLocked(*room)
			return &presented, false
		}
	}

	now := time.Now().UTC()
	room := Room{
		ID:           fmt.Sprintf("room-%d", now.UnixNano()),
		Title:        agentName,
		Subtitle:     formatRoomSubtitle(2),
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
	s.rooms[room.ID] = &room
	presented := s.presentRoomLocked(room)
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

func latestRoomMessageAt(room Room) time.Time {
	if len(room.Messages) == 0 {
		return time.Time{}
	}
	return room.Messages[len(room.Messages)-1].CreatedAt
}

func latestMessageAt(conv Conversation) time.Time {
	return latestRoomMessageAt(conv)
}

func seedTime(hour, minute int) time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
}
