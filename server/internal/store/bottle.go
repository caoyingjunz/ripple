package store

import (
	"database/sql"
	"errors"

	"ripple/server/internal/models"

	"github.com/google/uuid"
)

const (
	aliasThrower = "扔瓶人"
	aliasPicker  = "捡瓶人"
)

func (s *Store) ThrowBottle(userID, content string) (models.Bottle, error) {
	if content == "" {
		return models.Bottle{}, errors.New("content required")
	}
	b := models.Bottle{
		ID:        uuid.NewString(),
		Content:   content,
		Status:    "floating",
		CreatedAt: nowMS(),
		Role:      "thrower",
	}
	_, err := s.DB.Exec(
		`INSERT INTO bottles (id, thrower_id, content, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		b.ID, userID, content, b.Status, b.CreatedAt,
	)
	return b, err
}

func (s *Store) PickBottle(userID string) (models.BottleThread, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return models.BottleThread{}, err
	}
	defer tx.Rollback()

	var bottleID, content, throwerID string
	var createdAt int64
	err = tx.QueryRow(`
SELECT id, thrower_id, content, created_at FROM bottles
WHERE status = 'floating' AND thrower_id != ?
ORDER BY RANDOM() LIMIT 1
`, userID).Scan(&bottleID, &throwerID, &content, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.BottleThread{}, ErrNotFound
	}
	if err != nil {
		return models.BottleThread{}, err
	}

	res, err := tx.Exec(`
UPDATE bottles SET status = 'picked', picker_id = ?, picked_at = ?
WHERE id = ? AND status = 'floating'
`, userID, nowMS(), bottleID)
	if err != nil {
		return models.BottleThread{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.BottleThread{}, ErrConflict
	}

	threadID := uuid.NewString()
	ts := nowMS()
	_, err = tx.Exec(`
INSERT INTO bottle_threads (id, bottle_id, thrower_id, picker_id, round_count, max_rounds, revealed, closed, created_at)
VALUES (?, ?, ?, ?, 0, ?, 0, 0, ?)
`, threadID, bottleID, throwerID, userID, s.MaxRounds, ts)
	if err != nil {
		return models.BottleThread{}, err
	}

	msgID := uuid.NewString()
	_, err = tx.Exec(`
INSERT INTO bottle_messages (id, thread_id, sender_id, alias, body, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, msgID, threadID, throwerID, aliasThrower, content, createdAt)
	if err != nil {
		return models.BottleThread{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.BottleThread{}, err
	}
	return s.GetThread(threadID, userID)
}

func (s *Store) ListMyBottles(userID string) ([]models.Bottle, error) {
	rows, err := s.DB.Query(`
SELECT b.id, b.content, b.status, b.created_at, t.id,
  CASE WHEN b.thrower_id = ? THEN 'thrower' ELSE 'picker' END
FROM bottles b
LEFT JOIN bottle_threads t ON t.bottle_id = b.id
WHERE b.thrower_id = ? OR b.picker_id = ?
ORDER BY b.created_at DESC
`, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Bottle
	for rows.Next() {
		var b models.Bottle
		var tid sql.NullString
		if err := rows.Scan(&b.ID, &b.Content, &b.Status, &b.CreatedAt, &tid, &b.Role); err != nil {
			return nil, err
		}
		if tid.Valid {
			v := tid.String
			b.ThreadID = &v
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) threadRole(throwerID, pickerID, userID string) (alias string, ok bool) {
	if userID == throwerID {
		return aliasThrower, true
	}
	if userID == pickerID {
		return aliasPicker, true
	}
	return "", false
}

func (s *Store) GetThread(threadID, userID string) (models.BottleThread, error) {
	var t models.BottleThread
	var throwerID, pickerID, bottleID string
	var revealed, closed int
	var convID sql.NullString
	var content string
	err := s.DB.QueryRow(`
SELECT t.id, t.bottle_id, t.thrower_id, t.picker_id, t.round_count, t.max_rounds, t.revealed, t.conversation_id, t.closed, t.created_at, b.content
FROM bottle_threads t
JOIN bottles b ON b.id = t.bottle_id
WHERE t.id = ?
`, threadID).Scan(
		&t.ID, &bottleID, &throwerID, &pickerID, &t.RoundCount, &t.MaxRounds, &revealed, &convID, &closed, &t.CreatedAt, &content,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.BottleThread{}, ErrNotFound
	}
	if err != nil {
		return models.BottleThread{}, err
	}
	alias, ok := s.threadRole(throwerID, pickerID, userID)
	if !ok {
		return models.BottleThread{}, ErrForbidden
	}
	t.BottleID = bottleID
	t.BottleContent = content
	t.MyAlias = alias
	t.Revealed = revealed == 1
	t.Closed = closed == 1
	if convID.Valid {
		v := convID.String
		t.ConversationID = &v
	}

	rows, err := s.DB.Query(`
SELECT id, thread_id, sender_id, alias, body, created_at
FROM bottle_messages WHERE thread_id = ? ORDER BY created_at ASC
`, threadID)
	if err != nil {
		return models.BottleThread{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var m models.BottleMessage
		var senderID string
		if err := rows.Scan(&m.ID, &m.ThreadID, &senderID, &m.Alias, &m.Body, &m.CreatedAt); err != nil {
			return models.BottleThread{}, err
		}
		m.Mine = senderID == userID
		t.Messages = append(t.Messages, m)
	}
	return t, rows.Err()
}

func (s *Store) AddBottleMessage(threadID, userID, body string) (models.BottleMessage, string, string, error) {
	if body == "" {
		return models.BottleMessage{}, "", "", errors.New("body required")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return models.BottleMessage{}, "", "", err
	}
	defer tx.Rollback()

	var throwerID, pickerID string
	var roundCount, maxRounds, revealed, closed int
	err = tx.QueryRow(`
SELECT thrower_id, picker_id, round_count, max_rounds, revealed, closed
FROM bottle_threads WHERE id = ?
`, threadID).Scan(&throwerID, &pickerID, &roundCount, &maxRounds, &revealed, &closed)
	if errors.Is(err, sql.ErrNoRows) {
		return models.BottleMessage{}, "", "", ErrNotFound
	}
	if err != nil {
		return models.BottleMessage{}, "", "", err
	}
	alias, ok := s.threadRole(throwerID, pickerID, userID)
	if !ok {
		return models.BottleMessage{}, "", "", ErrForbidden
	}
	if closed == 1 || revealed == 1 {
		return models.BottleMessage{}, "", "", ErrThreadClosed
	}
	if roundCount >= maxRounds {
		return models.BottleMessage{}, "", "", ErrMaxRounds
	}

	m := models.BottleMessage{
		ID:        uuid.NewString(),
		ThreadID:  threadID,
		Alias:     alias,
		Mine:      true,
		Body:      body,
		CreatedAt: nowMS(),
	}
	_, err = tx.Exec(
		`INSERT INTO bottle_messages (id, thread_id, sender_id, alias, body, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		m.ID, threadID, userID, alias, body, m.CreatedAt,
	)
	if err != nil {
		return models.BottleMessage{}, "", "", err
	}
	_, err = tx.Exec(`UPDATE bottle_threads SET round_count = round_count + 1 WHERE id = ?`, threadID)
	if err != nil {
		return models.BottleMessage{}, "", "", err
	}
	if err := tx.Commit(); err != nil {
		return models.BottleMessage{}, "", "", err
	}
	peer := pickerID
	if userID == pickerID {
		peer = throwerID
	}
	return m, peer, throwerID, nil
}

func (s *Store) Reveal(threadID, userID string) (convID string, peerID string, peer models.User, err error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return "", "", models.User{}, err
	}
	defer tx.Rollback()

	var throwerID, pickerID string
	var revealed, closed int
	var existing sql.NullString
	err = tx.QueryRow(`
SELECT thrower_id, picker_id, revealed, closed, conversation_id FROM bottle_threads WHERE id = ?
`, threadID).Scan(&throwerID, &pickerID, &revealed, &closed, &existing)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", models.User{}, ErrNotFound
	}
	if err != nil {
		return "", "", models.User{}, err
	}
	if userID != throwerID && userID != pickerID {
		return "", "", models.User{}, ErrForbidden
	}
	if closed == 1 && revealed == 0 {
		return "", "", models.User{}, ErrThreadClosed
	}
	if existing.Valid {
		peerID = pickerID
		if userID == pickerID {
			peerID = throwerID
		}
		var u models.User
		if err := tx.QueryRow(`SELECT id, username, display_name, avatar_color FROM users WHERE id = ?`, peerID).
			Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarColor); err != nil {
			return "", "", models.User{}, err
		}
		return existing.String, peerID, u, tx.Commit()
	}

	a, b := orderedPair(throwerID, pickerID)
	convID = uuid.NewString()
	ts := nowMS()
	_, err = tx.Exec(
		`INSERT INTO conversations (id, user_a_id, user_b_id, created_at, last_message_at) VALUES (?, ?, ?, ?, ?)`,
		convID, a, b, ts, ts,
	)
	if err != nil {
		// may already exist from prior chat
		_ = tx.QueryRow(`SELECT id FROM conversations WHERE user_a_id = ? AND user_b_id = ?`, a, b).Scan(&convID)
	}
	notice := "你们已从漂流瓶公开身份，开始私聊吧。"
	msgID := uuid.NewString()
	_, err = tx.Exec(
		`INSERT INTO messages (id, conversation_id, sender_id, type, body, created_at) VALUES (?, ?, ?, 'text', ?, ?)`,
		msgID, convID, userID, notice, ts,
	)
	if err != nil {
		return "", "", models.User{}, err
	}
	_, err = tx.Exec(`
UPDATE bottle_threads SET revealed = 1, conversation_id = ?, closed = 1 WHERE id = ?
`, convID, threadID)
	if err != nil {
		return "", "", models.User{}, err
	}
	_, err = tx.Exec(`UPDATE bottles SET status = 'closed' WHERE id = (SELECT bottle_id FROM bottle_threads WHERE id = ?)`, threadID)
	if err != nil {
		return "", "", models.User{}, err
	}
	peerID = pickerID
	if userID == pickerID {
		peerID = throwerID
	}
	var u models.User
	if err := tx.QueryRow(`SELECT id, username, display_name, avatar_color FROM users WHERE id = ?`, peerID).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarColor); err != nil {
		return "", "", models.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return "", "", models.User{}, err
	}
	return convID, peerID, u, nil
}

func (s *Store) CloseThread(threadID, userID string) error {
	var throwerID, pickerID string
	var revealed, closed int
	err := s.DB.QueryRow(`
SELECT thrower_id, picker_id, revealed, closed FROM bottle_threads WHERE id = ?
`, threadID).Scan(&throwerID, &pickerID, &revealed, &closed)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if userID != throwerID && userID != pickerID {
		return ErrForbidden
	}
	if closed == 1 {
		return nil
	}
	_, err = s.DB.Exec(`UPDATE bottle_threads SET closed = 1 WHERE id = ?`, threadID)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`UPDATE bottles SET status = 'closed' WHERE id = (SELECT bottle_id FROM bottle_threads WHERE id = ?)`, threadID)
	return err
}
