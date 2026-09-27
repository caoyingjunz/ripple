package auth

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"ripple/server/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCreds = errors.New("invalid credentials")

type Service struct {
	DB     *sql.DB
	Secret []byte
}

func (s *Service) Login(username, password string) (string, models.User, error) {
	var u models.User
	var hash string
	err := s.DB.QueryRow(
		`SELECT id, username, display_name, avatar_color, password_hash FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarColor, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", models.User{}, ErrInvalidCreds
	}
	if err != nil {
		return "", models.User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", models.User{}, ErrInvalidCreds
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": u.ID,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	signed, err := token.SignedString(s.Secret)
	if err != nil {
		return "", models.User{}, err
	}
	return signed, u, nil
}

func (s *Service) Parse(tokenStr string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return s.Secret, nil
	})
	if err != nil || !token.Valid {
		return "", ErrInvalidCreds
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", ErrInvalidCreds
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", ErrInvalidCreds
	}
	return sub, nil
}

func (s *Service) UserByID(id string) (models.User, error) {
	var u models.User
	err := s.DB.QueryRow(
		`SELECT id, username, display_name, avatar_color FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarColor)
	return u, err
}

type ctxKey string

const UserIDKey ctxKey = "userID"

func Middleware(s *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			uid, err := s.Parse(strings.TrimPrefix(h, "Bearer "))
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), uid)))
		})
	}
}
