package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/im"
)

type Options struct {
	ListenAddr string
	Service    *agent.Service
	IM         *im.Service
	IMBus      *im.Bus
	PicoClaw   *im.PicoClawBridge
	Context    context.Context
}

type HTTPServer struct {
	svc      *agent.Service
	im       *im.Service
	imBus    *im.Bus
	picoclaw *im.PicoClawBridge
}

type imBootstrapResponse struct {
	CurrentUserID      string    `json:"current_user_id"`
	Users              []im.User `json:"users"`
	Rooms              []im.Room `json:"rooms"`
	InviteDraftUserIDs []string  `json:"invite_draft_user_ids,omitempty"`
}

type imEventResponse struct {
	Type    string      `json:"type"`
	RoomID  string      `json:"room_id,omitempty"`
	Room    *im.Room    `json:"room,omitempty"`
	User    *im.User    `json:"user,omitempty"`
	Message *im.Message `json:"message,omitempty"`
	Sender  *im.User    `json:"sender,omitempty"`
}

type createMessageRequest struct {
	RoomID   string `json:"room_id"`
	SenderID string `json:"sender_id"`
	Content  string `json:"content"`
}

type addRoomMembersRequest struct {
	RoomID    string   `json:"room_id"`
	InviterID string   `json:"inviter_id"`
	UserIDs   []string `json:"user_ids"`
	Locale    string   `json:"locale"`
}

func Run(opts Options) error {
	if opts.Context == nil {
		opts.Context = context.Background()
	}

	srv := &HTTPServer{svc: opts.Service, im: opts.IM, imBus: opts.IMBus, picoclaw: opts.PicoClaw}
	httpServer := &http.Server{
		Addr:              opts.ListenAddr,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	if opts.IMBus != nil && opts.PicoClaw != nil {
		events, cancel := opts.IMBus.Subscribe()
		defer cancel()

		go func() {
			for {
				select {
				case <-opts.Context.Done():
					return
				case evt, ok := <-events:
					if !ok {
						return
					}
					srv.publishPicoClawEvent(evt)
				}
			}
		}()
	}

	errCh := make(chan error, 1)
	go func() {
		<-opts.Context.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		errCh <- err
	}

	close(errCh)
	if err := <-errCh; err != nil {
		return err
	}
	if opts.Service != nil {
		return opts.Service.Close()
	}
	return nil
}

func (s *HTTPServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/api/v1/agents", s.handleAgents)
	mux.HandleFunc("/api/v1/agents/", s.handleAgentByID)
	mux.HandleFunc("/api/v1/bootstrap", s.handleIMBootstrap)
	mux.HandleFunc("/api/v1/events", s.handleIMEvents)
	mux.HandleFunc("/api/v1/rooms", s.handleRooms)
	mux.HandleFunc("/api/v1/rooms/", s.handleRoomByID)
	mux.HandleFunc("/api/v1/rooms/invite", s.handleIMRoomMembers)
	mux.HandleFunc("/api/v1/users", s.handleUsers)
	mux.HandleFunc("/api/v1/users/", s.handleUserByID)
	mux.HandleFunc("/api/v1/messages", s.handleMessages)
	mux.HandleFunc("/api/v1/workers", s.handleWorkers)
	mux.HandleFunc("/api/v1/im/agents/join", s.handleIMAgentJoin)
	mux.HandleFunc("/api/v1/im/bootstrap", s.handleIMBootstrap)
	mux.HandleFunc("/api/v1/im/events", s.handleIMEvents)
	mux.HandleFunc("/api/v1/im/messages", s.handleIMMessages)
	mux.HandleFunc("/api/v1/im/conversations", s.handleIMRooms)
	mux.HandleFunc("/api/v1/im/conversations/members", s.handleIMRoomMembers)
	mux.HandleFunc("/api/v1/im/rooms", s.handleIMRooms)
	mux.HandleFunc("/api/v1/im/rooms/invite", s.handleIMRoomMembers)
	mux.HandleFunc("/api/bots/", s.handlePicoClawBotRoutes)
	mux.Handle("/", uiHandler())
	return mux
}

func (s *HTTPServer) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *HTTPServer) handleWorkers(w http.ResponseWriter, r *http.Request) {
	if s.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.svc.ListWorkers())
	case http.MethodPost:
		s.handleCreateWorker(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleAgents(w http.ResponseWriter, r *http.Request) {
	if s.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.svc.List())
	case http.MethodPost:
		s.handleCreateWorker(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	if s.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/agents/"))
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		a, ok := s.svc.Agent(id)
		if !ok {
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, a)
	case http.MethodDelete:
		if err := s.svc.Delete(r.Context(), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "agent not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleCreateWorker(w http.ResponseWriter, r *http.Request) {
	var req agent.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	req.Role = agent.RoleWorker

	created, err := s.svc.CreateWorker(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.ensureWorkerIMState(created); err != nil {
		http.Error(w, fmt.Sprintf("agent created but failed to ensure im user: %v", err), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (s *HTTPServer) handleIMAgentJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	if s.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}

	var req im.AddAgentToConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	joinedAgent, ok := s.svc.Agent(req.AgentID)
	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	if _, _, err := s.im.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID:     joinedAgent.ID,
		Name:   joinedAgent.Name,
		Handle: deriveAgentHandle(joinedAgent),
		Role:   displayRole(joinedAgent.Role),
	}); err != nil {
		http.Error(w, fmt.Sprintf("ensure agent im user: %v", err), http.StatusBadGateway)
		return
	}

	if strings.TrimSpace(req.InviterID) == "" {
		req.InviterID = "u-admin"
	}
	room, err := s.im.AddAgentToRoom(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.publishRoomEvent(im.EventTypeRoomMembersAdded, room)
	writeJSON(w, http.StatusOK, room)
}

func (s *HTTPServer) handleIMBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, presentBootstrap(s.im.Bootstrap()))
}

func (s *HTTPServer) handleRooms(w http.ResponseWriter, r *http.Request) {
	if s.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.im.ListRooms())
	case http.MethodPost:
		s.handleCreateRoom(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleUsers(w http.ResponseWriter, r *http.Request) {
	if s.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.im.ListUsers())
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleMessages(w http.ResponseWriter, r *http.Request) {
	if s.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		roomID, err := roomIDFromQuery(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		messages, err := s.im.ListMessages(roomID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, messages)
	case http.MethodPost:
		s.handleCreateMessage(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleRoomByID(w http.ResponseWriter, r *http.Request) {
	if s.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/rooms/"))
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.im.DeleteRoom(id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "room not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleUserByID(w http.ResponseWriter, r *http.Request) {
	if s.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/users/"))
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.im.KickUser(id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}
			if strings.Contains(err.Error(), "cannot kick current user") {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	var req createMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	serviceReq, err := req.toServiceRequest()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	message, err := s.im.CreateMessage(serviceReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.publishMessageCreated(serviceReq.RoomID, serviceReq.SenderID, message)
	writeJSON(w, http.StatusCreated, message)
}

func (s *HTTPServer) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	var req im.CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	room, err := s.im.CreateRoom(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.publishRoomEvent(im.EventTypeRoomCreated, room)
	writeJSON(w, http.StatusCreated, room)
}

func (s *HTTPServer) handleIMMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.handleCreateMessage(w, r)
}

func (s *HTTPServer) handleIMRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.handleCreateRoom(w, r)
}

func (s *HTTPServer) handleIMRoomMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req addRoomMembersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	serviceReq, err := req.toServiceRequest()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	room, err := s.im.AddRoomMembers(serviceReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.publishRoomEvent(im.EventTypeRoomMembersAdded, room)
	writeJSON(w, http.StatusOK, room)
}

func (s *HTTPServer) handleIMEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.imBus == nil {
		http.Error(w, "im events are not configured", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events, cancel := s.imBus.Subscribe()
	defer cancel()

	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(presentEvent(evt))
			if err != nil {
				return
			}
			if _, err := io.Copy(w, bytes.NewReader([]byte("data: "))); err != nil {
				return
			}
			if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
				return
			}
			if _, err := io.WriteString(w, "\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *HTTPServer) handlePicoClawBotRoutes(w http.ResponseWriter, r *http.Request) {
	botID, action, ok := parsePicoClawBotPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if s.picoclaw == nil {
		http.Error(w, "picoclaw integration is not configured", http.StatusServiceUnavailable)
		return
	}
	if !s.picoclaw.ValidateAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch {
	case r.Method == http.MethodGet && action == "events":
		s.handlePicoClawEvents(w, r, botID)
	case r.Method == http.MethodPost && action == "messages/send":
		s.handlePicoClawSendMessage(w, r, botID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handlePicoClawEvents(w http.ResponseWriter, r *http.Request, botID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events, cancel := s.picoclaw.Subscribe(botID)
	defer cancel()

	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-events:
			data, err := evt.MarshalJSONLine()
			if err != nil {
				return
			}
			_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *HTTPServer) handlePicoClawSendMessage(w http.ResponseWriter, r *http.Request, botID string) {
	var req im.PicoClawSendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	message, err := s.im.DeliverMessage(im.DeliverMessageRequest{
		ChatID:   req.ChatID,
		SenderID: botID,
		Content:  req.Text,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.publishMessageCreated(req.ChatID, botID, message)
	writeJSON(w, http.StatusOK, map[string]string{"message_id": message.ID})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func deriveAgentHandle(a agent.Agent) string {
	if handle, ok := sanitizeHandle(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(a.Name), " ", "-"))); ok {
		return handle
	}
	if handle, ok := sanitizeHandle(strings.ToLower(strings.TrimPrefix(strings.TrimSpace(a.ID), "u-"))); ok {
		return handle
	}
	switch strings.ToLower(strings.TrimSpace(a.Role)) {
	case agent.RoleManager:
		return "manager"
	case agent.RoleWorker:
		return "worker"
	default:
		return "agent"
	}
}

func displayRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case agent.RoleManager:
		return "Manager"
	case agent.RoleWorker:
		return "Worker"
	default:
		return "Agent"
	}
}

func sanitizeHandle(input string) (string, bool) {
	var b strings.Builder
	hasAlphaNum := false
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			hasAlphaNum = true
			b.WriteRune(r)
			continue
		}
		if r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 || !hasAlphaNum {
		return "", false
	}
	return b.String(), true
}

func roomIDFromQuery(r *http.Request) (string, error) {
	roomID := strings.TrimSpace(r.URL.Query().Get("room_id"))
	if roomID == "" {
		return "", fmt.Errorf("room_id is required")
	}
	return roomID, nil
}

func presentBootstrap(state im.Bootstrap) imBootstrapResponse {
	return imBootstrapResponse{
		CurrentUserID:      state.CurrentUserID,
		Users:              state.Users,
		Rooms:              state.Rooms,
		InviteDraftUserIDs: state.InviteDraftUserIDs,
	}
}

func presentEvent(evt im.Event) imEventResponse {
	return imEventResponse{
		Type:    evt.Type,
		RoomID:  evt.RoomID,
		Room:    evt.Room,
		User:    evt.User,
		Message: evt.Message,
		Sender:  evt.Sender,
	}
}

func (r createMessageRequest) toServiceRequest() (im.CreateMessageRequest, error) {
	roomID := strings.TrimSpace(r.RoomID)
	if roomID == "" {
		return im.CreateMessageRequest{}, fmt.Errorf("room_id is required")
	}

	return im.CreateMessageRequest{
		RoomID:   roomID,
		SenderID: r.SenderID,
		Content:  r.Content,
	}, nil
}

func (r addRoomMembersRequest) toServiceRequest() (im.AddRoomMembersRequest, error) {
	roomID := strings.TrimSpace(r.RoomID)
	if roomID == "" {
		return im.AddRoomMembersRequest{}, fmt.Errorf("room_id is required")
	}

	return im.AddRoomMembersRequest{
		RoomID:    roomID,
		InviterID: r.InviterID,
		UserIDs:   r.UserIDs,
		Locale:    r.Locale,
	}, nil
}

func (s *HTTPServer) ensureWorkerIMState(created agent.Agent) error {
	if s.im == nil {
		return nil
	}

	user, room, err := s.im.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID:     created.ID,
		Name:   created.Name,
		Handle: deriveAgentHandle(created),
		Role:   displayRole(created.Role),
	})
	if err != nil {
		return err
	}
	s.publishUserEvent(im.EventTypeUserCreated, user)
	if room != nil {
		s.publishRoomEvent(im.EventTypeRoomCreated, *room)
	}
	return nil
}

func (s *HTTPServer) publishMessageCreated(conversationID, senderID string, message im.Message) {
	if s.imBus == nil {
		return
	}
	sender, ok := s.im.User(senderID)
	if !ok {
		return
	}
	messageCopy := message
	senderCopy := sender
	s.imBus.Publish(im.Event{
		Type:    im.EventTypeMessageCreated,
		RoomID:  conversationID,
		Message: &messageCopy,
		Sender:  &senderCopy,
	})
}

func (s *HTTPServer) publishRoomEvent(eventType string, room im.Room) {
	if s.imBus == nil {
		return
	}
	roomCopy := room
	s.imBus.Publish(im.Event{
		Type:   eventType,
		RoomID: room.ID,
		Room:   &roomCopy,
	})
}

func (s *HTTPServer) publishUserEvent(eventType string, user im.User) {
	if s.imBus == nil {
		return
	}
	userCopy := user
	s.imBus.Publish(im.Event{
		Type: eventType,
		User: &userCopy,
	})
}

func (s *HTTPServer) publishPicoClawEvent(evt im.Event) {
	if s.picoclaw == nil || evt.Type != im.EventTypeMessageCreated || evt.Message == nil || evt.Sender == nil {
		return
	}
	room, ok := s.im.Room(evt.RoomID)
	if !ok {
		return
	}
	s.picoclaw.PublishMessageEvent(room, *evt.Sender, *evt.Message)
}

func parsePicoClawBotPath(path string) (botID, action string, ok bool) {
	const prefix = "/api/bots/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}

	rest := strings.TrimPrefix(path, prefix)
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		return "", "", false
	}

	botID = parts[0]
	action = strings.Join(parts[1:], "/")
	switch action {
	case "events", "messages/send":
		return botID, action, true
	default:
		return "", "", false
	}
}
