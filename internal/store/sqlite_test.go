package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"birdie/internal/store"
)

// A database from before sessions existed must open and keep its messages.
func TestOpenUpgradesOldDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE messages (
	seq INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE, thread_id TEXT NOT NULL,
	in_reply_to TEXT NOT NULL DEFAULT '', sender TEXT NOT NULL, recipient TEXT NOT NULL, content TEXT NOT NULL,
	meta TEXT NOT NULL DEFAULT '{}', status TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0,
	last_error TEXT NOT NULL DEFAULT '', next_attempt_at INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL, delivered_at INTEGER);
INSERT INTO messages (id, thread_id, sender, recipient, content, status, created_at)
VALUES ('msg_old', 'thr_old', 'a', 'b', 'hello', 'queued', 1);`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	for range 2 { // opening again must not try to add the columns twice
		st, err := store.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		m, err := st.GetMessage(context.Background(), "msg_old")
		if err != nil || m.Content != "hello" || m.ToSession != "" {
			t.Fatalf("m = %+v, %v", m, err)
		}
		st.Close()
	}
}
