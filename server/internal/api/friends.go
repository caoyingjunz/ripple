package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"ripple/server/internal/auth"
	"ripple/server/internal/models"
	"ripple/server/internal/store"

	"github.com/gorilla/mux"
)

func (s *Server) addFriend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	uid := auth.UserID(r.Context())
	rowID, createdAt, friend, status, err := s.Store.AddFriend(uid, body.UserID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	if errors.Is(err, store.ErrValidation) {
		writeErr(w, http.StatusBadRequest, "cannot add yourself")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, "cannot send friend request")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	me, err := s.Auth.UserByID(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// pending→发给收件人；accepted（反向秒通过）→发给原申请人；两处收件人都是对方
	req := models.FriendRequest{ID: rowID, User: me, CreatedAt: createdAt}
	action := "pending"
	code := http.StatusCreated
	if status == "accepted" {
		action = "accepted"
		code = http.StatusOK
	}
	s.Hub.NotifyFriendRequest(body.UserID, action, req)
	writeJSON(w, code, map[string]any{"status": status, "friend": friend})
}

func (s *Server) acceptFriend(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	id := mux.Vars(r)["id"]
	friend, createdAt, from, err := s.Store.AcceptRequest(id, uid)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrForbidden) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, "request already handled")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	me, err := s.Auth.UserByID(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 通知原申请人：request.user=新好友（即当前用户）
	s.Hub.NotifyFriendRequest(from, "accepted", models.FriendRequest{ID: id, User: me, CreatedAt: createdAt})
	writeJSON(w, http.StatusOK, map[string]any{"friend": friend})
}

func (s *Server) declineFriend(w http.ResponseWriter, r *http.Request) {
	err := s.Store.DeclineRequest(mux.Vars(r)["id"], auth.UserID(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrForbidden) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, "request already handled")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "declined"})
}

func (s *Server) removeFriend(w http.ResponseWriter, r *http.Request) {
	err := s.Store.RemoveFriend(auth.UserID(r.Context()), mux.Vars(r)["user_id"])
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) blockFriend(w http.ResponseWriter, r *http.Request) {
	err := s.Store.BlockUser(auth.UserID(r.Context()), mux.Vars(r)["user_id"])
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, "already blocked")
		return
	}
	if errors.Is(err, store.ErrValidation) {
		writeErr(w, http.StatusBadRequest, "cannot block yourself")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "blocked"})
}

func (s *Server) unblockFriend(w http.ResponseWriter, r *http.Request) {
	err := s.Store.UnblockUser(auth.UserID(r.Context()), mux.Vars(r)["user_id"])
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unblocked"})
}

func (s *Server) listFriends(w http.ResponseWriter, r *http.Request) {
	view, err := s.Store.ListFriends(auth.UserID(r.Context()))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if view.Friends == nil {
		view.Friends = []models.User{}
	}
	if view.PendingIn == nil {
		view.PendingIn = []models.FriendRequest{}
	}
	if view.PendingOut == nil {
		view.PendingOut = []models.FriendRequest{}
	}
	if view.Blocked == nil {
		view.Blocked = []models.User{}
	}
	writeJSON(w, http.StatusOK, view)
}
