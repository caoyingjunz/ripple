package store

import (
	"database/sql"
	"errors"
	"fmt"

	"ripple/server/internal/models"

	"github.com/google/uuid"
)

const messageCols = `id, conversation_id, seq, sender_id, type, body, image_url, revoked_at, created_at`

func scanMessage(row scanner, m *models.Message) error {
	var img sql.NullString
	var revoked sql.NullInt64
	if err := row.Scan(&m.ID, &m.ConversationID, &m.Seq, &m.SenderID, &m.Type, &m.Body, &img, &revoked, &m.CreatedAt); err != nil {
		return err
	}
	if img.Valid {
		v := img.String
		m.ImageURL = &v
	}
	if revoked.Valid {
		v := revoked.Int64
		m.RevokedAt = &v
	}
	return nil
}

// blockedBetween 双向黑名单校验（任一方向 blocked 即拦截）
func (s *Store) blockedBetween(a, b string) (bool, error) {
	var n int
	err := s.DB.QueryRow(`
SELECT COUNT(*) FROM friendships
WHERE status = 'blocked' AND ((user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?))
`, a, b, b, a).Scan(&n)
	return n > 0, err
}

// insertMessageTx 事务内原子取号写消息（LAST_INSERT_ID 原子取号，禁止 MAX+1）
func insertMessageTx(tx *sql.Tx, convID, senderID, msgType, body string, imageURL *string, ts int64) (models.Message, error) {
	if _, err := tx.Exec(`UPDATE conversations SET last_seq = LAST_INSERT_ID(last_seq + 1) WHERE id = ?`, convID); err != nil {
		return models.Message{}, err
	}
	var seq int64
	if err := tx.QueryRow(`SELECT LAST_INSERT_ID()`).Scan(&seq); err != nil {
		return models.Message{}, err
	}
	m := models.Message{
		ID:             uuid.NewString(),
		ConversationID: convID,
		Seq:            seq,
		SenderID:       senderID,
		Type:           msgType,
		Body:           body,
		ImageURL:       imageURL,
		CreatedAt:      ts,
	}
	_, err := tx.Exec(
		`INSERT INTO messages (id, conversation_id, seq, sender_id, type, body, image_url, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ConversationID, m.Seq, m.SenderID, m.Type, m.Body, m.ImageURL, m.CreatedAt,
	)
	if err != nil {
		return models.Message{}, err
	}
	if _, err := tx.Exec(`UPDATE conversations SET last_message_at = ? WHERE id = ?`, ts, convID); err != nil {
		return models.Message{}, err
	}
	return m, nil
}

// InsertMessage REST/WS 共用的消息写入路径；single 型先做双向黑名单校验
func (s *Store) InsertMessage(convID, senderID, msgType, body string, imageURL *string) (models.Message, error) {
	if msgType != "text" && msgType != "image" {
		return models.Message{}, fmt.Errorf("invalid type")
	}
	if msgType == "image" && (imageURL == nil || *imageURL == "") {
		return models.Message{}, ErrValidation
	}
	t, err := s.conversationType(convID)
	if err != nil {
		return models.Message{}, err
	}
	if t == "bottle" {
		return models.Message{}, ErrForbidden
	}
	if t != "single" && t != "group" {
		return models.Message{}, ErrNotFound
	}
	members, err := s.MemberIDs(convID)
	if err != nil {
		return models.Message{}, err
	}
	inMembers := false
	peer := ""
	for _, id := range members {
		if id == senderID {
			inMembers = true
		} else {
			peer = id
		}
	}
	if !inMembers {
		return models.Message{}, ErrForbidden
	}
	if t == "single" {
		if peer == "" {
			return models.Message{}, ErrForbidden
		}
		blocked, err := s.blockedBetween(senderID, peer)
		if err != nil {
			return models.Message{}, err
		}
		if blocked {
			return models.Message{}, ErrForbidden
		}
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return models.Message{}, err
	}
	defer tx.Rollback()
	m, err := insertMessageTx(tx, convID, senderID, msgType, body, imageURL, nowMS())
	if err != nil {
		return models.Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.Message{}, err
	}
	return m, nil
}

// ListMessages 三模式：after_seq 增量（ASC）/ before_seq 翻历史（取最新 limit 条转 ASC）/ 默认最新 limit 条 ASC
func (s *Store) ListMessages(convID, userID string, after, before *int64, limit int) ([]models.Message, bool, error) {
	t, err := s.conversationType(convID)
	if err != nil {
		return nil, false, err
	}
	if t != "single" && t != "group" {
		return nil, false, ErrNotFound
	}
	ok, err := s.isMember(convID, userID)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, ErrForbidden
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	q := `SELECT ` + messageCols + ` FROM messages WHERE conversation_id = ?`
	args := []any{convID}
	desc := false
	switch {
	case after != nil:
		q += ` AND seq > ? ORDER BY seq ASC LIMIT ?`
		args = append(args, *after, limit+1)
	case before != nil:
		q += ` AND seq < ? ORDER BY seq DESC LIMIT ?`
		args = append(args, *before, limit+1)
		desc = true
	default:
		q += ` ORDER BY seq DESC LIMIT ?`
		args = append(args, limit+1)
		desc = true
	}
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []models.Message
	for rows.Next() {
		var m models.Message
		if err := scanMessage(rows, &m); err != nil {
			return nil, false, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	if desc {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, hasMore, nil
}

// RevokeMessage 仅发送者、2 分钟窗口内可撤回
func (s *Store) RevokeMessage(messageID, userID string) (models.Message, error) {
	var m models.Message
	err := scanMessage(s.DB.QueryRow(`SELECT `+messageCols+` FROM messages WHERE id = ?`, messageID), &m)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Message{}, ErrNotFound
	}
	if err != nil {
		return models.Message{}, err
	}
	if m.SenderID != userID {
		return models.Message{}, ErrForbidden
	}
	if m.RevokedAt != nil {
		return m, nil // 幂等
	}
	if nowMS()-m.CreatedAt > 2*60*1000 {
		return models.Message{}, ErrForbidden
	}
	ts := nowMS()
	if _, err := s.DB.Exec(
		`UPDATE messages SET body = '', image_url = NULL, revoked_at = ? WHERE id = ?`, ts, messageID,
	); err != nil {
		return models.Message{}, err
	}
	m.Body = ""
	m.ImageURL = nil
	m.RevokedAt = &ts
	return m, nil
}
