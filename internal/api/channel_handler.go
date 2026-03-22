package api

import (
	"encoding/json"
	"net/http"

	"github.com/openilink/openilink-hub/internal/auth"
	"github.com/openilink/openilink-hub/internal/database"
)

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	// Get user's bot IDs, then list channels for those bots
	bots, err := s.DB.ListBotsByUser(userID)
	if err != nil {
		jsonError(w, "list failed", http.StatusInternalServerError)
		return
	}
	botIDs := make([]string, len(bots))
	for i, b := range bots {
		botIDs[i] = b.ID
	}

	channels, err := s.DB.ListChannelsByBotIDs(botIDs)
	if err != nil {
		jsonError(w, "list failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channels)
}

func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		BotID      string               `json:"bot_id"`
		Name       string               `json:"name"`
		Handle     string               `json:"handle"`
		FilterRule *database.FilterRule  `json:"filter_rule,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BotID == "" || req.Name == "" {
		jsonError(w, "bot_id and name required", http.StatusBadRequest)
		return
	}

	// Verify bot ownership
	bot, err := s.DB.GetBot(req.BotID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "bot not found", http.StatusNotFound)
		return
	}

	ch, err := s.DB.CreateChannel(req.BotID, req.Name, req.Handle, req.FilterRule)
	if err != nil {
		jsonError(w, "create failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ch)
}

func (s *Server) handleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	ch, err := s.DB.GetChannel(id)
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	// Verify ownership via bot
	bot, err := s.DB.GetBot(ch.BotID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	var req struct {
		Name       string              `json:"name"`
		Handle     *string             `json:"handle"`
		FilterRule *database.FilterRule `json:"filter_rule"`
		Enabled    *bool               `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	name := ch.Name
	if req.Name != "" {
		name = req.Name
	}
	handle := ch.Handle
	if req.Handle != nil {
		handle = *req.Handle
	}
	filter := &ch.FilterRule
	if req.FilterRule != nil {
		filter = req.FilterRule
	}
	enabled := ch.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if err := s.DB.UpdateChannel(id, name, handle, filter, enabled); err != nil {
		jsonError(w, "update failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	ch, err := s.DB.GetChannel(id)
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	// Verify ownership via bot
	bot, err := s.DB.GetBot(ch.BotID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	if err := s.DB.DeleteChannel(id); err != nil {
		jsonError(w, "delete failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

func (s *Server) handleRotateKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	ch, err := s.DB.GetChannel(id)
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	// Verify ownership via bot
	bot, err := s.DB.GetBot(ch.BotID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	newKey, err := s.DB.RotateChannelKey(id)
	if err != nil {
		jsonError(w, "rotate failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"api_key": newKey})
}
