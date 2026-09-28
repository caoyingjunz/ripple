package store

import (
	"database/sql"
	"errors"

	"ripple/server/internal/models"

	"github.com/google/uuid"
)

// AddFriend 发起好友申请；反向 pending 存在时自动置 accepted（互加秒通过）。
// 返回（申请行 id、行 created_at、对方用户、pending|accepted）
func (s *Store) AddFriend(userID, targetID string) (string, int64, models.User, string, error) {
	if userID == targetID {
		return "", 0, models.User{}, "", ErrValidation
	}
	friend, err := s.userByID(targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, models.User{}, "", ErrNotFound
	}
	if err != nil {
		return "", 0, models.User{}, "", err
	}
	var mine, theirs sql.NullString
	err = s.DB.QueryRow(`
SELECT
  (SELECT status FROM friendships WHERE user_id = ? AND friend_id = ?),
  (SELECT status FROM friendships WHERE user_id = ? AND friend_id = ?)
`, userID, targetID, targetID, userID).Scan(&mine, &theirs)
	if err != nil {
		return "", 0, models.User{}, "", err
	}
	if theirs.Valid && theirs.String == "pending" {
		var rowID string
		var createdAt int64
		if err := s.DB.QueryRow(
			`SELECT id, created_at FROM friendships WHERE user_id = ? AND friend_id = ? AND status = 'pending'`,
			targetID, userID,
		).Scan(&rowID, &createdAt); err != nil {
			return "", 0, models.User{}, "", err
		}
		if _, err := s.DB.Exec(
			`UPDATE friendships SET status = 'accepted', updated_at = ? WHERE id = ?`, nowMS(), rowID,
		); err != nil {
			return "", 0, models.User{}, "", err
		}
		return rowID, createdAt, friend, "accepted", nil
	}
	if mine.Valid || theirs.Valid {
		// 已好友 / 已屏蔽 / 对方屏蔽我 / 重复申请
		return "", 0, models.User{}, "", ErrConflict
	}
	rowID := uuid.NewString()
	ts := nowMS()
	_, err = s.DB.Exec(
		`INSERT INTO friendships (id, user_id, friend_id, status, created_at, updated_at) VALUES (?, ?, ?, 'pending', ?, ?)`,
		rowID, userID, targetID, ts, ts,
	)
	if err != nil {
		return "", 0, models.User{}, "", err
	}
	return rowID, ts, friend, "pending", nil
}

// AcceptRequest 收件人同意申请。返回（原申请人用户、行 created_at、原申请人 id）
func (s *Store) AcceptRequest(requestID, recipientID string) (models.User, int64, string, error) {
	var from, to, status string
	var createdAt int64
	err := s.DB.QueryRow(
		`SELECT user_id, friend_id, status, created_at FROM friendships WHERE id = ?`, requestID,
	).Scan(&from, &to, &status, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.User{}, 0, "", ErrNotFound
	}
	if err != nil {
		return models.User{}, 0, "", err
	}
	if to != recipientID {
		return models.User{}, 0, "", ErrForbidden
	}
	if status != "pending" {
		return models.User{}, 0, "", ErrConflict
	}
	_, err = s.DB.Exec(
		`UPDATE friendships SET status = 'accepted', updated_at = ? WHERE id = ?`, nowMS(), requestID,
	)
	if err != nil {
		return models.User{}, 0, "", err
	}
	friend, err := s.userByID(from)
	if err != nil {
		return models.User{}, 0, "", err
	}
	return friend, createdAt, from, nil
}

// DeclineRequest 收件人拒绝申请（删除 pending 行）
func (s *Store) DeclineRequest(requestID, recipientID string) error {
	var from, to, status string
	err := s.DB.QueryRow(
		`SELECT user_id, friend_id, status FROM friendships WHERE id = ?`, requestID,
	).Scan(&from, &to, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if to != recipientID {
		return ErrForbidden
	}
	if status != "pending" {
		return ErrConflict
	}
	_, err = s.DB.Exec(`DELETE FROM friendships WHERE id = ?`, requestID)
	return err
}

// RemoveFriend 删好友，或撤销自己发起的 pending 申请
func (s *Store) RemoveFriend(userID, targetID string) error {
	if _, err := s.userByID(targetID); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	_, err := s.DB.Exec(`
DELETE FROM friendships
WHERE ((user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)) AND status = 'accepted'
`, userID, targetID, targetID, userID)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(
		`DELETE FROM friendships WHERE user_id = ? AND friend_id = ? AND status = 'pending'`,
		userID, targetID,
	)
	return err
}

// BlockUser 拉黑：upsert (me→them, blocked) 并删除反向行
func (s *Store) BlockUser(userID, targetID string) error {
	if userID == targetID {
		return ErrValidation
	}
	if _, err := s.userByID(targetID); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status sql.NullString
	if err := tx.QueryRow(
		`SELECT status FROM friendships WHERE user_id = ? AND friend_id = ?`, userID, targetID,
	).Scan(&status); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	ts := nowMS()
	if status.Valid {
		if status.String == "blocked" {
			return ErrConflict
		}
		if _, err := tx.Exec(
			`UPDATE friendships SET status = 'blocked', updated_at = ? WHERE user_id = ? AND friend_id = ?`,
			ts, userID, targetID,
		); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(
			`INSERT INTO friendships (id, user_id, friend_id, status, created_at, updated_at) VALUES (?, ?, ?, 'blocked', ?, ?)`,
			uuid.NewString(), userID, targetID, ts, ts,
		); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		`DELETE FROM friendships WHERE user_id = ? AND friend_id = ?`, targetID, userID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// UnblockUser 解除拉黑：删除 (me→them, blocked) 行，回到 none（不恢复好友）
func (s *Store) UnblockUser(userID, targetID string) error {
	if _, err := s.userByID(targetID); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	_, err := s.DB.Exec(
		`DELETE FROM friendships WHERE user_id = ? AND friend_id = ? AND status = 'blocked'`,
		userID, targetID,
	)
	return err
}

// ListFriends 好友全景：好友/收到的申请/发出的申请/黑名单
func (s *Store) ListFriends(userID string) (models.FriendsView, error) {
	var out models.FriendsView

	rows, err := s.DB.Query(`
SELECT DISTINCT u.id, u.username, u.display_name, u.avatar_color, u.avatar_url, u.signature, u.status
FROM users u JOIN friendships f
  ON (f.user_id = u.id AND f.friend_id = ?) OR (f.friend_id = u.id AND f.user_id = ?)
WHERE f.status = 'accepted'
ORDER BY u.display_name
`, userID, userID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var u models.User
		if err := scanUser(rows, &u); err != nil {
			return out, err
		}
		out.Friends = append(out.Friends, u)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	rows, err = s.DB.Query(`
SELECT f.id, f.user_id, f.created_at
FROM friendships f
WHERE f.friend_id = ? AND f.status = 'pending'
ORDER BY f.created_at DESC
`, userID)
	if err != nil {
		return out, err
	}
	type reqRow struct {
		id, uid string
		at      int64
	}
	var reqs []reqRow
	for rows.Next() {
		var r reqRow
		if err := rows.Scan(&r.id, &r.uid, &r.at); err != nil {
			rows.Close()
			return out, err
		}
		reqs = append(reqs, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	if len(reqs) > 0 {
		ids := make([]string, len(reqs))
		for i, r := range reqs {
			ids[i] = r.uid
		}
		users, err := s.usersByIDs(ids)
		if err != nil {
			return out, err
		}
		for _, r := range reqs {
			out.PendingIn = append(out.PendingIn, models.FriendRequest{ID: r.id, User: users[r.uid], CreatedAt: r.at})
		}
	}

	rows, err = s.DB.Query(`
SELECT f.id, f.friend_id, f.created_at
FROM friendships f
WHERE f.user_id = ? AND f.status = 'pending'
ORDER BY f.created_at DESC
`, userID)
	if err != nil {
		return out, err
	}
	reqs = nil
	for rows.Next() {
		var r reqRow
		if err := rows.Scan(&r.id, &r.uid, &r.at); err != nil {
			rows.Close()
			return out, err
		}
		reqs = append(reqs, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	if len(reqs) > 0 {
		ids := make([]string, len(reqs))
		for i, r := range reqs {
			ids[i] = r.uid
		}
		users, err := s.usersByIDs(ids)
		if err != nil {
			return out, err
		}
		for _, r := range reqs {
			out.PendingOut = append(out.PendingOut, models.FriendRequest{ID: r.id, User: users[r.uid], CreatedAt: r.at})
		}
	}

	rows, err = s.DB.Query(`
SELECT u.id, u.username, u.display_name, u.avatar_color, u.avatar_url, u.signature, u.status
FROM friendships f JOIN users u ON u.id = f.friend_id
WHERE f.user_id = ? AND f.status = 'blocked'
ORDER BY u.display_name
`, userID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var u models.User
		if err := scanUser(rows, &u); err != nil {
			return out, err
		}
		out.Blocked = append(out.Blocked, u)
	}
	return out, rows.Err()
}
