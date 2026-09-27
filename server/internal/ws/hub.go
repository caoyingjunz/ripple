package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"ripple/server/internal/auth"
	"ripple/server/internal/models"
	"ripple/server/internal/store"

	"github.com/gorilla/websocket"
)

type Hub struct {
	mu    sync.RWMutex
	conns map[string]map[*websocket.Conn]struct{}
	Auth  *auth.Service
	Store *store.Store
}

func NewHub(a *auth.Service, s *store.Store) *Hub {
	return &Hub{
		conns: make(map[string]map[*websocket.Conn]struct{}),
		Auth:  a,
		Store: s,
	}
}

func (h *Hub) add(userID string, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[userID] == nil {
		h.conns[userID] = make(map[*websocket.Conn]struct{})
	}
	h.conns[userID][c] = struct{}{}
}

func (h *Hub) remove(userID string, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m := h.conns[userID]; m != nil {
		delete(m, c)
		if len(m) == 0 {
			delete(h.conns, userID)
		}
	}
}

func (h *Hub) SendToUser(userID string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.conns[userID] {
		_ = c.WriteMessage(websocket.TextMessage, data)
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type clientMsg struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id"`
	ThreadID       string `json:"thread_id"`
	MsgType        string `json:"msg_type"`
	Body           string `json:"body"`
	ImageURL       string `json:"image_url"`
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
	h.add(uid, conn)
	defer func() {
		h.remove(uid, conn)
		_ = conn.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg clientMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			h.writeErr(conn, "invalid json")
			continue
		}
		switch msg.Type {
		case "ping":
			_ = conn.WriteJSON(map[string]string{"type": "pong"})
		case "chat.send":
			h.handleChat(uid, conn, msg)
		case "bottle.send":
			h.handleBottle(uid, conn, msg)
		default:
			h.writeErr(conn, "unknown type")
		}
	}
}

func (h *Hub) writeErr(c *websocket.Conn, m string) {
	_ = c.WriteJSON(map[string]string{"type": "error", "message": m})
}

func (h *Hub) handleChat(uid string, conn *websocket.Conn, msg clientMsg) {
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
		h.writeErr(conn, err.Error())
		return
	}
	payload := map[string]any{"type": "chat.message", "message": m}
	h.SendToUser(uid, payload)
	if peer, err := h.Store.PeerID(msg.ConversationID, uid); err == nil {
		h.SendToUser(peer, payload)
	}
}

func (h *Hub) handleBottle(uid string, conn *websocket.Conn, msg clientMsg) {
	m, peer, _, err := h.Store.AddBottleMessage(msg.ThreadID, uid, msg.Body)
	if err != nil {
		h.writeErr(conn, err.Error())
		return
	}
	mine := m
	mine.Mine = true
	theirs := m
	theirs.Mine = false
	h.SendToUser(uid, map[string]any{"type": "bottle.message", "thread_id": msg.ThreadID, "message": mine})
	h.SendToUser(peer, map[string]any{"type": "bottle.message", "thread_id": msg.ThreadID, "message": theirs})
	_ = models.BottleMessage{}
}

func (h *Hub) NotifyReveal(threadID, convID, actorID, peerID string, peer models.User) {
	payloadActor := map[string]any{
		"type": "bottle.revealed", "thread_id": threadID, "conversation_id": convID, "peer": peer,
	}
	actor, err := h.Auth.UserByID(actorID)
	if err != nil {
		log.Println("reveal actor", err)
		return
	}
	payloadPeer := map[string]any{
		"type": "bottle.revealed", "thread_id": threadID, "conversation_id": convID, "peer": actor,
	}
	h.SendToUser(actorID, payloadActor)
	h.SendToUser(peerID, payloadPeer)
}
