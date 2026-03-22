package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/openilink/openilink-hub/internal/auth"
)

func (s *Server) handleRetryMedia(w http.ResponseWriter, r *http.Request) {
	botID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	bot, err := s.DB.GetBot(botID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	msgIDStr := r.PathValue("msgId")
	msgID, err := strconv.ParseInt(msgIDStr, 10, 64)
	if err != nil {
		jsonError(w, "invalid message id", http.StatusBadRequest)
		return
	}

	msg, err := s.DB.GetMessage(msgID)
	if err != nil || msg.BotID != botID {
		jsonError(w, "message not found", http.StatusNotFound)
		return
	}

	if err := s.BotManager.RetryMediaDownload(msgID); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w)
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := r.PathValue("id")

	// Verify bot ownership
	bot, err := s.DB.GetBot(botID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	var beforeID int64
	if b := r.URL.Query().Get("before"); b != "" {
		beforeID, _ = strconv.ParseInt(b, 10, 64)
	}

	msgs, err := s.DB.ListMessages(botID, limit, beforeID)
	if err != nil {
		jsonError(w, "query failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}
