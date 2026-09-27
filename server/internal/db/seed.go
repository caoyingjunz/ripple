package db

import (
	"database/sql"

	"golang.org/x/crypto/bcrypt"
)

func SeedDemo(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("demo123"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	users := []struct {
		id, username, display, color string
	}{
		{"u-alice", "alice", "Alice", "#0d9488"},
		{"u-bob", "bob", "Bob", "#0284c7"},
		{"u-carol", "carol", "Carol", "#0369a1"},
		{"u-dave", "dave", "Dave", "#0f766e"},
		{"u-erin", "erin", "Erin", "#155e75"},
	}
	for _, u := range users {
		_, err := db.Exec(
			`INSERT INTO users (id, username, display_name, password_hash, avatar_color) VALUES (?, ?, ?, ?, ?)`,
			u.id, u.username, u.display, string(hash), u.color,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
