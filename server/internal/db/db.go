package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// IsDuplicate 判断是否唯一键冲突（errno 1062），用于幂等回查
func IsDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func Migrate(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	// 清理上次进程崩溃残留的在线状态
	_, err := db.Exec(`UPDATE users SET status = 'offline'`)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id VARCHAR(36) PRIMARY KEY,
  username VARCHAR(32) NOT NULL UNIQUE,
  display_name VARCHAR(64) NOT NULL,
  password_hash VARCHAR(100) NOT NULL,
  avatar_color VARCHAR(16) NOT NULL DEFAULT '#0d9488',
  avatar_url VARCHAR(255) NULL,
  signature VARCHAR(128) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL DEFAULT 'offline',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS friendships (
  id VARCHAR(36) PRIMARY KEY,
  user_id VARCHAR(36) NOT NULL,
  friend_id VARCHAR(36) NOT NULL,
  status VARCHAR(16) NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  UNIQUE KEY uq_friendship (user_id, friend_id),
  KEY idx_fs_friend (friend_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS conversations (
  id VARCHAR(36) PRIMARY KEY,
  type VARCHAR(8) NOT NULL,
  name VARCHAR(64) NULL,
  owner_id VARCHAR(36) NULL,
  single_key VARCHAR(83) NULL UNIQUE,
  last_seq BIGINT NOT NULL DEFAULT 0,
  last_message_at BIGINT NULL,
  created_at BIGINT NOT NULL,
  KEY idx_conv_time (last_message_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS conversation_members (
  conversation_id VARCHAR(36) NOT NULL,
  user_id VARCHAR(36) NOT NULL,
  role VARCHAR(16) NOT NULL DEFAULT 'member',
  alias VARCHAR(32) NULL,
  last_read_seq BIGINT NOT NULL DEFAULT 0,
  pinned TINYINT NOT NULL DEFAULT 0,
  muted TINYINT NOT NULL DEFAULT 0,
  joined_at BIGINT NOT NULL,
  PRIMARY KEY (conversation_id, user_id),
  KEY idx_cm_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS messages (
  id VARCHAR(36) PRIMARY KEY,
  conversation_id VARCHAR(36) NOT NULL,
  seq BIGINT NOT NULL,
  sender_id VARCHAR(36) NOT NULL,
  type VARCHAR(8) NOT NULL,
  body TEXT NOT NULL,
  image_url VARCHAR(255) NULL,
  revoked_at BIGINT NULL,
  created_at BIGINT NOT NULL,
  UNIQUE KEY uq_conv_seq (conversation_id, seq),
  KEY idx_msg_time (conversation_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS bottles (
  id VARCHAR(36) PRIMARY KEY,
  thrower_id VARCHAR(36) NOT NULL,
  content TEXT NOT NULL,
  status VARCHAR(16) NOT NULL,
  picker_id VARCHAR(36) NULL,
  created_at BIGINT NOT NULL,
  picked_at BIGINT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS bottle_threads (
  id VARCHAR(36) PRIMARY KEY,
  bottle_id VARCHAR(36) NOT NULL UNIQUE,
  thrower_id VARCHAR(36) NOT NULL,
  picker_id VARCHAR(36) NOT NULL,
  round_count INT NOT NULL DEFAULT 0,
  max_rounds INT NOT NULL DEFAULT 6,
  revealed TINYINT NOT NULL DEFAULT 0,
  conversation_id VARCHAR(36) NULL,
  closed TINYINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`
