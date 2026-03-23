package api

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/openilink/openilink-hub/internal/auth"
	"github.com/openilink/openilink-hub/internal/database"
)

//go:embed plugin_skill.md
var pluginSkillMD string

func handlePluginSkill(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write([]byte(pluginSkillMD))
}

// POST /api/webhook-plugins/submit
// Body: {"github_url": "https://github.com/user/repo/blob/main/plugin.js"}
// Alt:  {"script": "// @name ...\nfunction onRequest(ctx) {...}"} (inline, no GitHub)
func (s *Server) handleSubmitPlugin(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		GithubURL string `json:"github_url"`
		Script    string `json:"script"` // inline submission (alternative to github_url)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	var script, githubURL, commitHash string

	if req.Script != "" {
		// Inline submission
		script = req.Script
	} else if req.GithubURL != "" {
		// GitHub fetch
		rawURL, owner, repo, path, err := parseGithubBlobURL(req.GithubURL)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		var fetchErr error
		script, fetchErr = fetchURL(rawURL)
		if fetchErr != nil {
			jsonError(w, "failed to fetch script: "+fetchErr.Error(), http.StatusBadGateway)
			return
		}
		githubURL = req.GithubURL
		commitHash, err = fetchGithubCommitHash(owner, repo, path)
		if err != nil {
			slog.Warn("failed to fetch commit hash, using empty", "err", err)
		}
	} else {
		jsonError(w, "github_url or script required", http.StatusBadRequest)
		return
	}

	// Parse plugin metadata from script comments
	meta := parsePluginMeta(script)
	if meta.Name == "" {
		jsonError(w, "plugin must have @name in comments", http.StatusBadRequest)
		return
	}

	configSchema, _ := json.Marshal(meta.Config)

	newPlugin := &database.Plugin{
		Name:           meta.Name,
		Description:    meta.Description,
		Author:         meta.Author,
		Version:        meta.Version,
		Namespace:      meta.Namespace,
		Icon:           meta.Icon,
		License:        meta.License,
		Homepage:       meta.Homepage,
		MatchTypes:     meta.Match,
		ConnectDomains: meta.Connect,
		GrantPerms:     strings.Join(meta.Grant, ","),
		GithubURL:      githubURL,
		CommitHash:     commitHash,
		Script:         script,
		ConfigSchema:   configSchema,
		SubmittedBy:    userID,
	}

	// Check for existing pending plugin by same submitter + name → update instead of creating duplicate
	var plugin *database.Plugin
	existing, _ := s.DB.FindPendingPlugin(userID, meta.Name)
	if existing != nil {
		if err := s.DB.UpdatePlugin(existing.ID, newPlugin); err != nil {
			jsonError(w, "update failed", http.StatusInternalServerError)
			return
		}
		plugin, _ = s.DB.GetPlugin(existing.ID)
	} else {
		var err error
		plugin, err = s.DB.CreatePlugin(newPlugin)
		if err != nil {
			jsonError(w, "save failed", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plugin)
}

// optionalUser tries to extract the current user from session cookie (for public endpoints).
func (s *Server) optionalUser(r *http.Request) *database.User {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	userID, err := auth.ValidateSession(s.DB, cookie.Value)
	if err != nil {
		return nil
	}
	user, _ := s.DB.GetUserByID(userID)
	return user
}

// GET /api/webhook-plugins — list approved plugins (public), pending/rejected (admin only)
func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "approved"
	}
	// Only admin can see pending/rejected
	if status != "approved" {
		user := s.optionalUser(r)
		if user == nil || user.Role != database.RoleAdmin {
			status = "approved"
		}
	}

	plugins, err := s.DB.ListPlugins(status)
	if err != nil {
		slog.Error("list plugins failed", "status", status, "err", err)
		jsonError(w, "list failed", http.StatusInternalServerError)
		return
	}
	// Don't expose script content in list
	for i := range plugins {
		plugins[i].Script = ""
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plugins)
}

// GET /api/plugins/{id} — get plugin detail (with script for admin/owner)
func (s *Server) handleGetPlugin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	plugin, err := s.DB.GetPlugin(id)
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	user := s.optionalUser(r)
	userID := ""
	if user != nil {
		userID = user.ID
	}
	isAdmin := user != nil && user.Role == database.RoleAdmin
	isOwner := userID != "" && plugin.SubmittedBy == userID

	// Only admin and owner can see pending/rejected plugins
	if plugin.Status != "approved" && !isAdmin && !isOwner {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	// Hide script from non-admin non-owner (they can see it on GitHub)
	if !isAdmin && !isOwner {
		plugin.Script = ""
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plugin)
}

// PUT /api/admin/plugins/{id}/review — approve or reject
func (s *Server) handleReviewPlugin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		Status string `json:"status"` // "approved" or "rejected"
		Reason string `json:"reason"` // rejection reason (optional)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Status != "approved" && req.Status != "rejected" {
		jsonError(w, "status must be approved or rejected", http.StatusBadRequest)
		return
	}

	if err := s.DB.UpdatePluginStatus(id, req.Status, userID, req.Reason); err != nil {
		jsonError(w, "update failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

// DELETE /api/admin/plugins/{id}
func (s *Server) handleDeletePlugin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.DB.DeletePlugin(id); err != nil {
		jsonError(w, "delete failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

// POST /api/plugins/{id}/install — get script for installation into a channel
func (s *Server) handleInstallPlugin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	plugin, err := s.DB.GetPlugin(id)
	if err != nil || plugin.Status != "approved" {
		jsonError(w, "plugin not found or not approved", http.StatusNotFound)
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	s.DB.RecordPluginInstall(id, userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"plugin_id":     plugin.ID,
		"name":          plugin.Name,
		"script":        plugin.Script,
		"config_schema": plugin.ConfigSchema,
	})
}

// POST /api/webhook-plugins/{id}/install-to-channel
// Body: {"bot_id": "xxx", "channel_id": "xxx"}
func (s *Server) handleInstallPluginToChannel(w http.ResponseWriter, r *http.Request) {
	pluginID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	plugin, err := s.DB.GetPlugin(pluginID)
	if err != nil || plugin.Status != "approved" {
		jsonError(w, "plugin not found or not approved", http.StatusNotFound)
		return
	}

	var req struct {
		BotID     string `json:"bot_id"`
		ChannelID string `json:"channel_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BotID == "" || req.ChannelID == "" {
		jsonError(w, "bot_id and channel_id required", http.StatusBadRequest)
		return
	}

	// Verify ownership
	bot, err := s.DB.GetBot(req.BotID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "bot not found", http.StatusNotFound)
		return
	}
	ch, err := s.DB.GetChannel(req.ChannelID)
	if err != nil || ch.BotID != req.BotID {
		jsonError(w, "channel not found", http.StatusNotFound)
		return
	}

	// Set plugin_id in webhook config, keep existing URL/auth
	ch.WebhookConfig.PluginID = pluginID
	ch.WebhookConfig.Script = "" // clear inline script when using plugin
	if err := s.DB.UpdateChannel(ch.ID, ch.Name, ch.Handle, &ch.FilterRule, &ch.AIConfig, &ch.WebhookConfig, ch.Enabled); err != nil {
		jsonError(w, "update channel failed", http.StatusInternalServerError)
		return
	}

	s.DB.RecordPluginInstall(pluginID, userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":             true,
		"plugin_id":      plugin.ID,
		"plugin_name":    plugin.Name,
		"plugin_version": plugin.Version,
	})
}

// --- Helpers ---

var githubBlobRe = regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/blob/([^/]+)/(.+)$`)

func parseGithubBlobURL(url string) (rawURL, owner, repo, path string, err error) {
	m := githubBlobRe.FindStringSubmatch(url)
	if m == nil {
		return "", "", "", "", fmt.Errorf("invalid GitHub URL, expected: https://github.com/user/repo/blob/branch/path/to/plugin.js")
	}
	owner, repo, branch, path := m[1], m[2], m[3], m[4]
	rawURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, branch, path)
	return rawURL, owner, repo, path, nil
}

func fetchURL(url string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func fetchGithubCommitHash(owner, repo, path string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits?path=%s&per_page=1", owner, repo, path)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var commits []struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil || len(commits) == 0 {
		return "", fmt.Errorf("no commits found")
	}
	return commits[0].SHA, nil
}

type pluginMeta struct {
	Name        string
	Description string
	Author      string
	Version     string
	Namespace   string
	Icon        string
	License     string
	Homepage    string
	Match       string   // comma-separated msg types or "*"
	Connect     string   // comma-separated domains or "*"
	Grant       []string // "reply", "skip"
	Config      []database.ConfigField
}

var metaRe = regexp.MustCompile(`//\s*@(\w+)\s+(.+)`)

func parsePluginMeta(script string) pluginMeta {
	var meta pluginMeta
	// Find metadata block: between ==WebhookPlugin== markers
	// If no markers, fall back to scanning all lines (backward compat)
	lines := strings.Split(script, "\n")
	inBlock := false
	hasBlock := strings.Contains(script, "==WebhookPlugin==")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if hasBlock {
			if strings.Contains(trimmed, "// ==WebhookPlugin==") || strings.Contains(trimmed, "// ==/WebhookPlugin==") {
				inBlock = !inBlock
				continue
			}
			if !inBlock {
				continue
			}
		}

		m := metaRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		key, val := m[1], strings.TrimSpace(m[2])
		switch key {
		case "name":
			meta.Name = val
		case "description":
			meta.Description = val
		case "author":
			meta.Author = val
		case "version":
			meta.Version = val
		case "namespace":
			meta.Namespace = val
		case "icon":
			meta.Icon = val
		case "license":
			meta.License = val
		case "homepage":
			meta.Homepage = val
		case "match":
			meta.Match = val
		case "connect":
			meta.Connect = val
		case "grant":
			for _, g := range strings.Split(val, ",") {
				g = strings.TrimSpace(g)
				if g != "" {
					meta.Grant = append(meta.Grant, g)
				}
			}
		case "config":
			parts := strings.SplitN(val, " ", 3)
			if len(parts) >= 2 {
				desc := ""
				if len(parts) == 3 {
					desc = strings.Trim(parts[2], `"`)
				}
				meta.Config = append(meta.Config, database.ConfigField{
					Name: parts[0], Type: parts[1], Description: desc,
				})
			}
		}
	}
	return meta
}
