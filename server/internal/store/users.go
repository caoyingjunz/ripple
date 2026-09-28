package store

import (
	"database/sql"

	"ripple/server/internal/models"
)

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

const userCols = `id, username, display_name, avatar_color, avatar_url, signature, status`

func (s *Store) userByID(id string) (models.User, error) {
	var u models.User
	err := scanUser(s.DB.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id), &u)
	return u, err
}

// usersByIDs 批量取用户（IN 查询），供会话 peer / 好友申请列表复用
func (s *Store) usersByIDs(ids []string) (map[string]models.User, error) {
	out := make(map[string]models.User, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.DB.Query(
		`SELECT `+userCols+` FROM users WHERE id IN (`+joinComma(placeholders)+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var u models.User
		if err := scanUser(rows, &u); err != nil {
			return nil, err
		}
		out[u.ID] = u
	}
	return out, rows.Err()
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

// relation 计算视角 user 看 other：none/pending_out/pending_in/accepted/blocked
func (s *Store) relation(user, other string) (string, error) {
	var mine, theirs sql.NullString
	err := s.DB.QueryRow(
		`SELECT
		   (SELECT status FROM friendships WHERE user_id = ? AND friend_id = ?),
		   (SELECT status FROM friendships WHERE user_id = ? AND friend_id = ?)`,
		user, other, other, user,
	).Scan(&mine, &theirs)
	if err != nil {
		return "", err
	}
	if mine.Valid && mine.String == "blocked" || theirs.Valid && theirs.String == "blocked" {
		return "blocked", nil
	}
	if mine.Valid && mine.String == "accepted" || theirs.Valid && theirs.String == "accepted" {
		return "accepted", nil
	}
	if theirs.Valid && theirs.String == "pending" {
		return "pending_in", nil
	}
	if mine.Valid && mine.String == "pending" {
		return "pending_out", nil
	}
	return "none", nil
}

// SearchUsers 按 username/display_name 模糊搜索，排除自己，最多 20 条
func (s *Store) SearchUsers(userID, q string) ([]models.SearchEntry, error) {
	rows, err := s.DB.Query(`
SELECT id, username, display_name, avatar_color, avatar_url, signature, status
FROM users
WHERE (LOCATE(?, username) > 0 OR LOCATE(?, display_name) > 0) AND id != ?
ORDER BY username LIMIT 20
`, q, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SearchEntry
	for rows.Next() {
		var e models.SearchEntry
		var avatar sql.NullString
		if err := rows.Scan(&e.User.ID, &e.User.Username, &e.User.DisplayName, &e.User.AvatarColor, &avatar, &e.User.Signature, &e.User.Status); err != nil {
			return nil, err
		}
		if avatar.Valid {
			v := avatar.String
			e.User.AvatarURL = &v
		}
		rel, err := s.relation(userID, e.User.ID)
		if err != nil {
			return nil, err
		}
		e.Relation = rel
		out = append(out, e)
	}
	return out, rows.Err()
}
