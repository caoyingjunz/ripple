package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"ripple/server/internal/models"

	"github.com/google/uuid"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrMaxRounds    = errors.New("max rounds reached")
	ErrThreadClosed = errors.New("thread closed")
)

type Store struct {
	DB        *sql.DB
	MaxRounds int
}

func New(db *sql.DB, maxRounds int) *Store {
	return &Store{DB: db, MaxRounds: maxRounds}
}

func nowMS() int64 { return time.Now().UnixMilli() }

func orderedPair(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

func (s *Store) GetOrCreateConversation(userA, userB string) (string, error) {
	if userA == userB {
		return "", fmt.Errorf("cannot chat with self")
	}
	a, b := orderedPair(userA, userB)
	var id string
	err := s.DB.QueryRow(
		`SELECT id FROM conversations WHERE user_a_id = ? AND user_b_id = ?`, a, b,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	id = uuid.NewString()
	_, err = s.DB.Exec(
		`INSERT INTO conversations (id, user_a_id, user_b_id, created_at) VALUES (?, ?, ?, ?)`,
		id, a, b, nowMS(),
	)
	if err != nil {
		// race: fetch again
		if e2 := s.DB.QueryRow(
			`SELECT id FROM conversations WHERE user_a_id = ? AND user_b_id = ?`, a, b,
		).Scan(&id); e2 == nil {
			return id, nil
		}
		return "", err
	}
	return id, nil
}

func (s *Store) IsMember(conversationID, userID string) (bool, error) {
	var n int
	err := s.DB.QueryRow(
		`SELECT COUNT(*) FROM conversations WHERE id = ? AND (user_a_id = ? OR user_b_id = ?)`,
		conversationID, userID, userID,
	).Scan(&n)
	return n > 0, err
}

func (s *Store) ListConversations(userID string) ([]models.Conversation, error) {
	rows, err := s.DB.Query(`
SELECT c.id, c.created_at, c.last_message_at,
  u.id, u.username, u.display_name, u.avatar_color,
  (SELECT m.body FROM messages m WHERE m.conversation_id = c.id ORDER BY m.created_at DESC LIMIT 1)
FROM conversations c
JOIN users u ON u.id = CASE WHEN c.user_a_id = ? THEN c.user_b_id ELSE c.user_a_id END
WHERE c.user_a_id = ? OR c.user_b_id = ?
ORDER BY COALESCE(c.last_message_at, c.created_at) DESC
`, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Conversation
	for rows.Next() {
		var c models.Conversation
		var lastAt sql.NullInt64
		var last sql.NullString
		if err := rows.Scan(
			&c.ID, &c.CreatedAt, &lastAt,
			&c.Peer.ID, &c.Peer.Username, &c.Peer.DisplayName, &c.Peer.AvatarColor,
			&last,
		); err != nil {
			return nil, err
		}
		if lastAt.Valid {
			v := lastAt.Int64
			c.LastMessageAt = &v
		}
		if last.Valid {
			v := last.String
			c.LastMessage = &v
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ListMessages(conversationID, userID string, limit int) ([]models.Message, error) {
	ok, err := s.IsMember(conversationID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.DB.Query(`
SELECT id, conversation_id, sender_id, type, body, image_url, created_at
FROM messages WHERE conversation_id = ?
ORDER BY created_at ASC LIMIT ?
`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Message
	for rows.Next() {
		var m models.Message
		var img sql.NullString
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Type, &m.Body, &img, &m.CreatedAt); err != nil {
			return nil, err
		}
		if img.Valid {
			v := img.String
			m.ImageURL = &v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) InsertMessage(conversationID, senderID, msgType, body string, imageURL *string) (models.Message, error) {
	ok, err := s.IsMember(conversationID, senderID)
	if err != nil {
		return models.Message{}, err
	}
	if !ok {
		return models.Message{}, ErrForbidden
	}
	if msgType != "text" && msgType != "image" {
		return models.Message{}, fmt.Errorf("invalid type")
	}
	m := models.Message{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		SenderID:       senderID,
		Type:           msgType,
		Body:           body,
		ImageURL:       imageURL,
		CreatedAt:      nowMS(),
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return models.Message{}, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(
		`INSERT INTO messages (id, conversation_id, sender_id, type, body, image_url, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ConversationID, m.SenderID, m.Type, m.Body, m.ImageURL, m.CreatedAt,
	)
	if err != nil {
		return models.Message{}, err
	}
	preview := body
	if msgType == "image" {
		preview = "[图片]"
	}
	_, err = tx.Exec(
		`UPDATE conversations SET last_message_at = ? WHERE id = ?`,
		m.CreatedAt, conversationID,
	)
	if err != nil {
		return models.Message{}, err
	}
	_ = preview
	if err := tx.Commit(); err != nil {
		return models.Message{}, err
	}
	return m, nil
}

func (s *Store) PeerID(conversationID, userID string) (string, error) {
	var a, b string
	err := s.DB.QueryRow(`SELECT user_a_id, user_b_id FROM conversations WHERE id = ?`, conversationID).Scan(&a, &b)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if a == userID {
		return b, nil
	}
	if b == userID {
		return a, nil
	}
	return "", ErrForbidden
}
