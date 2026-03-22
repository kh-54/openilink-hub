package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/openilink/openilink-hub/internal/api"
	"github.com/openilink/openilink-hub/internal/auth"
	"github.com/openilink/openilink-hub/internal/bot"
	"github.com/openilink/openilink-hub/internal/config"
	"github.com/openilink/openilink-hub/internal/database"
	"github.com/openilink/openilink-hub/internal/provider"
	mockProvider "github.com/openilink/openilink-hub/internal/provider/mock"
	"github.com/openilink/openilink-hub/internal/relay"
)

func testDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://openilink:openilink@localhost:15432/openilink_test?sslmode=disable"
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Skipf("skip integration test: database unavailable: %v", err)
	}
	// Clean tables for a fresh test
	for _, table := range []string{"messages", "channels", "bots", "oauth_accounts", "sessions", "credentials", "users", "system_config"} {
		db.Exec("DELETE FROM " + table)
	}
	return db
}

type testEnv struct {
	t      *testing.T
	db     *database.DB
	srv    *httptest.Server
	client *http.Client
	mgr    *bot.Manager
	hub    *relay.Hub
}

func setup(t *testing.T) *testEnv {
	t.Helper()
	db := testDB(t)

	cfg := &config.Config{
		RPOrigin: "http://localhost",
		RPID:     "localhost",
		RPName:   "Test",
		Secret:   "test-secret",
	}

	server := &api.Server{
		DB:           db,
		SessionStore: auth.NewSessionStore(),
		Config:       cfg,
		OAuthStates:  api.SetupOAuth(cfg),
	}

	hub := relay.NewHub(server.SetupUpstreamHandler())
	mgr := bot.NewManager(db, hub)
	server.BotManager = mgr
	server.Hub = hub

	ts := httptest.NewServer(server.Handler())

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	return &testEnv{t: t, db: db, srv: ts, client: client, mgr: mgr, hub: hub}
}

func (e *testEnv) close() {
	e.mgr.StopAll()
	e.srv.Close()
	e.db.Close()
}

// --- HTTP helpers ---

func (e *testEnv) post(path string, body any) map[string]any {
	e.t.Helper()
	data, _ := json.Marshal(body)
	resp, err := e.client.Post(e.srv.URL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		e.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func (e *testEnv) get(path string) (int, map[string]any) {
	e.t.Helper()
	resp, err := e.client.Get(e.srv.URL + path)
	if err != nil {
		e.t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return resp.StatusCode, result
}

func (e *testEnv) getList(path string) (int, []any) {
	e.t.Helper()
	resp, err := e.client.Get(e.srv.URL + path)
	if err != nil {
		e.t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	var result []any
	json.NewDecoder(resp.Body).Decode(&result)
	return resp.StatusCode, result
}

func (e *testEnv) del(path string) int {
	e.t.Helper()
	req, _ := http.NewRequest("DELETE", e.srv.URL+path, nil)
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatalf("DELETE %s: %v", path, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func (e *testEnv) put(path string, body any) (int, map[string]any) {
	e.t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", e.srv.URL+path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatalf("PUT %s: %v", path, err)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return resp.StatusCode, result
}

func (e *testEnv) register(username, password string) {
	e.t.Helper()
	result := e.post("/api/auth/register", map[string]string{
		"username": username,
		"password": password,
	})
	if _, ok := result["error"]; ok {
		e.t.Fatalf("register failed: %v", result["error"])
	}
}

// --- Tests ---

func TestAuthFlow(t *testing.T) {
	env := setup(t)
	defer env.close()

	// Register first user (becomes admin)
	env.register("admin", "password123")

	// Check /me
	code, me := env.get("/api/auth/me")
	if code != 200 {
		t.Fatalf("GET /me: got %d", code)
	}
	if me["username"] != "admin" {
		t.Errorf("username = %v, want admin", me["username"])
	}
	if me["role"] != "admin" {
		t.Errorf("role = %v, want admin", me["role"])
	}

	// Logout
	env.post("/api/auth/logout", nil)

	// Should be unauthorized now
	code, _ = env.get("/api/auth/me")
	if code != 401 {
		t.Errorf("after logout: got %d, want 401", code)
	}

	// Login back
	result := env.post("/api/auth/login", map[string]string{
		"username": "admin",
		"password": "password123",
	})
	if _, ok := result["error"]; ok {
		t.Fatalf("login failed: %v", result["error"])
	}

	// Register second user (becomes member)
	env2 := setup(t) // fresh client
	defer env2.close()
	// Reuse same DB - don't clean
	env2.db = env.db

	jar2, _ := cookiejar.New(nil)
	env2.client = &http.Client{Jar: jar2}
	env2.srv = env.srv

	result = env2.post("/api/auth/register", map[string]string{
		"username": "member1",
		"password": "password123",
	})
	if _, ok := result["error"]; ok {
		t.Fatalf("register member failed: %v", result["error"])
	}
}

func TestBotAndChannelCRUD(t *testing.T) {
	env := setup(t)
	defer env.close()

	env.register("testuser", "password123")

	// Create bot directly in DB (since bind flow requires iLink)
	botObj, err := env.db.CreateBot("", "TestBot", "mock", mockProvider.Credentials())
	if err != nil {
		t.Fatalf("create bot: %v", err)
	}
	// Set user_id
	env.db.Exec("UPDATE bots SET user_id = (SELECT id FROM users LIMIT 1) WHERE id = $1", botObj.ID)

	// List bots
	code, bots := env.getList("/api/bots")
	if code != 200 {
		t.Fatalf("list bots: got %d", code)
	}
	if len(bots) != 1 {
		t.Fatalf("want 1 bot, got %d", len(bots))
	}

	// Create channel
	result := env.post("/api/channels", map[string]string{
		"bot_id": botObj.ID,
		"name":   "通道1",
		"handle": "ch1",
	})
	if _, ok := result["error"]; ok {
		t.Fatalf("create channel: %v", result["error"])
	}
	chID := result["id"].(string)
	if result["handle"] != "ch1" {
		t.Errorf("handle = %v, want ch1", result["handle"])
	}

	// Create second channel
	result = env.post("/api/channels", map[string]string{
		"bot_id": botObj.ID,
		"name":   "通道2",
		"handle": "ch2",
	})
	if _, ok := result["error"]; ok {
		t.Fatalf("create channel 2: %v", result["error"])
	}

	// List channels
	code, channels := env.getList("/api/channels")
	if code != 200 || len(channels) != 2 {
		t.Fatalf("list channels: code=%d, count=%d", code, len(channels))
	}

	// Update channel handle
	code, _ = env.put("/api/channels/"+chID, map[string]any{
		"handle": "newhandle",
	})
	if code != 200 {
		t.Errorf("update channel: got %d", code)
	}

	// Delete channel
	code = env.del("/api/channels/" + chID)
	if code != 200 {
		t.Errorf("delete channel: got %d", code)
	}
}

func TestMentionRouting(t *testing.T) {
	env := setup(t)
	defer env.close()

	env.register("testuser", "password123")

	// Get user ID
	_, me := env.get("/api/auth/me")
	userID := me["id"].(string)

	// Create bot
	botObj, _ := env.db.CreateBot(userID, "TestBot", "mock", mockProvider.Credentials())

	// Start bot with mock provider
	err := env.mgr.StartBot(context.Background(), botObj)
	if err != nil {
		t.Fatalf("start bot: %v", err)
	}

	// Create channels with handles
	ch1, _ := env.db.CreateChannel(botObj.ID, "支持", "support", nil)
	ch2, _ := env.db.CreateChannel(botObj.ID, "销售", "sales", nil)
	chAll, _ := env.db.CreateChannel(botObj.ID, "全部", "", nil) // no handle, catches all via filter

	// Connect WebSocket for each channel
	ws1 := env.connectWS(t, ch1.APIKey)
	defer ws1.Close()
	ws2 := env.connectWS(t, ch2.APIKey)
	defer ws2.Close()
	wsAll := env.connectWS(t, chAll.APIKey)
	defer wsAll.Close()

	// Read init messages
	readWS(t, ws1)
	readWS(t, ws2)
	readWS(t, wsAll)

	// Get mock provider instance
	inst, ok := env.mgr.GetInstance(botObj.ID)
	if !ok {
		t.Fatal("bot instance not found")
	}
	mockP := inst.Provider.(*mockProvider.Provider)

	// Test 1: Message with @support should go to ch1 only
	mockP.SimulateInbound(provider.InboundMessage{
		ExternalID: "1",
		Sender:     "user@wechat",
		Timestamp:  time.Now().UnixMilli(),
		Items:      []provider.MessageItem{{Type: "text", Text: "@support 帮我看看"}},
	})

	msg1 := readWSWithTimeout(t, ws1, 2*time.Second)
	if msg1 == nil {
		t.Error("ch1 (support) should receive @support message")
	}

	msg2 := readWSWithTimeout(t, ws2, 500*time.Millisecond)
	if msg2 != nil {
		t.Error("ch2 (sales) should NOT receive @support message")
	}

	msgAll := readWSWithTimeout(t, wsAll, 500*time.Millisecond)
	if msgAll != nil {
		t.Error("chAll should NOT receive @mention message (mention routing skips filter)")
	}

	// Test 2: Message without @mention should go to all channels (via filter)
	// Reconnect ws2 and wsAll since deadline-based timeout may break gorilla websocket state
	ws2.Close()
	wsAll.Close()
	ws2 = env.connectWS(t, ch2.APIKey)
	defer ws2.Close()
	wsAll = env.connectWS(t, chAll.APIKey)
	defer wsAll.Close()
	readWS(t, ws2)   // consume init
	readWS(t, wsAll)  // consume init

	mockP.SimulateInbound(provider.InboundMessage{
		ExternalID: "2",
		Sender:     "user@wechat",
		Timestamp:  time.Now().UnixMilli(),
		Items:      []provider.MessageItem{{Type: "text", Text: "普通消息"}},
	})

	msg1 = readWSWithTimeout(t, ws1, 2*time.Second)
	if msg1 == nil {
		t.Error("ch1 should receive non-mention message via filter")
	}
	msg2 = readWSWithTimeout(t, ws2, 2*time.Second)
	if msg2 == nil {
		t.Error("ch2 should receive non-mention message via filter")
	}
	msgAll = readWSWithTimeout(t, wsAll, 2*time.Second)
	if msgAll == nil {
		t.Error("chAll should receive non-mention message via filter")
	}

	// Test 3: Message with @sales should go to ch2 only
	// Reconnect ws1 and wsAll
	ws1.Close()
	wsAll.Close()
	ws1 = env.connectWS(t, ch1.APIKey)
	defer ws1.Close()
	wsAll = env.connectWS(t, chAll.APIKey)
	defer wsAll.Close()
	readWS(t, ws1)
	readWS(t, wsAll)

	mockP.SimulateInbound(provider.InboundMessage{
		ExternalID: "3",
		Sender:     "user@wechat",
		Timestamp:  time.Now().UnixMilli(),
		Items:      []provider.MessageItem{{Type: "text", Text: "@sales 价格多少"}},
	})

	msg2 = readWSWithTimeout(t, ws2, 2*time.Second)
	if msg2 == nil {
		t.Error("ch2 (sales) should receive @sales message")
	}

	msg1 = readWSWithTimeout(t, ws1, 500*time.Millisecond)
	if msg1 != nil {
		t.Error("ch1 (support) should NOT receive @sales message")
	}

	// Test 4: Message with @unknown should go nowhere
	ws1.Close()
	ws2.Close()
	wsAll.Close()
	ws1 = env.connectWS(t, ch1.APIKey)
	defer ws1.Close()
	ws2 = env.connectWS(t, ch2.APIKey)
	defer ws2.Close()
	wsAll = env.connectWS(t, chAll.APIKey)
	defer wsAll.Close()
	// Drain init + any replayed messages
	drainWS(t, ws1)
	drainWS(t, ws2)
	drainWS(t, wsAll)

	mockP.SimulateInbound(provider.InboundMessage{
		ExternalID: "4",
		Sender:     "user@wechat",
		Timestamp:  time.Now().UnixMilli(),
		Items:      []provider.MessageItem{{Type: "text", Text: "@unknown test"}},
	})

	msg1 = readWSWithTimeout(t, ws1, 500*time.Millisecond)
	msg2 = readWSWithTimeout(t, ws2, 500*time.Millisecond)
	msgAll = readWSWithTimeout(t, wsAll, 500*time.Millisecond)
	if msg1 != nil || msg2 != nil || msgAll != nil {
		t.Error("@unknown should not route to any channel")
	}
}

func TestMessageOwnershipCheck(t *testing.T) {
	env := setup(t)
	defer env.close()

	// Create two users
	env.register("user1", "password123")
	_, me := env.get("/api/auth/me")
	user1ID := me["id"].(string)

	// Create bot for user1
	botObj, _ := env.db.CreateBot(user1ID, "User1Bot", "mock", mockProvider.Credentials())

	// Logout and register user2
	env.post("/api/auth/logout", nil)
	env.register("user2", "password123")

	// user2 should not see user1's messages
	code, msgs := env.getList(fmt.Sprintf("/api/messages?bot_id=%s", botObj.ID))
	if code != 404 {
		t.Errorf("user2 accessing user1's bot messages: got %d, want 404, msgs=%v", code, msgs)
	}
}

func TestChannelOwnershipCheck(t *testing.T) {
	env := setup(t)
	defer env.close()

	env.register("user1", "password123")
	_, me := env.get("/api/auth/me")
	user1ID := me["id"].(string)

	botObj, _ := env.db.CreateBot(user1ID, "Bot1", "mock", mockProvider.Credentials())
	ch, _ := env.db.CreateChannel(botObj.ID, "Chan1", "c1", nil)

	// Logout and register user2
	env.post("/api/auth/logout", nil)
	env.register("user2", "password123")

	// user2 should not be able to delete user1's channel
	code := env.del("/api/channels/" + ch.ID)
	if code != 404 {
		t.Errorf("user2 deleting user1's channel: got %d, want 404", code)
	}
}

// --- WebSocket helpers ---

func (e *testEnv) connectWS(t *testing.T, apiKey string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + e.srv.URL[4:] + "/api/ws?key=" + apiKey
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	return ws
}

func readWS(t *testing.T, ws *websocket.Conn) map[string]any {
	t.Helper()
	return readWSWithTimeout(t, ws, 2*time.Second)
}

// drainWS reads all pending messages until timeout.
func drainWS(t *testing.T, ws *websocket.Conn) {
	t.Helper()
	for {
		if readWSWithTimeout(t, ws, 300*time.Millisecond) == nil {
			return
		}
	}
}

func readWSWithTimeout(t *testing.T, ws *websocket.Conn, timeout time.Duration) map[string]any {
	t.Helper()
	ws.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := ws.ReadMessage()
	ws.SetReadDeadline(time.Time{}) // reset
	if err != nil {
		return nil
	}
	var result map[string]any
	json.Unmarshal(msg, &result)
	return result
}
