package api

import (
	"net/http"
)

// GET /api/v1/channels/media?key=xxx&eqp=xxx&aes=xxx
// Proxy endpoint: downloads media from CDN via bot provider, decrypts and streams back.
// Used when MinIO is not configured — media_url points here instead.
func (s *Server) handleChannelMedia(w http.ResponseWriter, r *http.Request) {
	ch, err := s.authenticateChannel(r)
	if ch == nil {
		if err != nil {
			http.Error(w, "invalid key", http.StatusUnauthorized)
		} else {
			http.Error(w, "api key required", http.StatusUnauthorized)
		}
		return
	}

	eqp := r.URL.Query().Get("eqp")
	aes := r.URL.Query().Get("aes")
	if eqp == "" || aes == "" {
		http.Error(w, "eqp and aes required", http.StatusBadRequest)
		return
	}

	inst, ok := s.BotManager.GetInstance(ch.BotID)
	if !ok {
		http.Error(w, "bot not connected", http.StatusServiceUnavailable)
		return
	}

	data, err := inst.Provider.DownloadMedia(r.Context(), eqp, aes)
	if err != nil {
		http.Error(w, "download failed", http.StatusBadGateway)
		return
	}

	// Detect content type from query or default
	ct := r.URL.Query().Get("ct")
	if ct == "" {
		ct = http.DetectContentType(data)
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}
