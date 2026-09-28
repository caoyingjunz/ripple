package store

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrValidation   = errors.New("validation failed")
	ErrMaxRounds    = errors.New("max rounds reached")
	ErrThreadClosed = errors.New("thread closed")
)

type Store struct {
	DB        *sql.DB
	MaxRounds int
}

func New(db *sql.DB, maxRounds int) *Store {
	return &Store{DB: db, MaxRounds: maxRounds}
}

func nowMS() int64 { return time.Now().UnixMilli() }

// singleKey 升序拼接 a|b，single 会话幂等去重键
func singleKey(a, b string) string {
	parts := []string{a, b}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

type scanner interface {
	Scan(dest ...any) error
}
