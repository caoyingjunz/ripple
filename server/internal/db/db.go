package db

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func Migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  avatar_color TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS conversations (
  id TEXT PRIMARY KEY,
  user_a_id TEXT NOT NULL,
  user_b_id TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  last_message_at INTEGER,
  UNIQUE(user_a_id, user_b_id),
  FOREIGN KEY(user_a_id) REFERENCES users(id),
  FOREIGN KEY(user_b_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  sender_id TEXT NOT NULL,
  type TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  image_url TEXT,
  created_at INTEGER NOT NULL,
  FOREIGN KEY(conversation_id) REFERENCES conversations(id),
  FOREIGN KEY(sender_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS bottles (
  id TEXT PRIMARY KEY,
  thrower_id TEXT NOT NULL,
  content TEXT NOT NULL,
  status TEXT NOT NULL,
  picker_id TEXT,
  created_at INTEGER NOT NULL,
  picked_at INTEGER,
  FOREIGN KEY(thrower_id) REFERENCES users(id),
  FOREIGN KEY(picker_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS bottle_threads (
  id TEXT PRIMARY KEY,
  bottle_id TEXT NOT NULL UNIQUE,
  thrower_id TEXT NOT NULL,
  picker_id TEXT NOT NULL,
  round_count INTEGER NOT NULL DEFAULT 0,
  max_rounds INTEGER NOT NULL,
  revealed INTEGER NOT NULL DEFAULT 0,
  conversation_id TEXT,
  closed INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  FOREIGN KEY(bottle_id) REFERENCES bottles(id),
  FOREIGN KEY(thrower_id) REFERENCES users(id),
  FOREIGN KEY(picker_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS bottle_messages (
  id TEXT PRIMARY KEY,
  thread_id TEXT NOT NULL,
  sender_id TEXT NOT NULL,
  alias TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  FOREIGN KEY(thread_id) REFERENCES bottle_threads(id),
  FOREIGN KEY(sender_id) REFERENCES users(id)
);
`)
	return err
}
