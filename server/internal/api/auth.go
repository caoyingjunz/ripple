package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"ripple/server/internal/auth"
	"ripple/server/internal/models"
)

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

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	token, user, err := s.Auth.Register(body.Username, body.Password, body.DisplayName)
	if errors.Is(err, auth.ErrValidation) {
		writeErr(w, http.StatusBadRequest, "invalid username/password/display_name")
		return
	}
	if errors.Is(err, auth.ErrConflict) {
		writeErr(w, http.StatusConflict, "username already exists")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "user": user})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, err := s.Auth.UserByID(auth.UserID(r.Context()))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName string `json:"display_name"`
		Signature   string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	u, err := s.Auth.UpdateProfile(auth.UserID(r.Context()), body.DisplayName, body.Signature)
	if errors.Is(err, auth.ErrValidation) {
		writeErr(w, http.StatusBadRequest, "invalid display_name/signature")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) updateAvatar(w http.ResponseWriter, r *http.Request) {
	url, ok := s.saveImage(w, r)
	if !ok {
		return
	}
	if err := s.Auth.SetAvatarURL(auth.UserID(r.Context()), url); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"avatar_url": url})
}

func (s *Server) searchUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"users": []any{}})
		return
	}
	users, err := s.Store.SearchUsers(auth.UserID(r.Context()), q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if users == nil {
		users = []models.SearchEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}
