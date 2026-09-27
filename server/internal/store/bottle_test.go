package store_test

import (
	"path/filepath"
	"testing"

	"ripple/server/internal/db"
	"ripple/server/internal/store"
)

func TestPickAndReveal(t *testing.T) {
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.Migrate(sqlDB); err != nil {
		t.Fatal(err)
	}
	if err := db.SeedDemo(sqlDB); err != nil {
		t.Fatal(err)
	}
	st := store.New(sqlDB, 6)
	b, err := st.ThrowBottle("u-alice", "hello sea")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "floating" {
		t.Fatalf("status %s", b.Status)
	}
	thread, err := st.PickBottle("u-bob")
	if err != nil {
		t.Fatal(err)
	}
	if thread.MyAlias != "捡瓶人" {
		t.Fatalf("alias %s", thread.MyAlias)
	}
	if len(thread.Messages) != 1 || thread.Messages[0].Body != "hello sea" {
		t.Fatalf("messages %+v", thread.Messages)
	}
	_, err = st.PickBottle("u-carol")
	if err != store.ErrNotFound {
		t.Fatalf("expected no bottles, got %v", err)
	}
	m, _, _, err := st.AddBottleMessage(thread.ID, "u-bob", "hi back")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Mine || m.Alias != "捡瓶人" {
		t.Fatalf("%+v", m)
	}
	conv, peer, user, err := st.Reveal(thread.ID, "u-alice")
	if err != nil {
		t.Fatal(err)
	}
	if conv == "" || peer != "u-bob" || user.Username != "bob" {
		t.Fatalf("%s %s %+v", conv, peer, user)
	}
	conv2, _, _, err := st.Reveal(thread.ID, "u-bob")
	if err != nil || conv2 != conv {
		t.Fatalf("idempotent reveal %v %s", err, conv2)
	}
}
