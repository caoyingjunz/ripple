package store

import (
	"database/sql"
	"errors"
	"strings"

	"ripple/server/internal/models"

	"github.com/google/uuid"
)

const (
	aliasThrower = "扔瓶人"
	aliasPicker  = "捡瓶人"
)

func (s *Store) ThrowBottle(userID, content string) (models.Bottle, error) {
	if strings.TrimSpace(content) == "" {
		return models.Bottle{}, ErrValidation
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
		b.ID, userID, b.Content, b.Status, b.CreatedAt,
	)
	if err != nil {
		return models.Bottle{}, err
	}
	return b, nil
}

// PickBottle 随机捡瓶（排除自己的瓶与双向黑名单的瓶）；事务内建 bottle 会话+thread+首条消息
func (s *Store) PickBottle(userID string) (models.BottleThreadView, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return models.BottleThreadView{}, err
	}
	defer tx.Rollback()

	var bottleID, throwerID, content string
	var createdAt int64
	err = tx.QueryRow(`
SELECT id, thrower_id, content, created_at FROM bottles
WHERE status = 'floating' AND thrower_id != ?
  AND thrower_id NOT IN (SELECT friend_id FROM friendships WHERE user_id = ? AND status = 'blocked')
  AND thrower_id NOT IN (SELECT user_id FROM friendships WHERE friend_id = ? AND status = 'blocked')
ORDER BY RAND() LIMIT 1
`, userID, userID, userID).Scan(&bottleID, &throwerID, &content, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.BottleThreadView{}, ErrNotFound
	}
	if err != nil {
		return models.BottleThreadView{}, err
	}

	res, err := tx.Exec(`
UPDATE bottles SET status = 'picked', picker_id = ?, picked_at = ?
WHERE id = ? AND status = 'floating'
`, userID, nowMS(), bottleID)
	if err != nil {
		return models.BottleThreadView{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.BottleThreadView{}, ErrConflict
	}

	convID := uuid.NewString()
	threadID := uuid.NewString()
	ts := nowMS()
	_, err = tx.Exec(
		`INSERT INTO conversations (id, type, name, owner_id, single_key, last_seq, created_at) VALUES (?, 'bottle', NULL, NULL, NULL, 0, ?)`,
		convID, ts,
	)
	if err != nil {
		return models.BottleThreadView{}, err
	}
	_, err = tx.Exec(
		`INSERT INTO conversation_members (conversation_id, user_id, role, alias, joined_at) VALUES (?, ?, 'member', ?, ?)`,
		convID, throwerID, aliasThrower, ts,
	)
	if err != nil {
		return models.BottleThreadView{}, err
	}
	_, err = tx.Exec(
		`INSERT INTO conversation_members (conversation_id, user_id, role, alias, joined_at) VALUES (?, ?, 'member', ?, ?)`,
		convID, userID, aliasPicker, ts,
	)
	if err != nil {
		return models.BottleThreadView{}, err
	}
	_, err = tx.Exec(
		`INSERT INTO bottle_threads (id, bottle_id, thrower_id, picker_id, round_count, max_rounds, revealed, conversation_id, closed, created_at)
		 VALUES (?, ?, ?, ?, 0, ?, 0, ?, 0, ?)`,
		threadID, bottleID, throwerID, userID, s.MaxRounds, convID, ts,
	)
	if err != nil {
		return models.BottleThreadView{}, err
	}
	// 瓶内容作为首条消息进统一 messages 表
	if _, err := insertMessageTx(tx, convID, throwerID, "text", content, nil, ts); err != nil {
		return models.BottleThreadView{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.BottleThreadView{}, err
	}
	return s.GetThread(threadID, userID)
}

// threadAlias 计算 userID 在 thread 中的匿名别名
func threadAlias(throwerID, pickerID, userID string) (string, bool) {
	if userID == throwerID {
		return aliasThrower, true
	}
	if userID == pickerID {
		return aliasPicker, true
	}
	return "", false
}

// GetThread 匿名线程视图（对外不暴露真实身份）
func (s *Store) GetThread(threadID, userID string) (models.BottleThreadView, error) {
	var t models.BottleThreadView
	var throwerID, pickerID string
	var bottleID, content string
	var revealed, closed int
	var convID sql.NullString
	err := s.DB.QueryRow(`
SELECT t.id, t.bottle_id, t.thrower_id, t.picker_id, t.round_count, t.max_rounds, t.revealed, t.conversation_id, t.closed, t.created_at, b.content
FROM bottle_threads t JOIN bottles b ON b.id = t.bottle_id
WHERE t.id = ?
`, threadID).Scan(
		&t.ID, &bottleID, &throwerID, &pickerID, &t.RoundCount, &t.MaxRounds, &revealed, &convID, &closed, &t.CreatedAt, &content,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.BottleThreadView{}, ErrNotFound
	}
	if err != nil {
		return models.BottleThreadView{}, err
	}
	alias, ok := threadAlias(throwerID, pickerID, userID)
	if !ok {
		return models.BottleThreadView{}, ErrForbidden
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
	if !convID.Valid {
		return t, nil
	}
	rows, err := s.DB.Query(`
SELECT m.id, m.conversation_id, m.seq, m.sender_id, m.body, m.created_at, cm.alias
FROM messages m JOIN conversation_members cm ON cm.conversation_id = m.conversation_id AND cm.user_id = m.sender_id
WHERE m.conversation_id = ?
ORDER BY m.seq ASC
`, convID.String)
	if err != nil {
		return models.BottleThreadView{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var mv models.BottleMessageView
		var senderID string
		if err := rows.Scan(&mv.ID, &mv.ConversationID, &mv.Seq, &senderID, &mv.Body, &mv.CreatedAt, &mv.Alias); err != nil {
			return models.BottleThreadView{}, err
		}
		mv.Mine = senderID == userID
		mv.Type = "text"
		t.Messages = append(t.Messages, mv)
	}
	return t, rows.Err()
}

// ListMyBottles 我扔的+我捡的（含 thread 状态、未读数）
func (s *Store) ListMyBottles(userID string) ([]models.Bottle, error) {
	rows, err := s.DB.Query(`
SELECT b.id, b.content, b.status, b.created_at, t.id, t.conversation_id,
  CASE WHEN b.thrower_id = ? THEN 'thrower' ELSE 'picker' END,
  c.last_seq, cm.last_read_seq
FROM bottles b
LEFT JOIN bottle_threads t ON t.bottle_id = b.id
LEFT JOIN conversations c ON c.id = t.conversation_id
LEFT JOIN conversation_members cm ON cm.conversation_id = t.conversation_id AND cm.user_id = ?
WHERE b.thrower_id = ? OR b.picker_id = ?
ORDER BY b.created_at DESC
`, userID, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Bottle
	for rows.Next() {
		var b models.Bottle
		var tid, convID sql.NullString
		var lastSeq, lastRead sql.NullInt64
		if err := rows.Scan(&b.ID, &b.Content, &b.Status, &b.CreatedAt, &tid, &convID, &b.Role, &lastSeq, &lastRead); err != nil {
			return nil, err
		}
		if tid.Valid {
			v := tid.String
			b.ThreadID = &v
		}
		if convID.Valid {
			v := convID.String
			b.ConversationID = &v
		}
		if d := lastSeq.Int64 - lastRead.Int64; d > 0 {
			b.Unread = d
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// AddBottleMessage 匿名对话消息（round_count ≥ max_rounds → 409）
func (s *Store) AddBottleMessage(threadID, userID, body string) (models.BottleMessageView, string, error) {
	if strings.TrimSpace(body) == "" {
		return models.BottleMessageView{}, "", ErrValidation
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return models.BottleMessageView{}, "", err
	}
	defer tx.Rollback()
	var throwerID, pickerID string
	var roundCount, maxRounds, revealed, closed int
	var convID sql.NullString
	err = tx.QueryRow(`
SELECT thrower_id, picker_id, round_count, max_rounds, revealed, closed, conversation_id
FROM bottle_threads WHERE id = ?
`, threadID).Scan(&throwerID, &pickerID, &roundCount, &maxRounds, &revealed, &closed, &convID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.BottleMessageView{}, "", ErrNotFound
	}
	if err != nil {
		return models.BottleMessageView{}, "", err
	}
	alias, ok := threadAlias(throwerID, pickerID, userID)
	if !ok {
		return models.BottleMessageView{}, "", ErrForbidden
	}
	if closed == 1 || revealed == 1 {
		return models.BottleMessageView{}, "", ErrThreadClosed
	}
	if roundCount >= maxRounds {
		return models.BottleMessageView{}, "", ErrMaxRounds
	}
	m, err := insertMessageTx(tx, convID.String, userID, "text", body, nil, nowMS())
	if err != nil {
		return models.BottleMessageView{}, "", err
	}
	if _, err := tx.Exec(`UPDATE bottle_threads SET round_count = round_count + 1 WHERE id = ?`, threadID); err != nil {
		return models.BottleMessageView{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return models.BottleMessageView{}, "", err
	}
	view := models.BottleMessageView{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		Seq:            m.Seq,
		Alias:          alias,
		Mine:           true,
		Type:           "text",
		Body:           m.Body,
		CreatedAt:      m.CreatedAt,
	}
	peer := throwerID
	if userID == throwerID {
		peer = pickerID
	}
	return view, peer, nil
}

// Reveal 公开身份：建私聊会话 + system 消息，thread 置 revealed/closed
func (s *Store) Reveal(threadID, userID string) (string, models.User, error) {
	var throwerID, pickerID string
	var revealed, closed int
	var existing sql.NullString
	err := s.DB.QueryRow(`
SELECT thrower_id, picker_id, revealed, closed, conversation_id FROM bottle_threads WHERE id = ?
`, threadID).Scan(&throwerID, &pickerID, &revealed, &closed, &existing)
	if errors.Is(err, sql.ErrNoRows) {
		return "", models.User{}, ErrNotFound
	}
	if err != nil {
		return "", models.User{}, err
	}
	if userID != throwerID && userID != pickerID {
		return "", models.User{}, ErrForbidden
	}
	peerID := throwerID
	if userID == throwerID {
		peerID = pickerID
	}
	if revealed == 1 {
		// 幂等：已 reveal，私聊会话由 single_key 确定
		convID, _, err := s.GetOrCreateSingle(throwerID, pickerID)
		if err != nil {
			return "", models.User{}, err
		}
		peer, err := s.userByID(peerID)
		if err != nil {
			return "", models.User{}, err
		}
		return convID, peer, nil
	}
	if closed == 1 {
		return "", models.User{}, ErrThreadClosed
	}
	convID, _, err := s.GetOrCreateSingle(throwerID, pickerID)
	if err != nil {
		return "", models.User{}, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return "", models.User{}, err
	}
	defer tx.Rollback()
	if _, err := insertMessageTx(tx, convID, userID, "system", "你们已从漂流瓶公开身份，开始私聊吧。", nil, nowMS()); err != nil {
		return "", models.User{}, err
	}
	// 注意：不动 conversation_id（它是 bottle 型匿名会话 id），只翻转 revealed/closed
	if _, err := tx.Exec(`UPDATE bottle_threads SET revealed = 1, closed = 1 WHERE id = ?`, threadID); err != nil {
		return "", models.User{}, err
	}
	if _, err := tx.Exec(
		`UPDATE bottles SET status = 'closed' WHERE id = (SELECT bottle_id FROM bottle_threads WHERE id = ?)`, threadID,
	); err != nil {
		return "", models.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return "", models.User{}, err
	}
	peer, err := s.userByID(peerID)
	if err != nil {
		return "", models.User{}, err
	}
	return convID, peer, nil
}

// CloseThread 关闭匿名对话
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
	if _, err := s.DB.Exec(`UPDATE bottle_threads SET closed = 1 WHERE id = ?`, threadID); err != nil {
		return err
	}
	_, err = s.DB.Exec(
		`UPDATE bottles SET status = 'closed' WHERE id = (SELECT bottle_id FROM bottle_threads WHERE id = ?)`, threadID,
	)
	return err
}
