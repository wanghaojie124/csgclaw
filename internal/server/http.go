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
	mux.HandleFunc("/api/v1/im/bootstrap", s.handleIMBootstrap)
	mux.HandleFunc("/api/v1/im/events", s.handleIMEvents)
	mux.HandleFunc("/api/v1/im/messages", s.handleIMMessages)
	mux.HandleFunc("/api/v1/im/conversations", s.handleIMConversations)
	mux.HandleFunc("/api/v1/im/conversations/members", s.handleIMConversationMembers)
	mux.HandleFunc("/api/v1/im/rooms", s.handleIMConversations)
	mux.HandleFunc("/api/v1/im/rooms/invite", s.handleIMConversationMembers)
	mux.HandleFunc("/api/bots/", s.handlePicoClawBotRoutes)
	mux.Handle("/", uiHandler())
	return mux
}

func (s *HTTPServer) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
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
		var req CreateAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}

		created, err := s.svc.Create(r.Context(), agent.CreateRequest(req))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusCreated, created)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleIMBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.im.Bootstrap())
}

func (s *HTTPServer) handleIMMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req im.CreateMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	message, err := s.im.CreateMessage(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.publishMessageCreated(req.ConversationID, req.SenderID, message)
	writeJSON(w, http.StatusCreated, message)
}

func (s *HTTPServer) handleIMConversations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req im.CreateConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	conversation, err := s.im.CreateConversation(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.publishConversationEvent(im.EventTypeConversationCreated, conversation)
	writeJSON(w, http.StatusCreated, conversation)
}

func (s *HTTPServer) handleIMConversationMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req im.AddConversationMembersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	conversation, err := s.im.AddConversationMembers(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.publishConversationEvent(im.EventTypeConversationMembersAdded, conversation)
	writeJSON(w, http.StatusOK, conversation)
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
			data, err := json.Marshal(evt)
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
		Type:           im.EventTypeMessageCreated,
		ConversationID: conversationID,
		Message:        &messageCopy,
		Sender:         &senderCopy,
	})
}

func (s *HTTPServer) publishConversationEvent(eventType string, conversation im.Conversation) {
	if s.imBus == nil {
		return
	}
	conversationCopy := conversation
	s.imBus.Publish(im.Event{
		Type:         eventType,
		Conversation: &conversationCopy,
	})
}

func (s *HTTPServer) publishPicoClawEvent(evt im.Event) {
	if s.picoclaw == nil || evt.Type != im.EventTypeMessageCreated || evt.Message == nil || evt.Sender == nil {
		return
	}
	conversation, ok := s.im.Conversation(evt.ConversationID)
	if !ok {
		return
	}
	s.picoclaw.PublishMessageEvent(conversation, *evt.Sender, *evt.Message)
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

type Client struct {
	baseURL string
	http    *http.Client
}

type CreateAgentRequest struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{baseURL: baseURL, http: httpClient}
}

func (c *Client) CreateAgent(ctx context.Context, req CreateAgentRequest) (agent.Agent, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return agent.Agent{}, fmt.Errorf("encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/agents", bytes.NewReader(body))
	if err != nil {
		return agent.Agent{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return agent.Agent{}, fmt.Errorf("call api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		return agent.Agent{}, fmt.Errorf("api returned %s: %s", resp.Status, bytes.TrimSpace(data))
	}

	var created agent.Agent
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return agent.Agent{}, fmt.Errorf("decode response: %w", err)
	}
	return created, nil
}

func (c *Client) ListAgents(ctx context.Context) ([]agent.Agent, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/agents", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api returned %s: %s", resp.Status, bytes.TrimSpace(data))
	}

	var agents []agent.Agent
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return agents, nil
}
