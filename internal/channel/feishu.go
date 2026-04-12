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

const feishuManagerAppUserID = "u-manager"

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

type FeishuAddChatMembersRequest struct {
	ChatID       string
	MemberIDs    []string
	MemberAppIDs []string
}

type FeishuAddChatMembersFunc func(context.Context, FeishuAppConfig, FeishuAddChatMembersRequest) error

type FeishuListChatMembersFunc func(context.Context, FeishuAppConfig, string) ([]im.User, error)

type FeishuListChatsFunc func(context.Context, FeishuAppConfig) ([]im.Room, error)

type FeishuService struct {
	mu              sync.RWMutex
	users           map[string]im.User
	byHandle        map[string]string
	rooms           map[string]*im.Room
	apps            map[string]FeishuAppConfig
	createChat      FeishuCreateChatFunc
	addChatMembers  FeishuAddChatMembersFunc
	listChatMembers FeishuListChatMembersFunc
	listChats       FeishuListChatsFunc
}

func NewFeishuService(apps ...map[string]FeishuAppConfig) *FeishuService {
	configuredApps := make(map[string]FeishuAppConfig)
	if len(apps) > 0 {
		for name, app := range apps[0] {
			configuredApps[name] = app
		}
	}
	return &FeishuService{
		users:           make(map[string]im.User),
		byHandle:        make(map[string]string),
		rooms:           make(map[string]*im.Room),
		apps:            configuredApps,
		createChat:      defaultFeishuCreateChat,
		addChatMembers:  defaultFeishuAddChatMembers,
		listChatMembers: defaultFeishuListChatMembers,
		listChats:       defaultFeishuListChats,
	}
}

func NewFeishuServiceWithCreateChat(apps map[string]FeishuAppConfig, createChat FeishuCreateChatFunc) *FeishuService {
	svc := NewFeishuService(apps)
	if createChat != nil {
		svc.createChat = createChat
	}
	return svc
}

func NewFeishuServiceWithCreateChatAndAddMembers(apps map[string]FeishuAppConfig, createChat FeishuCreateChatFunc, addChatMembers FeishuAddChatMembersFunc, listChatMembers ...FeishuListChatMembersFunc) *FeishuService {
	svc := NewFeishuServiceWithCreateChat(apps, createChat)
	if addChatMembers != nil {
		svc.addChatMembers = addChatMembers
	}
	if len(listChatMembers) > 0 && listChatMembers[0] != nil {
		svc.listChatMembers = listChatMembers[0]
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

func defaultFeishuAddChatMembers(ctx context.Context, app FeishuAppConfig, req FeishuAddChatMembersRequest) error {
	memberAppIDs := normalizeNonEmptyStrings(req.MemberAppIDs)
	if len(memberAppIDs) == 0 {
		return fmt.Errorf("add feishu chat members: member app_ids are required")
	}

	client := lark.NewClient(app.AppID, app.AppSecret)
	addReq := larkim.NewCreateChatMembersReqBuilder().
		ChatId(req.ChatID).
		MemberIdType("app_id").
		SucceedType(0).
		Body(larkim.NewCreateChatMembersReqBodyBuilder().
			IdList(memberAppIDs).
			Build()).
		Build()

	resp, err := client.Im.V1.ChatMembers.Create(ctx, addReq)
	if err != nil {
		return fmt.Errorf("add feishu chat members: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("add feishu chat members: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
	}
	return nil
}

func defaultFeishuListChatMembers(ctx context.Context, app FeishuAppConfig, chatID string) ([]im.User, error) {
	client := lark.NewClient(app.AppID, app.AppSecret)
	members := make([]im.User, 0)
	pageToken := ""

	for {
		reqBuilder := larkim.NewGetChatMembersReqBuilder().
			ChatId(chatID).
			MemberIdType("open_id").
			PageSize(100)
		if pageToken != "" {
			reqBuilder.PageToken(pageToken)
		}

		resp, err := client.Im.V1.ChatMembers.Get(ctx, reqBuilder.Build())
		if err != nil {
			return nil, fmt.Errorf("list feishu chat members: %w", err)
		}
		if !resp.Success() {
			return nil, fmt.Errorf("list feishu chat members: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
		}
		if resp.Data == nil {
			return nil, fmt.Errorf("list feishu chat members: empty response data")
		}

		for _, item := range resp.Data.Items {
			if item == nil {
				continue
			}
			memberID := strings.TrimSpace(larkcore.StringValue(item.MemberId))
			if memberID == "" {
				continue
			}
			name := strings.TrimSpace(larkcore.StringValue(item.Name))
			if name == "" {
				name = memberID
			}
			members = append(members, im.User{
				ID:        memberID,
				Name:      name,
				Handle:    deriveHandle(name, memberID),
				Role:      "member",
				Avatar:    initials(name),
				IsOnline:  true,
				AccentHex: accentHexForID(memberID),
			})
		}

		if !larkcore.BoolValue(resp.Data.HasMore) {
			break
		}
		pageToken = strings.TrimSpace(larkcore.StringValue(resp.Data.PageToken))
		if pageToken == "" {
			break
		}
	}

	return members, nil
}

func defaultFeishuListChats(ctx context.Context, app FeishuAppConfig) ([]im.Room, error) {
	client := lark.NewClient(app.AppID, app.AppSecret)
	rooms := make([]im.Room, 0)
	pageToken := ""

	for {
		reqBuilder := larkim.NewListChatReqBuilder().
			UserIdType("open_id").
			SortType("ByCreateTimeAsc").
			PageSize(100)
		if pageToken != "" {
			reqBuilder.PageToken(pageToken)
		}

		resp, err := client.Im.V1.Chat.List(ctx, reqBuilder.Build())
		if err != nil {
			return nil, fmt.Errorf("list feishu chats: %w", err)
		}
		if !resp.Success() {
			return nil, fmt.Errorf("list feishu chats: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
		}
		if resp.Data == nil {
			return nil, fmt.Errorf("list feishu chats: empty response data")
		}

		for _, item := range resp.Data.Items {
			if item == nil {
				continue
			}
			chatID := strings.TrimSpace(larkcore.StringValue(item.ChatId))
			if chatID == "" {
				continue
			}
			title := strings.TrimSpace(larkcore.StringValue(item.Name))
			if title == "" {
				title = chatID
			}
			description := strings.TrimSpace(larkcore.StringValue(item.Description))
			participants := normalizeNonEmptyStrings([]string{larkcore.StringValue(item.OwnerId)})
			rooms = append(rooms, im.Room{
				ID:           chatID,
				Title:        title,
				Subtitle:     formatMembers(len(participants)),
				Description:  description,
				Participants: participants,
				Messages:     nil,
			})
		}

		if !larkcore.BoolValue(resp.Data.HasMore) {
			break
		}
		pageToken = strings.TrimSpace(larkcore.StringValue(resp.Data.PageToken))
		if pageToken == "" {
			break
		}
	}

	return rooms, nil
}

func (s *FeishuService) ListRooms() ([]im.Room, error) {
	app, err := s.appConfigForRoom("")
	if err != nil {
		return nil, err
	}

	rooms, err := s.listChats(context.Background(), app)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range rooms {
		local, ok := s.rooms[rooms[i].ID]
		if !ok {
			continue
		}
		if len(rooms[i].Participants) == 0 {
			rooms[i].Participants = append([]string(nil), local.Participants...)
			rooms[i].Subtitle = formatMembers(len(rooms[i].Participants))
		}
		rooms[i].Messages = append([]im.Message(nil), local.Messages...)
	}
	slices.SortFunc(rooms, func(a, b im.Room) int { return strings.Compare(a.Title, b.Title) })
	return rooms, nil
}

func (s *FeishuService) AddRoomMembers(req im.AddRoomMembersRequest) (im.Room, error) {
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return im.Room{}, fmt.Errorf("room_id is required")
	}
	if len(req.UserIDs) == 0 {
		return im.Room{}, fmt.Errorf("user_ids is required")
	}

	s.mu.Lock()
	room, ok := s.rooms[roomID]
	existing := make(map[string]struct{})
	if ok {
		for _, userID := range room.Participants {
			existing[userID] = struct{}{}
		}
	}

	newMembers := make([]string, 0, len(req.UserIDs))
	newMemberAppIDs := make([]string, 0, len(req.UserIDs))
	for _, userID := range req.UserIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, ok := existing[userID]; ok {
			continue
		}
		existing[userID] = struct{}{}
		newMembers = append(newMembers, userID)
		memberAppID := userID
		if app, ok := s.apps[userID]; ok {
			if configuredAppID := strings.TrimSpace(app.AppID); configuredAppID != "" {
				memberAppID = configuredAppID
			}
		}
		newMemberAppIDs = append(newMemberAppIDs, memberAppID)
	}
	if len(newMembers) == 0 {
		s.mu.Unlock()
		return im.Room{}, fmt.Errorf("no new users to invite")
	}
	appOwnerID := strings.TrimSpace(req.InviterID)
	if appOwnerID == "" && room != nil && len(room.Participants) > 0 {
		appOwnerID = room.Participants[0]
	}
	app, err := s.appConfigForCreatorLocked(appOwnerID)
	if err != nil {
		s.mu.Unlock()
		return im.Room{}, err
	}
	s.mu.Unlock()

	if err := s.addChatMembers(context.Background(), app, FeishuAddChatMembersRequest{
		ChatID:       roomID,
		MemberIDs:    newMembers,
		MemberAppIDs: newMemberAppIDs,
	}); err != nil {
		return im.Room{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok = s.rooms[roomID]
	if !ok {
		return im.Room{
			ID:           roomID,
			Subtitle:     formatMembers(len(newMembers)),
			Participants: append([]string(nil), newMembers...),
		}, nil
	}
	existing = make(map[string]struct{}, len(room.Participants))
	for _, userID := range room.Participants {
		existing[userID] = struct{}{}
	}
	for _, userID := range newMembers {
		if _, ok := existing[userID]; ok {
			continue
		}
		room.Participants = append(room.Participants, userID)
	}
	room.Subtitle = formatMembers(len(room.Participants))
	return cloneRoom(*room), nil
}

func (s *FeishuService) ListRoomMembers(roomID string) ([]im.User, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return nil, fmt.Errorf("room_id is required")
	}

	app, err := s.appConfigForRoom(roomID)
	if err != nil {
		return nil, err
	}

	members, err := s.listChatMembers(context.Background(), app, roomID)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]im.User, 0, len(members))
	for _, member := range members {
		if localUser, ok := s.users[member.ID]; ok {
			if member.Name != "" {
				localUser.Name = member.Name
			}
			users = append(users, localUser)
			continue
		}
		users = append(users, member)
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

	return s.appConfigForCreatorLocked(creatorID)
}

func (s *FeishuService) appConfigForCreatorLocked(creatorID string) (FeishuAppConfig, error) {
	return s.managerAppConfigLocked()
}

func (s *FeishuService) managerAppConfigLocked() (FeishuAppConfig, error) {
	app, ok := s.apps[feishuManagerAppUserID]
	if !ok {
		return FeishuAppConfig{}, fmt.Errorf("feishu app is not configured for %q", feishuManagerAppUserID)
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

func (s *FeishuService) appConfigForRoom(roomID string) (FeishuAppConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.managerAppConfigLocked()
}

func (s *FeishuService) appIDForMemberLocked(memberID string) (string, error) {
	memberID = strings.TrimSpace(memberID)
	if memberID == "" {
		return "", fmt.Errorf("member_id is required")
	}
	app, ok := s.apps[memberID]
	if !ok {
		return "", fmt.Errorf("feishu app is not configured for member_id %q", memberID)
	}
	appID := strings.TrimSpace(app.AppID)
	if appID == "" {
		return "", fmt.Errorf("feishu app_id is required for member_id %q", memberID)
	}
	return appID, nil
}

func normalizeNonEmptyStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	return normalized
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
