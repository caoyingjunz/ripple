package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ripple/server/internal/auth"
	"ripple/server/internal/config"
	"ripple/server/internal/models"
	"ripple/server/internal/store"
	"ripple/server/internal/ws"

	"github.com/google/uuid"
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
	r.Handle("/api/ws", h)

	authR := r.PathPrefix("/api").Subrouter()
	authR.Use(auth.Middleware(a))
	authR.HandleFunc("/me", api.me).Methods(http.MethodGet)
	authR.HandleFunc("/conversations", api.listConversations).Methods(http.MethodGet)
	authR.HandleFunc("/conversations/{id}/messages", api.listMessages).Methods(http.MethodGet)
	authR.HandleFunc("/conversations/{id}/messages", api.sendMessage).Methods(http.MethodPost)
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	token, user, err := s.Auth.Login(body.Username, body.Password)
	if errors.Is(err, auth.ErrInvalidCreds) {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, err := s.Auth.UserByID(auth.UserID(r.Context()))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListConversations(auth.UserID(r.Context()))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []models.Conversation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": list})
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	msgs, err := s.Store.ListMessages(id, auth.UserID(r.Context()), 200)
	if errors.Is(err, store.ErrForbidden) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Type     string  `json:"type"`
		Body     string  `json:"body"`
		ImageURL *string `json:"image_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Type == "" {
		body.Type = "text"
	}
	uid := auth.UserID(r.Context())
	m, err := s.Store.InsertMessage(id, uid, body.Type, body.Body, body.ImageURL)
	if errors.Is(err, store.ErrForbidden) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	payload := map[string]any{"type": "chat.message", "message": m}
	s.Hub.SendToUser(uid, payload)
	if peer, err := s.Store.PeerID(id, uid); err == nil {
		s.Hub.SendToUser(peer, payload)
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.Cfg.MaxUpload+1024)
	if err := r.ParseMultipartForm(s.Cfg.MaxUpload); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "file too large")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	ctype := http.DetectContentType(buf[:n])
	ext := ""
	switch ctype {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	default:
		writeErr(w, http.StatusBadRequest, "unsupported image type")
		return
	}
	_ = hdr
	if err := os.MkdirAll(s.Cfg.UploadDir, 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	name := uuid.NewString() + ext
	path := filepath.Join(s.Cfg.UploadDir, name)
	out, err := os.Create(path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer out.Close()
	if _, err := out.Write(buf[:n]); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": "/uploads/" + name})
}

func (s *Server) throwBottle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	b, err := s.Store.ThrowBottle(auth.UserID(r.Context()), strings.TrimSpace(body.Content))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) pickBottle(w http.ResponseWriter, r *http.Request) {
	t, err := s.Store.PickBottle(auth.UserID(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "no bottles available")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, "bottle already picked")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) myBottles(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListMyBottles(auth.UserID(r.Context()))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bottles": list})
}

func (s *Server) getThread(w http.ResponseWriter, r *http.Request) {
	t, err := s.Store.GetThread(mux.Vars(r)["id"], auth.UserID(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrForbidden) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) bottleMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	uid := auth.UserID(r.Context())
	tid := mux.Vars(r)["id"]
	m, peer, _, err := s.Store.AddBottleMessage(tid, uid, strings.TrimSpace(body.Body))
	if errors.Is(err, store.ErrMaxRounds) {
		writeErr(w, http.StatusConflict, "max rounds reached")
		return
	}
	if errors.Is(err, store.ErrThreadClosed) {
		writeErr(w, http.StatusConflict, "thread closed")
		return
	}
	if errors.Is(err, store.ErrForbidden) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	mine := m
	mine.Mine = true
	theirs := m
	theirs.Mine = false
	s.Hub.SendToUser(uid, map[string]any{"type": "bottle.message", "thread_id": tid, "message": mine})
	s.Hub.SendToUser(peer, map[string]any{"type": "bottle.message", "thread_id": tid, "message": theirs})
	writeJSON(w, http.StatusOK, mine)
}

func (s *Server) reveal(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	tid := mux.Vars(r)["id"]
	convID, peerID, peer, err := s.Store.Reveal(tid, uid)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrForbidden) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if errors.Is(err, store.ErrThreadClosed) {
		writeErr(w, http.StatusConflict, "thread closed")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Hub.NotifyReveal(tid, convID, uid, peerID, peer)
	writeJSON(w, http.StatusOK, map[string]any{"conversation_id": convID, "peer": peer})
}

func (s *Server) closeThread(w http.ResponseWriter, r *http.Request) {
	err := s.Store.CloseThread(mux.Vars(r)["id"], auth.UserID(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrForbidden) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}
