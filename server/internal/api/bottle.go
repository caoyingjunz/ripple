package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ripple/server/internal/auth"
	"ripple/server/internal/models"
	"ripple/server/internal/store"

	"github.com/gorilla/mux"
)

func (s *Server) throwBottle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	b, err := s.Store.ThrowBottle(auth.UserID(r.Context()), strings.TrimSpace(body.Content))
	if errors.Is(err, store.ErrValidation) {
		writeErr(w, http.StatusBadRequest, "content required")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, b)
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
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) myBottles(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListMyBottles(auth.UserID(r.Context()))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []models.Bottle{}
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
	m, peer, err := s.Store.AddBottleMessage(tid, uid, strings.TrimSpace(body.Body))
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
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrValidation) {
		writeErr(w, http.StatusBadRequest, "body required")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 双端各收自己视角的匿名投影
	theirs := m
	theirs.Mine = false
	s.Hub.NotifyBottleMessage(uid, tid, m)
	s.Hub.NotifyBottleMessage(peer, tid, theirs)
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) reveal(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	tid := mux.Vars(r)["id"]
	convID, peer, err := s.Store.Reveal(tid, uid)
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
	actor, err := s.Auth.UserByID(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 双端各收到对方视角的 peer
	s.Hub.NotifyReveal(tid, convID, uid, peer.ID, actor, peer)
	s.Hub.NotifyConversationUpdated([]string{uid, peer.ID}, convID, "created", uid, nil)
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
