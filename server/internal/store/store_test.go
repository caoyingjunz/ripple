package store_test

import (
	"sync"
	"testing"

	"ripple/server/internal/db"
	"ripple/server/internal/store"
	"ripple/server/internal/testdb"
)

// TestSeqMonotonic 并发写同一会话：seq 严格单调递增、无重号无空洞（LAST_INSERT_ID 原子取号）
func TestSeqMonotonic(t *testing.T) {
	sqlDB := testdb.Open(t)
	if err := db.SeedDemo(sqlDB); err != nil {
		t.Fatal(err)
	}
	st := store.New(sqlDB, 6)
	convID, created, err := st.GetOrCreateSingle("u-alice", "u-bob")
	if err != nil || !created {
		t.Fatalf("create single conv: %v created=%v", err, created)
	}
	const workers, perWorker = 10, 10
	var mu sync.Mutex
	seqs := make(map[int64]bool)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				m, err := st.InsertMessage(convID, "u-alice", "text", "hi", nil)
				if err != nil {
					t.Errorf("insert: %v", err)
					return
				}
				mu.Lock()
				if seqs[m.Seq] {
					t.Errorf("duplicate seq %d", m.Seq)
				}
				seqs[m.Seq] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seqs) != workers*perWorker {
		t.Fatalf("expected %d unique seqs, got %d", workers*perWorker, len(seqs))
	}
	for i := int64(1); i <= workers*perWorker; i++ {
		if !seqs[i] {
			t.Fatalf("seq %d missing (gap)", i)
		}
	}
}

// TestFriendStateMachine 好友状态机：申请→秒通过→删除→拉黑→解除
func TestFriendStateMachine(t *testing.T) {
	sqlDB := testdb.Open(t)
	if err := db.SeedDemo(sqlDB); err != nil {
		t.Fatal(err)
	}
	st := store.New(sqlDB, 6)

	// carol→dave 申请，dave→carol 反向申请自动 accepted
	_, _, _, status, err := st.AddFriend("u-carol", "u-dave")
	if err != nil || status != "pending" {
		t.Fatalf("carol request: %v status=%s", err, status)
	}
	_, _, friend, status, err := st.AddFriend("u-dave", "u-carol")
	if err != nil || status != "accepted" || friend.Username != "carol" {
		t.Fatalf("dave auto-accept: %v status=%s friend=%+v", err, status, friend)
	}
	// 已好友再申请 → 409
	if _, _, _, _, err := st.AddFriend("u-dave", "u-carol"); err != store.ErrConflict {
		t.Fatalf("re-request should conflict, got %v", err)
	}

	// 删好友 → none；再次互加又可 pending
	if err := st.RemoveFriend("u-dave", "u-carol"); err != nil {
		t.Fatal(err)
	}
	_, _, _, status, err = st.AddFriend("u-alice", "u-bob")
	if err != nil || status != "pending" {
		t.Fatalf("alice request: %v status=%s", err, status)
	}
	// bob 屏蔽 alice：pending 行 upsert 为 blocked，反向行删除
	if err := st.BlockUser("u-bob", "u-alice"); err != nil {
		t.Fatal(err)
	}
	// 再屏蔽 → 409
	if err := st.BlockUser("u-bob", "u-alice"); err != store.ErrConflict {
		t.Fatalf("double block should conflict, got %v", err)
	}
	// 黑名单拦截 single 消息（双向）
	convID, _, err := st.GetOrCreateSingle("u-alice", "u-bob")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertMessage(convID, "u-alice", "text", "blocked?", nil); err != store.ErrForbidden {
		t.Fatalf("blocked send should be forbidden, got %v", err)
	}
	if _, err := st.InsertMessage(convID, "u-bob", "text", "blocked?", nil); err != store.ErrForbidden {
		t.Fatalf("blocker send should be forbidden, got %v", err)
	}
	// 解除 → 回到 none，可继续发
	if err := st.UnblockUser("u-bob", "u-alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertMessage(convID, "u-bob", "text", "open again", nil); err != nil {
		t.Fatalf("send after unblock: %v", err)
	}
}
