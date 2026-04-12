package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"csgclaw/internal/channel"
	"csgclaw/internal/im"
)

func (h *Handler) handleFeishuUsers(w http.ResponseWriter, r *http.Request) {
	if h.feishu == nil {
		http.Error(w, "feishu channel is not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, h.feishu.ListUsers())
	case http.MethodPost:
		var req channel.FeishuCreateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		user, err := h.feishu.CreateUser(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, user)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleFeishuRooms(w http.ResponseWriter, r *http.Request) {
	if h.feishu == nil {
		http.Error(w, "feishu channel is not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rooms, err := h.feishu.ListRooms()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, rooms)
	case http.MethodPost:
		var req im.CreateRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		room, err := h.feishu.CreateRoom(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, room)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleFeishuRoomByID(w http.ResponseWriter, r *http.Request) {
	if h.feishu == nil {
		http.Error(w, "feishu channel is not configured", http.StatusServiceUnavailable)
		return
	}

	roomID, ok := parseFeishuRoomMembersPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		members, err := h.feishu.ListRoomMembers(roomID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "room not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, members)
	case http.MethodPost:
		var req im.AddRoomMembersRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		req.RoomID = roomID
		room, err := h.feishu.AddRoomMembers(req)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, room)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func parseFeishuRoomMembersPath(path string) (string, bool) {
	const prefix = "/api/v1/channels/feishu/rooms/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	roomID, suffix, ok := strings.Cut(rest, "/")
	if !ok || strings.TrimSpace(roomID) == "" || suffix != "members" {
		return "", false
	}
	return roomID, true
}
