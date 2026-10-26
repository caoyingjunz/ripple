package api

import (
	"encoding/json"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

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

	// H5：托管 web/dist（须注册在 /api、/uploads 之后兜底）。目录缺失则只提供 API（不影响桌面端）。
	if st, err := os.Stat(cfg.WebDir); err == nil && st.IsDir() {
		r.PathPrefix("/").Handler(spaHandler(cfg.WebDir))
		log.Printf("h5 static serving enabled: %s", cfg.WebDir)
	} else {
		log.Printf("h5 static serving disabled: %s not found (run npm run build:web)", cfg.WebDir)
	}

	return cors(r)
}

// spaHandler 静态托管 web/dist 并做 SPA 回退：文件命中直出；未命中（BrowserRouter
// 深链/刷新）回落 index.html——与 electron/main.js 的 ripple://local/ 行为保持一致。
func spaHandler(dir string) http.Handler {
	// .webmanifest 在部分系统 mime 表中缺失，注册以保证 PWA 安装识别
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		// API 未命中路由：返回 404 JSON，绝不能回落 index.html
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		p := filepath.Join(dir, filepath.FromSlash(path.Clean("/"+r.URL.Path)))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
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
