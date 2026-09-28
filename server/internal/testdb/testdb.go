// Package testdb 提供 ripple_test 测试库的打开/DROP/重建逻辑。
// 共享实例红线：只允许 ripple_test 库；DSN 未指向 ripple_test 时直接失败。
package testdb

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ripple/server/internal/db"

	"golang.org/x/sys/unix"
)

// Open 打开 ripple_test：加跨进程文件锁（go test ./... 并行跑多个包，测试表共享），
// DROP 全部业务表后重建 schema。RIPPLE_TEST_MYSQL_DSN 未设置时 Skip。
func Open(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("RIPPLE_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RIPPLE_TEST_MYSQL_DSN not set; skipping MySQL-backed test")
	}
	if !strings.Contains(dsn, "ripple_test") {
		t.Fatalf("RIPPLE_TEST_MYSQL_DSN must point at ripple_test database, got: %s", dsn)
	}
	// 跨进程串行化：同一时刻只有一个测试进程占用 ripple_test
	lockPath := filepath.Join(os.TempDir(), "ripple_test_db.lock")
	lockFile, err := os.Create(lockPath)
	if err != nil {
		t.Fatalf("create lock file: %v", err)
	}
	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX); err != nil {
		t.Fatalf("flock: %v", err)
	}
	t.Cleanup(func() {
		_ = unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)
		_ = lockFile.Close()
	})

	sqlDB, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open ripple_test: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if _, err := sqlDB.Exec(
		`DROP TABLE IF EXISTS messages, conversation_members, conversations, bottle_threads, bottles, friendships, users`,
	); err != nil {
		t.Fatalf("drop tables: %v", err)
	}
	if err := db.Migrate(sqlDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return sqlDB
}
