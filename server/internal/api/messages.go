package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"ripple/server/internal/auth"
	"ripple/server/internal/models"
	"ripple/server/internal/store"

	"github.com/gorilla/mux"
)

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	convID := mux.Vars(r)["id"]
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	var after, before *int64
	if v := q.Get("after_seq"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			after = &n
		}
	}
	if v := q.Get("before_seq"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			before = &n
		}
	}
	msgs, hasMore, err := s.Store.ListMessages(convID, auth.UserID(r.Context()), after, before, limit)
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
	if msgs == nil {
		msgs = []models.Message{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": msgs, "has_more": hasMore})
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	convID := mux.Vars(r)["id"]
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
	m, err := s.Store.InsertMessage(convID, uid, body.Type, body.Body, body.ImageURL)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "conversation not found")
		return
	}
	if errors.Is(err, store.ErrForbidden) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if errors.Is(err, store.ErrValidation) {
		writeErr(w, http.StatusBadRequest, "invalid message")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 全体成员（含发送者其它端）收 message.new
	s.Hub.NotifyMessageNew(convID, m)
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) revokeMessage(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	m, err := s.Store.RevokeMessage(mux.Vars(r)["id"], uid)
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
	if m.RevokedAt != nil {
		if members, err := s.Store.MemberIDs(m.ConversationID); err == nil {
			s.Hub.NotifyRevoked(members, m.ConversationID, m.ID, *m.RevokedAt)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": m})
}
