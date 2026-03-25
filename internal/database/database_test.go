package database

import (
	"encoding/json"
	"os"
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}
	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func cleanupUser(t *testing.T, db *DB, id string) {
	t.Helper()
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id = $1", id) })
}

func TestUserCRUD(t *testing.T) {
	db := testDB(t)

	user, err := db.CreateUser("test_user_crud", "Test User")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	cleanupUser(t, db, user.ID)

	got, err := db.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Username != "test_user_crud" {
		t.Errorf("username = %q", got.Username)
	}

	got2, err := db.GetUserByUsername("test_user_crud")
	if err != nil || got2.ID != user.ID {
		t.Fatalf("get by username failed")
	}

	if err := db.UpdateUserProfile(user.ID, "New Name", "test@example.com"); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	got, _ = db.GetUserByID(user.ID)
	if got.DisplayName != "New Name" || got.Email != "test@example.com" {
		t.Errorf("profile = %q / %q", got.DisplayName, got.Email)
	}

	users, err := db.ListUsers()
	if err != nil || len(users) == 0 {
		t.Fatalf("list users failed")
	}
}

func TestBotCRUD(t *testing.T) {
	db := testDB(t)

	user, _ := db.CreateUser("test_bot_user2", "Bot User")
	cleanupUser(t, db, user.ID)

	creds, _ := json.Marshal(map[string]string{"bot_id": "ilink-bot-123", "bot_token": "token-abc"})
	bot, err := db.CreateBot(user.ID, "MyBot", "ilink", "ilink-bot-123", creds)
	if err != nil {
		t.Fatalf("create bot: %v", err)
	}
	if bot.Name != "MyBot" || bot.Status != "connected" {
		t.Errorf("name=%q status=%q", bot.Name, bot.Status)
	}

	bots, err := db.ListBotsByUser(user.ID)
	if err != nil || len(bots) != 1 {
		t.Fatalf("list bots: got %d", len(bots))
	}

	db.UpdateBotName(bot.ID, "Renamed")
	got, _ := db.GetBot(bot.ID)
	if got.Name != "Renamed" {
		t.Errorf("name after rename = %q", got.Name)
	}

	db.IncrBotMsgCount(bot.ID)
	db.IncrBotMsgCount(bot.ID)
	got, _ = db.GetBot(bot.ID)
	if got.MsgCount != 2 {
		t.Errorf("msg_count = %d, want 2", got.MsgCount)
	}

	stats, err := db.GetBotStats(user.ID)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TotalBots != 1 {
		t.Errorf("total_bots = %d", stats.TotalBots)
	}

	db.DeleteBot(bot.ID)
}

func TestChannelWithFilter(t *testing.T) {
	db := testDB(t)

	user, _ := db.CreateUser("test_ch_filter", "Ch Filter")
	cleanupUser(t, db, user.ID)

	creds, _ := json.Marshal(map[string]string{"mock": "true"})
	bot, _ := db.CreateBot(user.ID, "", "mock", "", creds)

	filter := &FilterRule{
		UserIDs:  []string{"user-a", "user-b"},
		Keywords: []string{"help"},
	}
	ch, err := db.CreateChannel(bot.ID, "Filtered", "filtered", filter, nil)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if len(ch.FilterRule.UserIDs) != 2 {
		t.Errorf("filter user_ids = %v", ch.FilterRule.UserIDs)
	}
	if ch.Handle != "filtered" {
		t.Errorf("handle = %q, want filtered", ch.Handle)
	}

	got, err := db.GetChannelByAPIKey(ch.APIKey)
	if err != nil {
		t.Fatalf("get by key: %v", err)
	}
	if len(got.FilterRule.UserIDs) != 2 || got.FilterRule.Keywords[0] != "help" {
		t.Errorf("filter after reload = %+v", got.FilterRule)
	}

	newFilter := &FilterRule{MessageTypes: []string{"text", "image"}}
	db.UpdateChannel(ch.ID, "Updated", "newhandle", newFilter, nil, nil, true)
	got, _ = db.GetChannel(ch.ID)
	if got.Name != "Updated" || got.Handle != "newhandle" || len(got.FilterRule.MessageTypes) != 2 {
		t.Errorf("after update = %+v", got)
	}

	db.DeleteChannel(ch.ID)
	db.DeleteBot(bot.ID)
}

func TestMessageCRUD(t *testing.T) {
	db := testDB(t)

	user, _ := db.CreateUser("test_msg_user2", "Msg User")
	cleanupUser(t, db, user.ID)

	creds, _ := json.Marshal(map[string]string{"mock": "true"})
	bot, _ := db.CreateBot(user.ID, "", "mock", "", creds)

	itemList, _ := json.Marshal([]map[string]any{{"type": "text", "text": "hello"}})
	for i := 0; i < 5; i++ {
		db.SaveMessage(&Message{
			BotID: bot.ID, Direction: "inbound", FromUserID: "user@im.wechat",
			MessageType: 1, ItemList: itemList,
		})
	}
	replyItems, _ := json.Marshal([]map[string]any{{"type": "text", "text": "reply"}})
	db.SaveMessage(&Message{
		BotID: bot.ID, Direction: "outbound", ToUserID: "user@im.wechat",
		MessageType: 2, ItemList: replyItems,
	})

	msgs, err := db.ListMessages(bot.ID, 10, 0)
	if err != nil || len(msgs) != 6 {
		t.Fatalf("list: got %d msgs, err=%v", len(msgs), err)
	}

	byUser, err := db.ListMessagesBySender(bot.ID, "user@im.wechat", 10)
	if err != nil || len(byUser) < 5 {
		t.Fatalf("by sender: got %d", len(byUser))
	}

	since, err := db.GetMessagesSince(bot.ID, msgs[len(msgs)-1].ID, 100)
	if err != nil {
		t.Fatalf("since: %v", err)
	}
	if len(since) != 5 {
		t.Errorf("since got %d, want 5", len(since))
	}

	db.DeleteBot(bot.ID)
}

func TestCredentialCRUD(t *testing.T) {
	db := testDB(t)

	user, _ := db.CreateUser("test_cred_user2", "Cred User")
	cleanupUser(t, db, user.ID)

	if err := db.SaveCredential(&Credential{
		ID: "cred-id-2", UserID: user.ID, PublicKey: []byte("pk"),
		AttestationType: "none", Transport: `["internal"]`,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	creds, _ := db.GetCredentialsByUserID(user.ID)
	if len(creds) != 1 {
		t.Fatalf("got %d creds", len(creds))
	}

	db.UpdateCredentialSignCount("cred-id-2", 5)
	creds, _ = db.GetCredentialsByUserID(user.ID)
	if creds[0].SignCount != 5 {
		t.Errorf("sign_count = %d", creds[0].SignCount)
	}
}

func TestSystemConfig(t *testing.T) {
	db := testDB(t)

	if err := db.SetConfig("oauth.github.client_id", "test-id"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := db.SetConfig("oauth.github.client_secret", "test-secret"); err != nil {
		t.Fatalf("set: %v", err)
	}

	val, err := db.GetConfig("oauth.github.client_id")
	if err != nil || val != "test-id" {
		t.Errorf("get = %q, err=%v", val, err)
	}

	configs, err := db.ListConfigByPrefix("oauth.")
	if err != nil || len(configs) < 2 {
		t.Errorf("list = %v, err=%v", configs, err)
	}

	db.DeleteConfig("oauth.github.client_id")
	db.DeleteConfig("oauth.github.client_secret")

	val, _ = db.GetConfig("oauth.github.client_id")
	if val != "" {
		t.Errorf("after delete = %q", val)
	}
}
