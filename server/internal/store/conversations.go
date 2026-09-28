package store

import (
	"database/sql"
	"errors"
	"fmt"

	"ripple/server/internal/db"
	"ripple/server/internal/models"

	"github.com/google/uuid"
)

func (s *Store) conversationType(convID string) (string, error) {
	var t string
	err := s.DB.QueryRow(`SELECT type FROM conversations WHERE id = ?`, convID).Scan(&t)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return t, err
}

func (s *Store) isMember(convID, userID string) (bool, error) {
	var n int
	err := s.DB.QueryRow(
		`SELECT COUNT(*) FROM conversation_members WHERE conversation_id = ? AND user_id = ?`,
		convID, userID,
	).Scan(&n)
	return n > 0, err
}

// MemberIDs 会话全体成员（广播用）
func (s *Store) MemberIDs(convID string) ([]string, error) {
	rows, err := s.DB.Query(`SELECT user_id FROM conversation_members WHERE conversation_id = ?`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) isOwner(convID, userID string) (bool, error) {
	var role string
	err := s.DB.QueryRow(
		`SELECT role FROM conversation_members WHERE conversation_id = ? AND user_id = ?`,
		convID, userID,
	).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return role == "owner", err
}

// GetOrCreateSingle single 会话幂等获取/创建（single_key 唯一键撞锁回查）
func (s *Store) GetOrCreateSingle(a, b string) (string, bool, error) {
	if a == b {
		return "", false, ErrValidation
	}
	key := singleKey(a, b)
	var id string
	err := s.DB.QueryRow(`SELECT id FROM conversations WHERE single_key = ?`, key).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	id = uuid.NewString()
	ts := nowMS()
	tx, err := s.DB.Begin()
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(
		`INSERT INTO conversations (id, type, name, owner_id, single_key, last_seq, created_at) VALUES (?, 'single', NULL, NULL, ?, 0, ?)`,
		id, key, ts,
	)
	if err != nil {
		if db.IsDuplicate(err) {
			_ = tx.Rollback()
			var existing string
			if e := s.DB.QueryRow(`SELECT id FROM conversations WHERE single_key = ?`, key).Scan(&existing); e != nil {
				return "", false, err
			}
			return existing, false, nil
		}
		return "", false, err
	}
	for _, uid := range []string{a, b} {
		if _, err := tx.Exec(
			`INSERT INTO conversation_members (conversation_id, user_id, role, alias, joined_at) VALUES (?, ?, 'member', NULL, ?)`,
			id, uid, ts,
		); err != nil {
			return "", false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return id, true, nil
}

// CreateGroup 建群：写 members + 系统消息
func (s *Store) CreateGroup(ownerID, name string, memberIDs []string) (models.ConversationSummary, error) {
	if n := len([]rune(name)); n < 1 || n > 32 {
		return models.ConversationSummary{}, ErrValidation
	}
	seen := map[string]bool{ownerID: true}
	var members []string
	for _, id := range memberIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		members = append(members, id)
	}
	if len(members) > 50 {
		return models.ConversationSummary{}, ErrValidation
	}
	users, err := s.usersByIDs(members)
	if err != nil {
		return models.ConversationSummary{}, err
	}
	for _, id := range members {
		if _, ok := users[id]; !ok {
			return models.ConversationSummary{}, ErrNotFound
		}
	}
	owner, err := s.userByID(ownerID)
	if err != nil {
		return models.ConversationSummary{}, ErrNotFound
	}

	convID := uuid.NewString()
	ts := nowMS()
	tx, err := s.DB.Begin()
	if err != nil {
		return models.ConversationSummary{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO conversations (id, type, name, owner_id, single_key, last_seq, created_at) VALUES (?, 'group', ?, ?, NULL, 0, ?)`,
		convID, name, ownerID, ts,
	); err != nil {
		return models.ConversationSummary{}, err
	}
	if _, err := tx.Exec(
		`INSERT INTO conversation_members (conversation_id, user_id, role, alias, joined_at) VALUES (?, ?, 'owner', NULL, ?)`,
		convID, ownerID, ts,
	); err != nil {
		return models.ConversationSummary{}, err
	}
	for _, id := range members {
		if _, err := tx.Exec(
			`INSERT INTO conversation_members (conversation_id, user_id, role, alias, joined_at) VALUES (?, ?, 'member', NULL, ?)`,
			convID, id, ts,
		); err != nil {
			return models.ConversationSummary{}, err
		}
	}
	if _, err := insertMessageTx(tx, convID, ownerID, "system", fmt.Sprintf("%s 创建了群聊「%s」", owner.DisplayName, name), nil, ts); err != nil {
		return models.ConversationSummary{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ConversationSummary{}, err
	}
	sums, err := s.summaryRows(ownerID, convID)
	if err != nil || len(sums) == 0 {
		return models.ConversationSummary{}, err
	}
	return sums[0], nil
}

const summarySelect = `
SELECT c.id, c.type, c.name, c.owner_id, c.last_seq, c.last_message_at, c.created_at,
       cm.last_read_seq, cm.pinned, cm.muted,
       (SELECT COUNT(*) FROM conversation_members mc WHERE mc.conversation_id = c.id),
       (SELECT m.type FROM messages m WHERE m.conversation_id = c.id ORDER BY m.seq DESC LIMIT 1),
       (SELECT m.body FROM messages m WHERE m.conversation_id = c.id ORDER BY m.seq DESC LIMIT 1),
       (SELECT m.revoked_at FROM messages m WHERE m.conversation_id = c.id ORDER BY m.seq DESC LIMIT 1)
FROM conversations c
JOIN conversation_members cm ON cm.conversation_id = c.id AND cm.user_id = ?
WHERE c.type IN ('single', 'group')`

func truncateRunes(str string, n int) string {
	r := []rune(str)
	if len(r) <= n {
		return str
	}
	return string(r[:n])
}

// summaryRows 构建会话摘要；convID 为空取全部，否则只取该会话
func (s *Store) summaryRows(userID, convID string) ([]models.ConversationSummary, error) {
	q := summarySelect
	args := []any{userID}
	if convID != "" {
		q += ` AND c.id = ?`
		args = append(args, convID)
	}
	q += ` ORDER BY cm.pinned DESC, COALESCE(c.last_message_at, c.created_at) DESC`
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ConversationSummary
	var singles []string
	for rows.Next() {
		var sum models.ConversationSummary
		var name, owner sql.NullString
		var lastAt, lmRevoked sql.NullInt64
		var lmType, lmBody sql.NullString
		var pinned, muted int
		if err := rows.Scan(
			&sum.ID, &sum.Type, &name, &owner, &sum.LastSeq, &lastAt, &sum.CreatedAt,
			&sum.LastReadSeq, &pinned, &muted, &sum.MemberCount, &lmType, &lmBody, &lmRevoked,
		); err != nil {
			return nil, err
		}
		sum.Pinned = pinned == 1
		sum.Muted = muted == 1
		if name.Valid {
			v := name.String
			sum.Name = &v
		}
		if owner.Valid {
			v := owner.String
			sum.OwnerID = &v
		}
		if lastAt.Valid {
			v := lastAt.Int64
			sum.LastMessageAt = &v
		}
		if d := sum.LastSeq - sum.LastReadSeq; d > 0 {
			sum.Unread = d
		}
		if lmType.Valid {
			preview := ""
			switch {
			case lmRevoked.Valid:
				preview = "[已撤回]"
			case lmType.String == "image":
				preview = "[图片]"
			default:
				preview = truncateRunes(lmBody.String, 40)
			}
			sum.LastMessagePreview = &preview
		}
		if sum.Type == "single" {
			singles = append(singles, sum.ID)
		}
		out = append(out, sum)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(singles) == 0 {
		return out, nil
	}
	// single 型补对端用户
	ph := make([]string, len(singles))
	for i := range ph {
		ph[i] = "?"
	}
	args = make([]any, 0, len(singles)+1)
	args = append(args, userID)
	for _, id := range singles {
		args = append(args, id)
	}
	peerRows, err := s.DB.Query(
		`SELECT cm.conversation_id, u.id, u.username, u.display_name, u.avatar_color, u.avatar_url, u.signature, u.status
		 FROM conversation_members cm JOIN users u ON u.id = cm.user_id
		 WHERE cm.user_id != ? AND cm.conversation_id IN (`+joinComma(ph)+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer peerRows.Close()
	peers := map[string]models.User{}
	for peerRows.Next() {
		var cid string
		var u models.User
		var avatar sql.NullString
		if err := peerRows.Scan(
			&cid, &u.ID, &u.Username, &u.DisplayName, &u.AvatarColor, &avatar, &u.Signature, &u.Status,
		); err != nil {
			return nil, err
		}
		if avatar.Valid {
			v := avatar.String
			u.AvatarURL = &v
		}
		peers[cid] = u
	}
	if err := peerRows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Type == "single" {
			if u, ok := peers[out[i].ID]; ok {
				u := u
				out[i].Peer = &u
			}
		}
	}
	return out, nil
}

func (s *Store) ListConversations(userID string) ([]models.ConversationSummary, error) {
	return s.summaryRows(userID, "")
}

// ConversationSummaryOf 单会话摘要（创建/改名后响应用）
func (s *Store) ConversationSummaryOf(convID, userID string) (models.ConversationSummary, error) {
	sums, err := s.summaryRows(userID, convID)
	if err != nil {
		return models.ConversationSummary{}, err
	}
	if len(sums) == 0 {
		return models.ConversationSummary{}, ErrNotFound
	}
	return sums[0], nil
}

// GetConversation single/group 明细（bottle 型 404）
func (s *Store) GetConversation(convID, userID string) (models.ConversationSummary, []models.MemberEntry, error) {
	t, err := s.conversationType(convID)
	if err != nil {
		return models.ConversationSummary{}, nil, err
	}
	if t == "bottle" {
		return models.ConversationSummary{}, nil, ErrNotFound
	}
	if t != "single" && t != "group" {
		return models.ConversationSummary{}, nil, ErrNotFound
	}
	ok, err := s.isMember(convID, userID)
	if err != nil {
		return models.ConversationSummary{}, nil, err
	}
	if !ok {
		return models.ConversationSummary{}, nil, ErrForbidden
	}
	sums, err := s.summaryRows(userID, convID)
	if err != nil {
		return models.ConversationSummary{}, nil, err
	}
	if len(sums) == 0 {
		return models.ConversationSummary{}, nil, ErrNotFound
	}
	rows, err := s.DB.Query(`
SELECT cm.role, cm.alias, cm.joined_at, u.id, u.username, u.display_name, u.avatar_color, u.avatar_url, u.signature, u.status
FROM conversation_members cm JOIN users u ON u.id = cm.user_id
WHERE cm.conversation_id = ?
ORDER BY cm.joined_at, u.display_name
`, convID)
	if err != nil {
		return models.ConversationSummary{}, nil, err
	}
	defer rows.Close()
	var members []models.MemberEntry
	for rows.Next() {
		var me models.MemberEntry
		var alias sql.NullString
		var avatar sql.NullString
		if err := rows.Scan(
			&me.Role, &alias, &me.JoinedAt,
			&me.User.ID, &me.User.Username, &me.User.DisplayName, &me.User.AvatarColor, &avatar, &me.User.Signature, &me.User.Status,
		); err != nil {
			return models.ConversationSummary{}, nil, err
		}
		if alias.Valid {
			v := alias.String
			me.Alias = &v
		}
		if avatar.Valid {
			v := avatar.String
			me.User.AvatarURL = &v
		}
		members = append(members, me)
	}
	return sums[0], members, rows.Err()
}

// AddMembers 群主加人（已成员跳过）
func (s *Store) AddMembers(convID, ownerID string, userIDs []string) ([]models.User, error) {
	t, err := s.conversationType(convID)
	if err != nil {
		return nil, err
	}
	if t == "bottle" {
		return nil, ErrNotFound
	}
	if t != "group" {
		return nil, ErrConflict
	}
	owner, err := s.isOwner(convID, ownerID)
	if err != nil {
		return nil, err
	}
	if !owner {
		return nil, ErrForbidden
	}
	if len(userIDs) > 50 {
		return nil, ErrValidation
	}
	existing, err := s.MemberIDs(convID)
	if err != nil {
		return nil, err
	}
	existSet := map[string]bool{}
	for _, id := range existing {
		existSet[id] = true
	}
	seen := map[string]bool{}
	var toAdd []string
	for _, id := range userIDs {
		if seen[id] || existSet[id] {
			continue
		}
		seen[id] = true
		toAdd = append(toAdd, id)
	}
	if len(toAdd) == 0 {
		return []models.User{}, nil
	}
	users, err := s.usersByIDs(toAdd)
	if err != nil {
		return nil, err
	}
	for _, id := range toAdd {
		if _, ok := users[id]; !ok {
			return nil, ErrNotFound
		}
	}
	ownerUser, err := s.userByID(ownerID)
	if err != nil {
		return nil, err
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ts := nowMS()
	for _, id := range toAdd {
		if _, err := tx.Exec(
			`INSERT INTO conversation_members (conversation_id, user_id, role, alias, joined_at) VALUES (?, ?, 'member', NULL, ?)`,
			convID, id, ts,
		); err != nil {
			return nil, err
		}
	}
	names := make([]string, len(toAdd))
	for i, id := range toAdd {
		names[i] = users[id].DisplayName
	}
	if _, err := insertMessageTx(tx, convID, ownerID, "system", fmt.Sprintf("%s 邀请 %s 加入群聊", ownerUser.DisplayName, joinComma(names)), nil, ts); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	added := make([]models.User, 0, len(toAdd))
	for _, id := range toAdd {
		added = append(added, users[id])
	}
	return added, nil
}

// RemoveMember 群主移人
func (s *Store) RemoveMember(convID, ownerID, targetID string) error {
	t, err := s.conversationType(convID)
	if err != nil {
		return err
	}
	if t == "bottle" {
		return ErrNotFound
	}
	if t != "group" {
		return ErrConflict
	}
	owner, err := s.isOwner(convID, ownerID)
	if err != nil {
		return err
	}
	if !owner {
		return ErrForbidden
	}
	if targetID == ownerID {
		return ErrForbidden
	}
	member, err := s.isMember(convID, targetID)
	if err != nil {
		return err
	}
	if !member {
		return ErrNotFound
	}
	ownerUser, err := s.userByID(ownerID)
	if err != nil {
		return err
	}
	target, err := s.userByID(targetID)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM conversation_members WHERE conversation_id = ? AND user_id = ?`, convID, targetID,
	); err != nil {
		return err
	}
	if _, err := insertMessageTx(tx, convID, ownerID, "system", fmt.Sprintf("%s 移除了 %s", ownerUser.DisplayName, target.DisplayName), nil, nowMS()); err != nil {
		return err
	}
	return tx.Commit()
}

// LeaveGroup 退群（群主不可退）
func (s *Store) LeaveGroup(convID, userID string) error {
	t, err := s.conversationType(convID)
	if err != nil {
		return err
	}
	if t == "bottle" {
		return ErrNotFound
	}
	if t != "group" {
		return ErrConflict
	}
	ok, err := s.isMember(convID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	owner, err := s.isOwner(convID, userID)
	if err != nil {
		return err
	}
	if owner {
		return ErrForbidden
	}
	u, err := s.userByID(userID)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM conversation_members WHERE conversation_id = ? AND user_id = ?`, convID, userID,
	); err != nil {
		return err
	}
	if _, err := insertMessageTx(tx, convID, userID, "system", fmt.Sprintf("%s 退出了群聊", u.DisplayName), nil, nowMS()); err != nil {
		return err
	}
	return tx.Commit()
}

// RenameGroup 群主改名
func (s *Store) RenameGroup(convID, ownerID, name string) (models.ConversationSummary, error) {
	if n := len([]rune(name)); n < 1 || n > 32 {
		return models.ConversationSummary{}, ErrValidation
	}
	t, err := s.conversationType(convID)
	if err != nil {
		return models.ConversationSummary{}, err
	}
	if t == "bottle" {
		return models.ConversationSummary{}, ErrNotFound
	}
	if t != "group" {
		return models.ConversationSummary{}, ErrConflict
	}
	owner, err := s.isOwner(convID, ownerID)
	if err != nil {
		return models.ConversationSummary{}, err
	}
	if !owner {
		return models.ConversationSummary{}, ErrForbidden
	}
	ownerUser, err := s.userByID(ownerID)
	if err != nil {
		return models.ConversationSummary{}, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return models.ConversationSummary{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE conversations SET name = ? WHERE id = ?`, name, convID); err != nil {
		return models.ConversationSummary{}, err
	}
	if _, err := insertMessageTx(tx, convID, ownerID, "system", fmt.Sprintf("%s 修改群名为「%s」", ownerUser.DisplayName, name), nil, nowMS()); err != nil {
		return models.ConversationSummary{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ConversationSummary{}, err
	}
	sums, err := s.summaryRows(ownerID, convID)
	if err != nil || len(sums) == 0 {
		return models.ConversationSummary{}, err
	}
	return sums[0], nil
}

// SetReadSeq 已读上报，水位只升不降
func (s *Store) SetReadSeq(convID, userID string, seq int64) (int64, error) {
	ok, err := s.isMember(convID, userID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrForbidden
	}
	if _, err := s.DB.Exec(
		`UPDATE conversation_members SET last_read_seq = ? WHERE conversation_id = ? AND user_id = ? AND last_read_seq < ?`,
		seq, convID, userID, seq,
	); err != nil {
		return 0, err
	}
	var cur int64
	if err := s.DB.QueryRow(
		`SELECT last_read_seq FROM conversation_members WHERE conversation_id = ? AND user_id = ?`, convID, userID,
	).Scan(&cur); err != nil {
		return 0, err
	}
	return cur, nil
}
