package e2e

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"ripple/server/internal/api"
	"ripple/server/internal/auth"
	"ripple/server/internal/config"
	"ripple/server/internal/models"
	"ripple/server/internal/store"
	"ripple/server/internal/testdb"
	"ripple/server/internal/ws"

	"github.com/gorilla/websocket"
)

type testUser struct {
	name, id, token string
}

type client struct {
	t    *testing.T
	base string
}

func (c *client) do(u *testUser, method, path string, body any, out any) int {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if u != nil {
		req.Header.Set("Authorization", "Bearer "+u.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("decode %s %s -> %s: %v", method, path, raw, err)
		}
	}
	return resp.StatusCode
}

func (c *client) upload(u *testUser, path string, content []byte) (int, map[string]any) {
	c.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "x.png")
	_, _ = fw.Write(content)
	_ = w.Close()
	req, err := http.NewRequest(http.MethodPost, c.base+path, &buf)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if u != nil {
		req.Header.Set("Authorization", "Bearer "+u.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func (c *client) register(name string) testUser {
	c.t.Helper()
	var out struct {
		Token string      `json:"token"`
		User  models.User `json:"user"`
	}
	code := c.do(nil, "POST", "/api/auth/register",
		map[string]any{"username": name, "password": "secret6", "display_name": name + "-disp"}, &out)
	if code != http.StatusCreated {
		c.t.Fatalf("register %s: status %d", name, code)
	}
	return testUser{name: name, id: out.User.ID, token: out.Token}
}

// pngBytes 1x1 PNG（内容嗅探用）
var pngBytes = func() []byte {
	data, _ := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")
	return data
}()

func wsDial(t *testing.T, base, token string) *websocket.Conn {
	t.Helper()
	url := strings.Replace(base, "http://", "ws://", 1) + "/api/ws?token=" + token
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	return conn
}

func readFrame(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var f map[string]any
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("frame json %s: %v", data, err)
	}
	return f
}

func readUntil(t *testing.T, conn *websocket.Conn, typ string, match func(map[string]any) bool) map[string]any {
	t.Helper()
	for i := 0; i < 100; i++ {
		f := readFrame(t, conn)
		if f["type"] == typ && (match == nil || match(f)) {
			return f
		}
	}
	t.Fatalf("frame %s not received in time", typ)
	return nil
}

func TestE2E(t *testing.T) {
	sqlDB := testdb.Open(t)
	authSvc := &auth.Service{DB: sqlDB, Secret: []byte("e2e-secret")}
	st := store.New(sqlDB, 6)
	hub := ws.NewHub(authSvc, st)
	cfg := config.Config{
		Addr: "test", MySQLDSN: "test", UploadDir: t.TempDir(),
		JWTSecret: "e2e-secret", MaxRounds: 6, MaxUpload: 2 << 20,
	}
	srv := httptest.NewServer(api.New(cfg, authSvc, st, hub))
	t.Cleanup(srv.Close)
	c := &client{t: t, base: srv.URL}

	var alice, bob, carol, dave testUser

	t.Run("register_login_profile", func(t *testing.T) {
		alice, bob, carol, dave = c.register("alice"), c.register("bob"), c.register("carol"), c.register("dave")
		// 重复注册 → 409；非法用户名 → 400；短密码 → 400
		if code := c.do(nil, "POST", "/api/auth/register", map[string]any{"username": "alice", "password": "secret6"}, nil); code != http.StatusConflict {
			t.Fatalf("duplicate register: %d", code)
		}
		if code := c.do(nil, "POST", "/api/auth/register", map[string]any{"username": "Bad Name", "password": "secret6"}, nil); code != http.StatusBadRequest {
			t.Fatalf("bad username: %d", code)
		}
		if code := c.do(nil, "POST", "/api/auth/register", map[string]any{"username": "okname", "password": "123"}, nil); code != http.StatusBadRequest {
			t.Fatalf("short password: %d", code)
		}
		// 登录
		var loginOut struct {
			Token string      `json:"token"`
			User  models.User `json:"user"`
		}
		if code := c.do(nil, "POST", "/api/auth/login", map[string]any{"username": "alice", "password": "secret6"}, &loginOut); code != http.StatusOK {
			t.Fatalf("login: %d", code)
		}
		if code := c.do(nil, "POST", "/api/auth/login", map[string]any{"username": "alice", "password": "wrong!"}, nil); code != http.StatusUnauthorized {
			t.Fatalf("bad login: %d", code)
		}
		// 资料编辑 + 头像上传
		var me models.User
		if code := c.do(&alice, "PUT", "/api/me", map[string]any{"display_name": "Alice2", "signature": "hello sea"}, &me); code != http.StatusOK {
			t.Fatalf("update me: %d", code)
		}
		if me.DisplayName != "Alice2" || me.Signature != "hello sea" {
			t.Fatalf("profile not updated: %+v", me)
		}
		code, out := c.upload(&alice, "/api/me/avatar", pngBytes)
		if code != http.StatusOK || out["avatar_url"] == nil {
			t.Fatalf("avatar upload: %d %v", code, out)
		}
		var me2 models.User
		if code := c.do(&alice, "GET", "/api/me", nil, &me2); code != http.StatusOK || me2.AvatarURL == nil {
			t.Fatalf("me after avatar: %d %+v", code, me2)
		}
	})

	t.Run("search_and_friends", func(t *testing.T) {
		// 搜索
		var searchOut struct {
			Users []models.SearchEntry `json:"users"`
		}
		if code := c.do(&alice, "GET", "/api/users/search?q=bob", nil, &searchOut); code != http.StatusOK {
			t.Fatalf("search: %d", code)
		}
		if len(searchOut.Users) == 0 || searchOut.Users[0].User.ID != bob.id || searchOut.Users[0].Relation != "none" {
			t.Fatalf("search result: %+v", searchOut.Users)
		}
		// alice→bob 申请
		var reqOut struct {
			Status string      `json:"status"`
			Friend models.User `json:"friend"`
		}
		if code := c.do(&alice, "POST", "/api/friends/requests", map[string]any{"user_id": bob.id}, &reqOut); code != http.StatusCreated || reqOut.Status != "pending" {
			t.Fatalf("alice request bob: %d %+v", code, reqOut)
		}
		// 重复申请 → 409
		if code := c.do(&alice, "POST", "/api/friends/requests", map[string]any{"user_id": bob.id}, nil); code != http.StatusConflict {
			t.Fatalf("repeat request: %d", code)
		}
		// bob 视角 pending_in
		var bobFriends models.FriendsView
		if code := c.do(&bob, "GET", "/api/friends", nil, &bobFriends); code != http.StatusOK {
			t.Fatalf("bob friends: %d", code)
		}
		if len(bobFriends.PendingIn) != 1 || bobFriends.PendingIn[0].User.ID != alice.id {
			t.Fatalf("bob pending_in: %+v", bobFriends.PendingIn)
		}
		// carol→dave、dave→carol：反向 pending 秒通过
		if code := c.do(&carol, "POST", "/api/friends/requests", map[string]any{"user_id": dave.id}, nil); code != http.StatusCreated {
			t.Fatalf("carol request dave: %d", code)
		}
		if code := c.do(&dave, "POST", "/api/friends/requests", map[string]any{"user_id": carol.id}, &reqOut); code != http.StatusOK || reqOut.Status != "accepted" {
			t.Fatalf("dave auto-accept: %d %+v", code, reqOut)
		}
		var carolFriends models.FriendsView
		if code := c.do(&carol, "GET", "/api/friends", nil, &carolFriends); code != http.StatusOK {
			t.Fatalf("carol friends: %d", code)
		}
		if len(carolFriends.Friends) != 1 || carolFriends.Friends[0].ID != dave.id {
			t.Fatalf("carol friends: %+v", carolFriends.Friends)
		}
		// bob 同意 alice
		var accOut struct {
			Friend models.User `json:"friend"`
		}
		if code := c.do(&bob, "POST", "/api/friends/requests/"+bobFriends.PendingIn[0].ID+"/accept", nil, &accOut); code != http.StatusOK || accOut.Friend.ID != alice.id {
			t.Fatalf("bob accept: %d %+v", code, accOut)
		}
		// 再同意一次 → 409
		if code := c.do(&bob, "POST", "/api/friends/requests/"+bobFriends.PendingIn[0].ID+"/accept", nil, nil); code != http.StatusConflict {
			t.Fatalf("re-accept: %d", code)
		}
		// 互为好友
		var aliceFriends models.FriendsView
		if code := c.do(&alice, "GET", "/api/friends", nil, &aliceFriends); code != http.StatusOK {
			t.Fatalf("alice friends: %d", code)
		}
		if len(aliceFriends.Friends) != 1 || aliceFriends.Friends[0].ID != bob.id {
			t.Fatalf("alice friends: %+v", aliceFriends.Friends)
		}
		// 搜索结果 relation=accepted
		if code := c.do(&alice, "GET", "/api/users/search?q=bob", nil, &searchOut); code != http.StatusOK {
			t.Fatalf("search again: %d", code)
		}
		if searchOut.Users[0].Relation != "accepted" {
			t.Fatalf("relation after accept: %s", searchOut.Users[0].Relation)
		}
		// 拒绝流程：alice→carol 申请，carol 拒绝
		if code := c.do(&alice, "POST", "/api/friends/requests", map[string]any{"user_id": carol.id}, nil); code != http.StatusCreated {
			t.Fatalf("alice request carol: %d", code)
		}
		var carolFriends2 models.FriendsView
		_ = c.do(&carol, "GET", "/api/friends", nil, &carolFriends2)
		if code := c.do(&carol, "POST", "/api/friends/requests/"+carolFriends2.PendingIn[0].ID+"/decline", nil, nil); code != http.StatusOK {
			t.Fatalf("carol decline: %d", code)
		}
		var carolFriends3 models.FriendsView
		_ = c.do(&carol, "GET", "/api/friends", nil, &carolFriends3)
		if len(carolFriends3.PendingIn) != 0 {
			t.Fatalf("decline should clear pending_in: %+v", carolFriends3.PendingIn)
		}
	})

	var groupID string

	t.Run("group_chat", func(t *testing.T) {
		// alice 建群（含 bob、carol）
		var group models.ConversationSummary
		if code := c.do(&alice, "POST", "/api/conversations",
			map[string]any{"type": "group", "name": "测试群", "member_ids": []string{bob.id, carol.id}}, &group); code != http.StatusCreated {
			t.Fatalf("create group: %d", code)
		}
		groupID = group.ID
		if group.MemberCount != 3 || group.OwnerID == nil || *group.OwnerID != alice.id {
			t.Fatalf("group summary: %+v", group)
		}
		// bob 看到群与创建系统消息
		var convs struct {
			Conversations []models.ConversationSummary `json:"conversations"`
		}
		if code := c.do(&bob, "GET", "/api/conversations", nil, &convs); code != http.StatusOK {
			t.Fatalf("bob convs: %d", code)
		}
		var seen bool
		for _, cv := range convs.Conversations {
			if cv.ID == groupID && cv.Type == "group" {
				seen = true
			}
		}
		if !seen {
			t.Fatalf("bob cannot see group: %+v", convs.Conversations)
		}
		var detail struct {
			Conversation models.ConversationSummary `json:"conversation"`
			Members      []models.MemberEntry       `json:"members"`
		}
		if code := c.do(&bob, "GET", "/api/conversations/"+groupID, nil, &detail); code != http.StatusOK {
			t.Fatalf("group detail: %d", code)
		}
		if len(detail.Members) != 3 {
			t.Fatalf("group members: %+v", detail.Members)
		}
		// 群发消息：alice 发 → bob 可见
		var sent models.Message
		if code := c.do(&alice, "POST", "/api/conversations/"+groupID+"/messages",
			map[string]any{"type": "text", "body": "群hello"}, &sent); code != http.StatusCreated {
			t.Fatalf("group send: %d", code)
		}
		var msgs struct {
			Messages []models.Message `json:"messages"`
			HasMore bool              `json:"has_more"`
		}
		if code := c.do(&bob, "GET", "/api/conversations/"+groupID+"/messages", nil, &msgs); code != http.StatusOK {
			t.Fatalf("bob group messages: %d", code)
		}
		if len(msgs.Messages) != 2 || msgs.Messages[0].Type != "system" || msgs.Messages[1].Body != "群hello" {
			t.Fatalf("group messages: %+v", msgs.Messages)
		}
		// carol 发 → alice 可见
		if code := c.do(&carol, "POST", "/api/conversations/"+groupID+"/messages",
			map[string]any{"type": "text", "body": "carol 来了"}, nil); code != http.StatusCreated {
			t.Fatalf("carol group send: %d", code)
		}
		_ = c.do(&alice, "GET", "/api/conversations/"+groupID+"/messages", nil, &msgs)
		if len(msgs.Messages) != 3 {
			t.Fatalf("alice group messages: %+v", msgs.Messages)
		}
		// 加人 dave
		var addOut struct {
			Members []models.User `json:"members"`
		}
		if code := c.do(&alice, "POST", "/api/conversations/"+groupID+"/members",
			map[string]any{"user_ids": []string{dave.id}}, &addOut); code != http.StatusOK || len(addOut.Members) != 1 {
			t.Fatalf("add member: %d %+v", code, addOut)
		}
		_ = c.do(&alice, "GET", "/api/conversations/"+groupID, nil, &detail)
		if len(detail.Members) != 4 {
			t.Fatalf("members after add: %d", len(detail.Members))
		}
		// 非群主操作 → 403
		if code := c.do(&bob, "POST", "/api/conversations/"+groupID+"/members",
			map[string]any{"user_ids": []string{carol.id}}, nil); code != http.StatusForbidden {
			t.Fatalf("non-owner add: %d", code)
		}
		// carol 退群
		if code := c.do(&carol, "POST", "/api/conversations/"+groupID+"/leave", nil, nil); code != http.StatusOK {
			t.Fatalf("carol leave: %d", code)
		}
		// 退群后不可见明细
		if code := c.do(&carol, "GET", "/api/conversations/"+groupID, nil, nil); code != http.StatusForbidden {
			t.Fatalf("left member detail: %d", code)
		}
		// 改名（群主）
		if code := c.do(&alice, "PUT", "/api/conversations/"+groupID, map[string]any{"name": "新群名"}, nil); code != http.StatusOK {
			t.Fatalf("rename: %d", code)
		}
		if code := c.do(&bob, "PUT", "/api/conversations/"+groupID, map[string]any{"name": "bob 改"}, nil); code != http.StatusForbidden {
			t.Fatalf("non-owner rename: %d", code)
		}
		// 群主移人 dave
		if code := c.do(&alice, "DELETE", "/api/conversations/"+groupID+"/members/"+dave.id, nil, nil); code != http.StatusOK {
			t.Fatalf("remove member: %d", code)
		}
		// 移群主自己 → 403
		if code := c.do(&alice, "DELETE", "/api/conversations/"+groupID+"/members/"+alice.id, nil, nil); code != http.StatusForbidden {
			t.Fatalf("remove owner: %d", code)
		}
		// 群主不可退群
		if code := c.do(&alice, "POST", "/api/conversations/"+groupID+"/leave", nil, nil); code != http.StatusForbidden {
			t.Fatalf("owner leave: %d", code)
		}
	})

	t.Run("single_chat_reliability", func(t *testing.T) {
		// alice 与 bob 建私聊（幂等：201 → 200，同 id）
		var sum models.ConversationSummary
		if code := c.do(&alice, "POST", "/api/conversations", map[string]any{"type": "single", "user_id": bob.id}, &sum); code != http.StatusCreated {
			t.Fatalf("create single: %d", code)
		}
		var sum2 models.ConversationSummary
		if code := c.do(&alice, "POST", "/api/conversations", map[string]any{"type": "single", "user_id": bob.id}, &sum2); code != http.StatusOK || sum2.ID != sum.ID {
			t.Fatalf("single idempotent: %d %+v", code, sum2)
		}
		if sum2.Peer == nil || sum2.Peer.ID != bob.id || sum2.MemberCount != 2 {
			t.Fatalf("single summary: %+v", sum2)
		}
		// 发 55 条（超过默认 limit=50，证明旧 200 条 ASC 缺口场景下仍可取最新）
		for i := 1; i <= 55; i++ {
			if code := c.do(&alice, "POST", "/api/conversations/"+sum.ID+"/messages",
				map[string]any{"type": "text", "body": "m" + strconv.Itoa(i)}, nil); code != http.StatusCreated {
				t.Fatalf("send %d: status", i)
			}
		}
		// bob 默认拉取：最新 50 条 ASC + has_more
		var msgs struct {
			Messages []models.Message `json:"messages"`
			HasMore  bool            `json:"has_more"`
		}
		if code := c.do(&bob, "GET", "/api/conversations/"+sum.ID+"/messages", nil, &msgs); code != http.StatusOK {
			t.Fatalf("bob list: %d", code)
		}
		if len(msgs.Messages) != 50 || !msgs.HasMore {
			t.Fatalf("default list: len=%d has_more=%v", len(msgs.Messages), msgs.HasMore)
		}
		if msgs.Messages[0].Seq != 6 || msgs.Messages[49].Seq != 55 {
			t.Fatalf("default list window: first=%d last=%d", msgs.Messages[0].Seq, msgs.Messages[49].Seq)
		}
		// 未读数
		var bobConvs struct {
			Conversations []models.ConversationSummary `json:"conversations"`
		}
		if code := c.do(&bob, "GET", "/api/conversations", nil, &bobConvs); code != http.StatusOK {
			t.Fatalf("bob convs: %d", code)
		}
		var single *models.ConversationSummary
		for i := range bobConvs.Conversations {
			if bobConvs.Conversations[i].ID == sum.ID {
				single = &bobConvs.Conversations[i]
			}
		}
		if single == nil || single.Unread != 55 {
			t.Fatalf("unread: %+v", single)
		}
		if single.LastMessagePreview == nil || *single.LastMessagePreview != "m55" {
			t.Fatalf("preview: %+v", single.LastMessagePreview)
		}
		// 已读上报（只升不降）
		var readOut struct {
			LastReadSeq int64 `json:"last_read_seq"`
		}
		if code := c.do(&bob, "POST", "/api/conversations/"+sum.ID+"/read", map[string]any{"seq": 55}, &readOut); code != http.StatusOK || readOut.LastReadSeq != 55 {
			t.Fatalf("read: %d %+v", code, readOut)
		}
		if code := c.do(&bob, "POST", "/api/conversations/"+sum.ID+"/read", map[string]any{"seq": 10}, &readOut); code != http.StatusOK || readOut.LastReadSeq != 55 {
			t.Fatalf("read downgrade: %d %+v", code, readOut)
		}
		_ = c.do(&bob, "GET", "/api/conversations", nil, &bobConvs)
		for i := range bobConvs.Conversations {
			if bobConvs.Conversations[i].ID == sum.ID {
				single = &bobConvs.Conversations[i]
			}
		}
		if single == nil || single.Unread != 0 {
			t.Fatalf("unread after read: %+v", single)
		}
		// after_seq 增量：alice 再发 3 条，bob 从 55 增量拉取 → 不重不丢
		for i := 56; i <= 58; i++ {
			c.do(&alice, "POST", "/api/conversations/"+sum.ID+"/messages",
				map[string]any{"type": "text", "body": "m" + strconv.Itoa(i)}, nil)
		}
		var inc struct {
			Messages []models.Message `json:"messages"`
			HasMore  bool            `json:"has_more"`
		}
		if code := c.do(&bob, "GET", "/api/conversations/"+sum.ID+"/messages?after_seq=55", nil, &inc); code != http.StatusOK {
			t.Fatalf("after_seq pull: %d", code)
		}
		if len(inc.Messages) != 3 || inc.HasMore {
			t.Fatalf("after_seq result: len=%d has_more=%v", len(inc.Messages), inc.HasMore)
		}
		for i, m := range inc.Messages {
			if m.Seq != int64(56+i) || m.Body != "m"+strconv.Itoa(56+i) {
				t.Fatalf("after_seq seq mismatch at %d: %+v", i, m)
			}
		}
		// before_seq 翻历史
		var old struct {
			Messages []models.Message `json:"messages"`
			HasMore  bool            `json:"has_more"`
		}
		if code := c.do(&bob, "GET", "/api/conversations/"+sum.ID+"/messages?before_seq=56&limit=10", nil, &old); code != http.StatusOK {
			t.Fatalf("before_seq pull: %d", code)
		}
		if len(old.Messages) != 10 || old.Messages[0].Seq != 46 || old.Messages[9].Seq != 55 || !old.HasMore {
			t.Fatalf("before_seq result: %+v", old.Messages)
		}
		// 撤回：bob 发一条，alice 也能看到；2 分钟窗口内撤回成功，双端 revoked
		var sent models.Message
		if code := c.do(&bob, "POST", "/api/conversations/"+sum.ID+"/messages",
			map[string]any{"type": "text", "body": "to be revoked"}, &sent); code != http.StatusCreated {
			t.Fatalf("send for revoke: %d", code)
		}
		var revOut struct {
			Message models.Message `json:"message"`
		}
		if code := c.do(&bob, "POST", "/api/messages/"+sent.ID+"/revoke", nil, &revOut); code != http.StatusOK {
			t.Fatalf("revoke: %d", code)
		}
		if revOut.Message.RevokedAt == nil || revOut.Message.Body != "" || revOut.Message.ImageURL != nil {
			t.Fatalf("revoked message: %+v", revOut.Message)
		}
		// alice 视角确认 revoked
		_ = c.do(&alice, "GET", "/api/conversations/"+sum.ID+"/messages?after_seq=58", nil, &inc)
		var revokedSeen bool
		for _, m := range inc.Messages {
			if m.ID == sent.ID {
				revokedSeen = m.RevokedAt != nil && m.Body == ""
			}
		}
		if !revokedSeen {
			t.Fatalf("peer does not see revoked: %+v", inc.Messages)
		}
		// 非发送者撤回 → 403
		if code := c.do(&alice, "POST", "/api/messages/"+sent.ID+"/revoke", nil, nil); code != http.StatusForbidden {
			t.Fatalf("revoke others: %d", code)
		}
		// 超过 2 分钟 → 403（把消息时间拨回 3 分钟前）
		var fresh models.Message
		c.do(&bob, "POST", "/api/conversations/"+sum.ID+"/messages", map[string]any{"type": "text", "body": "stale"}, &fresh)
		if _, err := sqlDB.Exec(`UPDATE messages SET created_at = created_at - 180000 WHERE id = ?`, fresh.ID); err != nil {
			t.Fatal(err)
		}
		if code := c.do(&bob, "POST", "/api/messages/"+fresh.ID+"/revoke", nil, nil); code != http.StatusForbidden {
			t.Fatalf("revoke after 2min: %d", code)
		}
		// 图片消息：先上传再发送
		code, up := c.upload(&alice, "/api/media/upload", pngBytes)
		if code != http.StatusOK || up["url"] == nil {
			t.Fatalf("media upload: %d %v", code, up)
		}
		var imgMsg models.Message
		if code := c.do(&alice, "POST", "/api/conversations/"+sum.ID+"/messages",
			map[string]any{"type": "image", "body": "", "image_url": up["url"].(string)}, &imgMsg); code != http.StatusCreated || imgMsg.Type != "image" {
			t.Fatalf("image message: %d", code)
		}
	})

	t.Run("block_single_message", func(t *testing.T) {
		var sum models.ConversationSummary
		if code := c.do(&bob, "POST", "/api/conversations", map[string]any{"type": "single", "user_id": carol.id}, &sum); code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("bob-carol single: %d", code)
		}
		if code := c.do(&bob, "POST", "/api/friends/"+carol.id+"/block", nil, nil); code != http.StatusOK {
			t.Fatalf("block: %d", code)
		}
		// 双向拦截
		if code := c.do(&bob, "POST", "/api/conversations/"+sum.ID+"/messages", map[string]any{"type": "text", "body": "x"}, nil); code != http.StatusForbidden {
			t.Fatalf("blocker send: %d", code)
		}
		if code := c.do(&carol, "POST", "/api/conversations/"+sum.ID+"/messages", map[string]any{"type": "text", "body": "x"}, nil); code != http.StatusForbidden {
			t.Fatalf("blocked send: %d", code)
		}
		// 重复拉黑 → 409；解除后恢复
		if code := c.do(&bob, "POST", "/api/friends/"+carol.id+"/block", nil, nil); code != http.StatusConflict {
			t.Fatalf("double block: %d", code)
		}
		if code := c.do(&bob, "DELETE", "/api/friends/"+carol.id+"/block", nil, nil); code != http.StatusOK {
			t.Fatalf("unblock: %d", code)
		}
		if code := c.do(&carol, "POST", "/api/conversations/"+sum.ID+"/messages", map[string]any{"type": "text", "body": "open"}, nil); code != http.StatusCreated {
			t.Fatalf("send after unblock: %d", code)
		}
	})

	t.Run("bottle", func(t *testing.T) {
		// alice 扔瓶，carol 无瓶可捡前的校验先走一遍
		if code := c.do(&bob, "POST", "/api/bottles/pick", nil, nil); code != http.StatusNotFound {
			t.Fatalf("pick empty sea: %d", code)
		}
		var thrown models.Bottle
		if code := c.do(&alice, "POST", "/api/bottles", map[string]any{"content": "海上信"}, &thrown); code != http.StatusCreated || thrown.Status != "floating" {
			t.Fatalf("throw: %d %+v", code, thrown)
		}
		// bob 捡瓶
		pickRaw := map[string]any{}
		req, _ := http.NewRequest(http.MethodPost, c.base+"/api/bottles/pick", nil)
		req.Header.Set("Authorization", "Bearer "+bob.token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("pick: %d %s", resp.StatusCode, raw)
		}
		if err := json.Unmarshal(raw, &pickRaw); err != nil {
			t.Fatal(err)
		}
		// 匿名投影：响应 JSON 绝不含 sender_id 字段
		if strings.Contains(string(raw), "sender_id") {
			t.Fatalf("pick response leaks sender_id: %s", raw)
		}
		threadID := pickRaw["id"].(string)
		if pickRaw["my_alias"] != "捡瓶人" {
			t.Fatalf("picker alias: %v", pickRaw["my_alias"])
		}
		msgs0 := pickRaw["messages"].([]any)
		if len(msgs0) != 1 {
			t.Fatalf("initial messages: %v", msgs0)
		}
		m0 := msgs0[0].(map[string]any)
		if m0["alias"] != "扔瓶人" || m0["body"] != "海上信" || m0["mine"] != false {
			t.Fatalf("first message view: %v", m0)
		}
		// carol 无瓶可捡（瓶已被捡走）
		if code := c.do(&carol, "POST", "/api/bottles/pick", nil, nil); code != http.StatusNotFound {
			t.Fatalf("pick again: %d", code)
		}
		// bob 匿名回复
		var reply models.BottleMessageView
		if code := c.do(&bob, "POST", "/api/bottles/threads/"+threadID+"/messages", map[string]any{"body": "捡到啦"}, &reply); code != http.StatusCreated || !reply.Mine || reply.Alias != "捡瓶人" {
			t.Fatalf("bottle reply: %d %+v", code, reply)
		}
		// alice 视角
		var threadView models.BottleThreadView
		if code := c.do(&alice, "GET", "/api/bottles/threads/"+threadID, nil, &threadView); code != http.StatusOK || threadView.MyAlias != "扔瓶人" {
			t.Fatalf("alice thread: %d %+v", code, threadView)
		}
		if len(threadView.Messages) != 2 || threadView.Messages[1].Mine || threadView.Messages[1].Alias != "捡瓶人" {
			t.Fatalf("alice thread messages: %+v", threadView.Messages)
		}
		// 非成员不可见
		if code := c.do(&carol, "GET", "/api/bottles/threads/"+threadID, nil, nil); code != http.StatusForbidden {
			t.Fatalf("outsider thread: %d", code)
		}
		// 轮次：当前 1（bob 回复过），补满 6 轮
		for i := 0; i < 5; i++ {
			sender := bob
			if i%2 == 0 {
				sender = alice
			}
			if code := c.do(&sender, "POST", "/api/bottles/threads/"+threadID+"/messages", map[string]any{"body": "round"}, nil); code != http.StatusCreated {
				t.Fatalf("round %d: %d", i, code)
			}
		}
		if code := c.do(&alice, "POST", "/api/bottles/threads/"+threadID+"/messages", map[string]any{"body": "overflow"}, nil); code != http.StatusConflict {
			t.Fatalf("round cap: %d", code)
		}
		// 我的瓶子列表：含 conversation_id / unread
		var mineOut struct {
			Bottles []models.Bottle `json:"bottles"`
		}
		if code := c.do(&alice, "GET", "/api/bottles/mine", nil, &mineOut); code != http.StatusOK {
			t.Fatalf("my bottles: %d", code)
		}
		if len(mineOut.Bottles) == 0 {
			t.Fatalf("no bottles for alice")
		}
		found := false
		for _, b := range mineOut.Bottles {
			if b.ID == thrown.ID {
				found = b.ThreadID != nil && b.ConversationID != nil && b.Role == "thrower"
			}
		}
		if !found {
			t.Fatalf("bottle meta missing: %+v", mineOut.Bottles)
		}
		// reveal → 私聊会话
		var revOut struct {
			ConversationID string      `json:"conversation_id"`
			Peer           models.User `json:"peer"`
		}
		if code := c.do(&alice, "POST", "/api/bottles/threads/"+threadID+"/reveal", nil, &revOut); code != http.StatusOK || revOut.Peer.ID != bob.id {
			t.Fatalf("reveal: %d %+v", code, revOut)
		}
		// 幂等 reveal
		var revOut2 struct {
			ConversationID string      `json:"conversation_id"`
			Peer           models.User `json:"peer"`
		}
		if code := c.do(&bob, "POST", "/api/bottles/threads/"+threadID+"/reveal", nil, &revOut2); code != http.StatusOK || revOut2.ConversationID != revOut.ConversationID {
			t.Fatalf("idempotent reveal: %d %+v", code, revOut2)
		}
		// 会话列表出现该私聊（bottle 型不出现在列表）
		var aliceConvs struct {
			Conversations []models.ConversationSummary `json:"conversations"`
		}
		if code := c.do(&alice, "GET", "/api/conversations", nil, &aliceConvs); code != http.StatusOK {
			t.Fatalf("alice convs: %d", code)
		}
		var hasRevealConv, onlyChatTypes = false, true
		for _, cv := range aliceConvs.Conversations {
			if cv.Type != "single" && cv.Type != "group" {
				onlyChatTypes = false
			}
			if cv.ID == revOut.ConversationID && cv.Peer != nil && cv.Peer.ID == bob.id {
				hasRevealConv = true
			}
		}
		if !hasRevealConv || !onlyChatTypes {
			t.Fatalf("reveal conv missing or bottle leaked: %+v", aliceConvs.Conversations)
		}
		// reveal 后匿名消息 → 409 thread closed
		if code := c.do(&bob, "POST", "/api/bottles/threads/"+threadID+"/messages", map[string]any{"body": "late"}, nil); code != http.StatusConflict {
			t.Fatalf("send after reveal: %d", code)
		}
	})

	t.Run("websocket", func(t *testing.T) {
		// alice 与 bob 各开一条 WS
		aliceWS := wsDial(t, c.base, alice.token)
		defer aliceWS.Close()
		bobWS := wsDial(t, c.base, bob.token)
		defer bobWS.Close()
		// bob 上线广播对 alice 可见
		f := readUntil(t, aliceWS, "presence.change", func(m map[string]any) bool {
			return m["user_id"] == bob.id && m["online"] == true
		})
		if f == nil {
			t.Fatal("no presence for bob online")
		}
		// 用 alice-bob 既有私聊会话走 message.send 路径
		var sum models.ConversationSummary
		if code := c.do(&alice, "POST", "/api/conversations", map[string]any{"type": "single", "user_id": bob.id}, &sum); code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("ws single conv: %d", code)
		}
		if sum.Peer == nil || sum.Peer.ID != bob.id {
			t.Fatalf("ws single peer: %+v", sum)
		}
		sendFrame := map[string]any{
			"type": "message.send", "client_msg_id": "cmid-1",
			"conversation_id": sum.ID, "msg_type": "text", "body": "ws hello",
		}
		if err := aliceWS.WriteJSON(sendFrame); err != nil {
			t.Fatal(err)
		}
		ack := readUntil(t, aliceWS, "message.ack", nil)
		if ack["client_msg_id"] != "cmid-1" {
			t.Fatalf("ack: %v", ack)
		}
		ackMsg := ack["message"].(map[string]any)
		if ackMsg["body"] != "ws hello" || ackMsg["sender_id"] != alice.id {
			t.Fatalf("ack message: %v", ackMsg)
		}
		newF := readUntil(t, bobWS, "message.new", nil)
		newMsg := newF["message"].(map[string]any)
		if newMsg["id"] != ackMsg["id"] || newMsg["conversation_id"] != sum.ID {
			t.Fatalf("message.new: %v", newMsg)
		}
		// bob 断线 → alice 收到 presence.change 下线
		_ = bobWS.Close()
		readUntil(t, aliceWS, "presence.change", func(m map[string]any) bool {
			return m["user_id"] == bob.id && m["online"] == false
		})
		// 服务端心跳：ping → pong
		if err := aliceWS.WriteJSON(map[string]string{"type": "ping"}); err != nil {
			t.Fatal(err)
		}
		readUntil(t, aliceWS, "pong", nil)
	})
}
