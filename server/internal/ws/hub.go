package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"ripple/server/internal/auth"
	"ripple/server/internal/models"
	"ripple/server/internal/store"

	"github.com/gorilla/websocket"
)

// client 单条 WS 连接；mu 保护写（gorilla 并发写会 panic）
type client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *client) write(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.WriteMessage(websocket.TextMessage, data)
}

type Hub struct {
	mu    sync.RWMutex
	conns map[string]map[*client]struct{}
	Auth  *auth.Service
	Store *store.Store
}

func NewHub(a *auth.Service, s *store.Store) *Hub {
	return &Hub{
		conns: make(map[string]map[*client]struct{}),
		Auth:  a,
		Store: s,
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// readTimeout 服务端心跳超时：任意客户端帧续期
const readTimeout = 70 * time.Second

type clientFrame struct {
	Type            string `json:"type"`
	ClientMsgID     string `json:"client_msg_id"`
	ConversationID  string `json:"conversation_id"`
	MsgType         string `json:"msg_type"`
	Body            string `json:"body"`
	ImageURL        string `json:"image_url"`
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	uid, err := h.Auth.Parse(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{conn: conn}
	if h.add(uid, c) {
		h.markOnline(uid, true)
	}
	defer func() {
		if h.remove(uid, c) {
			h.markOnline(uid, false)
		}
		_ = conn.Close()
	}()

	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		var msg clientFrame
		if err := json.Unmarshal(data, &msg); err != nil {
			c.write(map[string]string{"type": "error", "message": "invalid json"})
			continue
		}
		switch msg.Type {
		case "ping":
			c.write(map[string]string{"type": "pong"})
		case "message.send":
			h.handleMessageSend(uid, c, msg)
		default:
			c.write(map[string]string{"type": "error", "message": "unknown type"})
		}
	}
}

// add 返回是否为该用户首个连接（0→1）
func (h *Hub) add(uid string, c *client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	first := len(h.conns[uid]) == 0
	if h.conns[uid] == nil {
		h.conns[uid] = make(map[*client]struct{})
	}
	h.conns[uid][c] = struct{}{}
	return first
}

// remove 返回是否为该用户最后一个连接（→0）
func (h *Hub) remove(uid string, c *client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	m := h.conns[uid]
	if m == nil {
		return false
	}
	delete(m, c)
	if len(m) > 0 {
		return false
	}
	delete(h.conns, uid)
	return true
}

// markOnline presence 变化：更新 users.status 并向全体在线用户广播
func (h *Hub) markOnline(uid string, online bool) {
	status := "offline"
	if online {
		status = "online"
	}
	if _, err := h.Store.DB.Exec(`UPDATE users SET status = ? WHERE id = ?`, status, uid); err != nil {
		log.Println("presence update:", err)
	}
	h.Broadcast(map[string]any{"type": "presence.change", "user_id": uid, "online": online})
}

func (h *Hub) SendToUser(uid string, v any) {
	h.sendToClients(uid, nil, v)
}

func (h *Hub) sendToClients(uid string, except *client, v any) {
	h.mu.RLock()
	targets := make([]*client, 0, len(h.conns[uid]))
	for c := range h.conns[uid] {
		if c != except {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range targets {
		c.write(v)
	}
}

// Broadcast 广播给全体在线用户
func (h *Hub) Broadcast(v any) {
	h.mu.RLock()
	targets := make([]*client, 0)
	for _, m := range h.conns {
		for c := range m {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range targets {
		c.write(v)
	}
}

func (h *Hub) handleMessageSend(uid string, c *client, msg clientFrame) {
	mt := msg.MsgType
	if mt == "" {
		mt = "text"
	}
	var img *string
	if msg.ImageURL != "" {
		v := msg.ImageURL
		img = &v
	}
	m, err := h.Store.InsertMessage(msg.ConversationID, uid, mt, msg.Body, img)
	if err != nil {
		c.write(map[string]any{"type": "error", "client_msg_id": msg.ClientMsgID, "message": err.Error()})
		return
	}
	c.write(map[string]any{"type": "message.ack", "client_msg_id": msg.ClientMsgID, "message": m})
	h.notifyMessageNew(msg.ConversationID, m, c)
}

// notifyMessageNew 全体成员收 message.new；发送者仅其它端（发送连接已收 ack）
func (h *Hub) notifyMessageNew(convID string, m models.Message, senderConn *client) {
	members, err := h.Store.MemberIDs(convID)
	if err != nil {
		log.Println("message.new members:", err)
		return
	}
	frame := map[string]any{"type": "message.new", "message": m}
	for _, id := range members {
		if id == m.SenderID {
			h.sendToClients(id, senderConn, frame)
		} else {
			h.SendToUser(id, frame)
		}
	}
}

// ---- 供 api 层调用的通知方法 ----

// NotifyMessageNew REST 发消息后通知全体成员（发送者含全部端）
func (h *Hub) NotifyMessageNew(convID string, m models.Message) {
	h.notifyMessageNew(convID, m, nil)
}

func (h *Hub) NotifyRevoked(members []string, conversationID, messageID string, revokedAt int64) {
	frame := map[string]any{
		"type": "message.revoked", "conversation_id": conversationID,
		"message_id": messageID, "revoked_at": revokedAt,
	}
	for _, id := range members {
		h.SendToUser(id, frame)
	}
}

func (h *Hub) NotifyRead(members []string, conversationID, userID string, seq int64) {
	frame := map[string]any{
		"type": "message.read", "conversation_id": conversationID,
		"user_id": userID, "seq": seq,
	}
	for _, id := range members {
		if id != userID {
			h.SendToUser(id, frame)
		}
	}
}

func (h *Hub) NotifyFriendRequest(recipientID, action string, req models.FriendRequest) {
	h.SendToUser(recipientID, map[string]any{
		"type": "friend.request", "action": action, "request": req,
	})
}

func (h *Hub) NotifyConversationUpdated(members []string, conversationID, event, actorID string, userIDs []string) {
	frame := map[string]any{
		"type": "conversation.updated", "conversation_id": conversationID,
		"event": event, "actor_id": actorID,
	}
	if userIDs != nil {
		frame["user_ids"] = userIDs
	}
	for _, id := range members {
		h.SendToUser(id, frame)
	}
}

func (h *Hub) NotifyBottleMessage(userID, threadID string, msg models.BottleMessageView) {
	h.SendToUser(userID, map[string]any{
		"type": "bottle.message", "thread_id": threadID, "message": msg,
	})
}

func (h *Hub) NotifyReveal(threadID, convID string, actorID, peerID string, actor, peer models.User) {
	h.SendToUser(actorID, map[string]any{
		"type": "bottle.revealed", "thread_id": threadID, "conversation_id": convID, "peer": peer,
	})
	h.SendToUser(peerID, map[string]any{
		"type": "bottle.revealed", "thread_id": threadID, "conversation_id": convID, "peer": actor,
	})
}
