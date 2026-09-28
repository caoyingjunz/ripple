package api

import (
	"encoding/json"
	"net/http"

	"ripple/server/internal/auth"
	"ripple/server/internal/config"
	"ripple/server/internal/store"
	"ripple/server/internal/ws"

	"github.com/gorilla/mux"
)

type Server struct {
	Cfg   config.Config
	Auth  *auth.Service
	Store *store.Store
	Hub   *ws.Hub
}

func New(cfg config.Config, a *auth.Service, s *store.Store, h *ws.Hub) http.Handler {
	api := &Server{Cfg: cfg, Auth: a, Store: s, Hub: h}
	r := mux.NewRouter()

	r.HandleFunc("/api/auth/login", api.login).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/register", api.register).Methods(http.MethodPost)
	r.Handle("/api/ws", h)

	authR := r.PathPrefix("/api").Subrouter()
	authR.Use(auth.Middleware(a))
	authR.HandleFunc("/me", api.me).Methods(http.MethodGet)
	authR.HandleFunc("/me", api.updateMe).Methods(http.MethodPut)
	authR.HandleFunc("/me/avatar", api.updateAvatar).Methods(http.MethodPost)
	authR.HandleFunc("/users/search", api.searchUsers).Methods(http.MethodGet)
	authR.HandleFunc("/friends", api.listFriends).Methods(http.MethodGet)
	authR.HandleFunc("/friends/requests", api.addFriend).Methods(http.MethodPost)
	authR.HandleFunc("/friends/requests/{id}/accept", api.acceptFriend).Methods(http.MethodPost)
	authR.HandleFunc("/friends/requests/{id}/decline", api.declineFriend).Methods(http.MethodPost)
	authR.HandleFunc("/friends/{user_id}", api.removeFriend).Methods(http.MethodDelete)
	authR.HandleFunc("/friends/{user_id}/block", api.blockFriend).Methods(http.MethodPost)
	authR.HandleFunc("/friends/{user_id}/block", api.unblockFriend).Methods(http.MethodDelete)
	authR.HandleFunc("/conversations", api.listConversations).Methods(http.MethodGet)
	authR.HandleFunc("/conversations", api.createConversation).Methods(http.MethodPost)
	authR.HandleFunc("/conversations/{id}", api.getConversation).Methods(http.MethodGet)
	authR.HandleFunc("/conversations/{id}", api.renameConversation).Methods(http.MethodPut)
	authR.HandleFunc("/conversations/{id}/members", api.addMembers).Methods(http.MethodPost)
	authR.HandleFunc("/conversations/{id}/members/{uid}", api.removeMember).Methods(http.MethodDelete)
	authR.HandleFunc("/conversations/{id}/leave", api.leaveConversation).Methods(http.MethodPost)
	authR.HandleFunc("/conversations/{id}/messages", api.listMessages).Methods(http.MethodGet)
	authR.HandleFunc("/conversations/{id}/messages", api.sendMessage).Methods(http.MethodPost)
	authR.HandleFunc("/conversations/{id}/read", api.readConversation).Methods(http.MethodPost)
	authR.HandleFunc("/messages/{id}/revoke", api.revokeMessage).Methods(http.MethodPost)
	authR.HandleFunc("/media/upload", api.upload).Methods(http.MethodPost)
	authR.HandleFunc("/bottles", api.throwBottle).Methods(http.MethodPost)
	authR.HandleFunc("/bottles/pick", api.pickBottle).Methods(http.MethodPost)
	authR.HandleFunc("/bottles/mine", api.myBottles).Methods(http.MethodGet)
	authR.HandleFunc("/bottles/threads/{id}", api.getThread).Methods(http.MethodGet)
	authR.HandleFunc("/bottles/threads/{id}/messages", api.bottleMessage).Methods(http.MethodPost)
	authR.HandleFunc("/bottles/threads/{id}/reveal", api.reveal).Methods(http.MethodPost)
	authR.HandleFunc("/bottles/threads/{id}/close", api.closeThread).Methods(http.MethodPost)

	r.PathPrefix("/uploads/").Handler(http.StripPrefix("/uploads/", http.FileServer(http.Dir(cfg.UploadDir))))

	return cors(r)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
