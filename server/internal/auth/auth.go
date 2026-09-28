package auth

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ripple/server/internal/db"
	"ripple/server/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCreds = errors.New("invalid credentials")
	ErrValidation   = errors.New("validation failed")
	ErrConflict     = errors.New("conflict")
)

var usernameRE = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)

type Service struct {
	DB     *sql.DB
	Secret []byte
}

type scanner interface {
	Scan(dest ...any) error
}

const userCols = `id, username, display_name, avatar_color, avatar_url, signature, status`

func scanUser(row scanner, u *models.User) error {
	var avatar sql.NullString
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarColor, &avatar, &u.Signature, &u.Status); err != nil {
		return err
	}
	if avatar.Valid {
		v := avatar.String
		u.AvatarURL = &v
	}
	return nil
}

func (s *Service) issueToken(uid string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": uid,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	return token.SignedString(s.Secret)
}

func (s *Service) Login(username, password string) (string, models.User, error) {
	var u models.User
	var hash string
	var avatar sql.NullString
	err := s.DB.QueryRow(
		`SELECT `+userCols+`, password_hash FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarColor, &avatar, &u.Signature, &u.Status, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", models.User{}, ErrInvalidCreds
	}
	if err != nil {
		return "", models.User{}, err
	}
	if avatar.Valid {
		v := avatar.String
		u.AvatarURL = &v
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", models.User{}, ErrInvalidCreds
	}
	signed, err := s.issueToken(u.ID)
	if err != nil {
		return "", models.User{}, err
	}
	return signed, u, nil
}

func (s *Service) Register(username, password, displayName string) (string, models.User, error) {
	if !usernameRE.MatchString(username) || len(password) < 6 {
		return "", models.User{}, ErrValidation
	}
	if displayName == "" {
		displayName = username
	}
	if len([]rune(displayName)) > 32 {
		return "", models.User{}, ErrValidation
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", models.User{}, err
	}
	u := models.User{
		ID:          uuid.NewString(),
		Username:     username,
		DisplayName: displayName,
		AvatarColor: "#0d9488",
		Status:      "offline",
	}
	ts := time.Now().UnixMilli()
	_, err = s.DB.Exec(
		`INSERT INTO users (id, username, display_name, password_hash, avatar_color, signature, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', ?, ?, ?)`,
		u.ID, u.Username, u.DisplayName, string(hash), u.AvatarColor, u.Status, ts, ts,
	)
	if db.IsDuplicate(err) {
		return "", models.User{}, ErrConflict
	}
	if err != nil {
		return "", models.User{}, err
	}
	signed, err := s.issueToken(u.ID)
	if err != nil {
		return "", models.User{}, err
	}
	return signed, u, nil
}

func (s *Service) UserByID(id string) (models.User, error) {
	var u models.User
	err := scanUser(s.DB.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id), &u)
	return u, err
}

func (s *Service) UpdateProfile(uid, displayName, signature string) (models.User, error) {
	u, err := s.UserByID(uid)
	if err != nil {
		return models.User{}, err
	}
	if displayName != "" {
		if len([]rune(displayName)) > 32 {
			return models.User{}, ErrValidation
		}
		u.DisplayName = displayName
	}
	if len([]rune(signature)) > 128 {
		return models.User{}, ErrValidation
	}
	u.Signature = signature
	_, err = s.DB.Exec(
		`UPDATE users SET display_name = ?, signature = ?, updated_at = ? WHERE id = ?`,
		u.DisplayName, u.Signature, time.Now().UnixMilli(), uid,
	)
	if err != nil {
		return models.User{}, err
	}
	return u, nil
}

func (s *Service) SetAvatarURL(uid, url string) error {
	_, err := s.DB.Exec(
		`UPDATE users SET avatar_url = ?, updated_at = ? WHERE id = ?`,
		url, time.Now().UnixMilli(), uid,
	)
	return err
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
