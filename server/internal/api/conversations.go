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

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListConversations(auth.UserID(r.Context()))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []models.ConversationSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": list})
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type      string   `json:"type"`
		UserID    string   `json:"user_id"`
		Name      string   `json:"name"`
		MemberIDs []string `json:"member_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	uid := auth.UserID(r.Context())
	switch body.Type {
	case "single":
		if _, err := s.Auth.UserByID(body.UserID); err != nil {
			writeErr(w, http.StatusNotFound, "user not found")
			return
		}
		convID, created, err := s.Store.GetOrCreateSingle(uid, body.UserID)
		if errors.Is(err, store.ErrValidation) {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		sum, err := s.Store.ConversationSummaryOf(convID, uid)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if created {
			members, err := s.Store.MemberIDs(convID)
			if err == nil {
				s.Hub.NotifyConversationUpdated(members, convID, "created", uid, nil)
			}
		}
		code := http.StatusOK
		if created {
			code = http.StatusCreated
		}
		writeJSON(w, code, sum)
	case "group":
		sum, err := s.Store.CreateGroup(uid, body.Name, body.MemberIDs)
		if errors.Is(err, store.ErrValidation) {
			writeErr(w, http.StatusBadRequest, "invalid name or member_ids")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "member not found")
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if members, err := s.Store.MemberIDs(sum.ID); err == nil {
			s.Hub.NotifyConversationUpdated(members, sum.ID, "created", uid, nil)
		}
		writeJSON(w, http.StatusCreated, sum)
	default:
		writeErr(w, http.StatusBadRequest, "invalid type")
	}
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	sum, members, err := s.Store.GetConversation(mux.Vars(r)["id"], auth.UserID(r.Context()))
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
	if members == nil {
		members = []models.MemberEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation": sum, "members": members})
}

func (s *Server) renameConversation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	convID := mux.Vars(r)["id"]
	uid := auth.UserID(r.Context())
	sum, err := s.Store.RenameGroup(convID, uid, body.Name)
	if errors.Is(err, store.ErrValidation) {
		writeErr(w, http.StatusBadRequest, "invalid name")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, "not a group")
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
	if members, err := s.Store.MemberIDs(convID); err == nil {
		s.Hub.NotifyConversationUpdated(members, convID, "renamed", uid, nil)
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) addMembers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	convID := mux.Vars(r)["id"]
	uid := auth.UserID(r.Context())
	added, err := s.Store.AddMembers(convID, uid, body.UserIDs)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, "not a group")
		return
	}
	if errors.Is(err, store.ErrForbidden) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if errors.Is(err, store.ErrValidation) {
		writeErr(w, http.StatusBadRequest, "too many user_ids")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(added) > 0 {
		ids := make([]string, len(added))
		for i, u := range added {
			ids[i] = u.ID
		}
		if members, err := s.Store.MemberIDs(convID); err == nil {
			s.Hub.NotifyConversationUpdated(members, convID, "members_added", uid, ids)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": added})
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	convID := mux.Vars(r)["id"]
	target := mux.Vars(r)["uid"]
	uid := auth.UserID(r.Context())
	err := s.Store.RemoveMember(convID, uid, target)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, "not a group")
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
	// 通知剩余成员与被移除者
	if members, err := s.Store.MemberIDs(convID); err == nil {
		targets := append(members, target)
		s.Hub.NotifyConversationUpdated(targets, convID, "member_removed", uid, []string{target})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) leaveConversation(w http.ResponseWriter, r *http.Request) {
	convID := mux.Vars(r)["id"]
	uid := auth.UserID(r.Context())
	if err := s.Store.LeaveGroup(convID, uid); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, http.StatusConflict, "not a group")
			return
		}
		if errors.Is(err, store.ErrForbidden) {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if members, err := s.Store.MemberIDs(convID); err == nil {
		s.Hub.NotifyConversationUpdated(members, convID, "left", uid, []string{uid})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "left"})
}

func (s *Server) readConversation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Seq int64 `json:"seq"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	convID := mux.Vars(r)["id"]
	uid := auth.UserID(r.Context())
	cur, err := s.Store.SetReadSeq(convID, uid, body.Seq)
	if errors.Is(err, store.ErrForbidden) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if members, err := s.Store.MemberIDs(convID); err == nil {
		s.Hub.NotifyRead(members, convID, uid, cur)
	}
	writeJSON(w, http.StatusOK, map[string]int64{"last_read_seq": cur})
}
