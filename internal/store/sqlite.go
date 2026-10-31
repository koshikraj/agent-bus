// Package store implements bus.Store on SQLite (pure Go, no cgo), so the
// binary cross-compiles to every platform Go supports.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"agentbus/internal/bus"
)

const schema = `
CREATE TABLE IF NOT EXISTS agents (
	name       TEXT PRIMARY KEY,
	token_hash TEXT NOT NULL UNIQUE,
	delivery   TEXT NOT NULL DEFAULT 'pull',
	config     TEXT NOT NULL DEFAULT '{}',
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
	seq             INTEGER PRIMARY KEY AUTOINCREMENT,
	id              TEXT NOT NULL UNIQUE,
	thread_id       TEXT NOT NULL,
	in_reply_to     TEXT NOT NULL DEFAULT '',
	sender          TEXT NOT NULL,
	recipient       TEXT NOT NULL,
	content         TEXT NOT NULL,
	meta            TEXT NOT NULL DEFAULT '{}',
	status          TEXT NOT NULL,
	attempts        INTEGER NOT NULL DEFAULT 0,
	last_error      TEXT NOT NULL DEFAULT '',
	next_attempt_at INTEGER NOT NULL DEFAULT 0,
	created_at      INTEGER NOT NULL,
	delivered_at    INTEGER
);
CREATE INDEX IF NOT EXISTS messages_inbox ON messages(recipient, status, seq);
CREATE INDEX IF NOT EXISTS messages_thread ON messages(thread_id, seq);
`

const (
	msgCols   = `m.id, m.thread_id, m.in_reply_to, m.sender, m.recipient, m.content, m.meta, m.status, m.attempts, m.created_at`
	agentCols = `a.name, a.token_hash, a.delivery, a.config, a.created_at`
)

type SQLite struct{ db *sql.DB }

var _ bus.Store = (*SQLite)(nil)

func Open(path string) (*SQLite, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	// One connection serializes writers in-process; WAL plus busy_timeout
	// handles the CLI touching the same file concurrently.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return &SQLite{db: db}, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

type scanner interface{ Scan(...any) error }

func scanAgent(r scanner) (bus.Agent, error) {
	var a bus.Agent
	var cfg string
	var created int64
	if err := r.Scan(&a.Name, &a.TokenHash, &a.Delivery, &cfg, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return a, bus.ErrNotFound
		}
		return a, err
	}
	a.CreatedAt = time.UnixMilli(created).UTC()
	return a, json.Unmarshal([]byte(cfg), &a.Config)
}

func scanMessage(r scanner) (bus.Message, error) {
	var m bus.Message
	var meta string
	var created int64
	if err := r.Scan(&m.ID, &m.ThreadID, &m.InReplyTo, &m.From, &m.To, &m.Content, &meta, &m.Status, &m.Attempts, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return m, bus.ErrNotFound
		}
		return m, err
	}
	m.CreatedAt = time.UnixMilli(created).UTC()
	if err := json.Unmarshal([]byte(meta), &m.Meta); err != nil {
		return m, err
	}
	if len(m.Meta) == 0 {
		m.Meta = nil
	}
	return m, nil
}

func collectMessages(rows *sql.Rows, err error) ([]bus.Message, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	msgs := []bus.Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (s *SQLite) CreateAgent(ctx context.Context, a bus.Agent) error {
	cfg, err := json.Marshal(a.Config)
	if err != nil {
		return err
	}
	if a.Config == nil {
		cfg = []byte("{}")
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO agents (name, token_hash, delivery, config, created_at) VALUES (?, ?, ?, ?, ?)`,
		a.Name, a.TokenHash, a.Delivery, string(cfg), a.CreatedAt.UnixMilli())
	return err
}

func (s *SQLite) DeleteAgent(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE name = ?`, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return bus.ErrNotFound
	}
	return nil
}

func (s *SQLite) GetAgent(ctx context.Context, name string) (bus.Agent, error) {
	return scanAgent(s.db.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents a WHERE a.name = ?`, name))
}

func (s *SQLite) AgentByTokenHash(ctx context.Context, hash string) (bus.Agent, error) {
	return scanAgent(s.db.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents a WHERE a.token_hash = ?`, hash))
}

func (s *SQLite) ListAgents(ctx context.Context) ([]bus.Agent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+agentCols+` FROM agents a ORDER BY a.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agents := []bus.Agent{}
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *SQLite) InsertMessage(ctx context.Context, m bus.Message) error {
	meta := []byte("{}")
	if len(m.Meta) > 0 {
		var err error
		if meta, err = json.Marshal(m.Meta); err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO messages (id, thread_id, in_reply_to, sender, recipient, content, meta, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ThreadID, m.InReplyTo, m.From, m.To, m.Content, string(meta), m.Status, m.CreatedAt.UnixMilli())
	return err
}

func (s *SQLite) GetMessage(ctx context.Context, id string) (bus.Message, error) {
	return scanMessage(s.db.QueryRowContext(ctx, `SELECT `+msgCols+` FROM messages m WHERE m.id = ?`, id))
}

func (s *SQLite) ThreadAccess(ctx context.Context, threadID, agent string) (bool, bool, error) {
	var total, mine int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(sender = ? OR recipient = ?), 0) FROM messages WHERE thread_id = ?`,
		agent, agent, threadID).Scan(&total, &mine)
	return total > 0, mine > 0, err
}

func (s *SQLite) Thread(ctx context.Context, threadID, agent string) ([]bus.Message, error) {
	return collectMessages(s.db.QueryContext(ctx,
		`SELECT `+msgCols+` FROM messages m WHERE m.thread_id = ? AND (m.sender = ? OR m.recipient = ?) ORDER BY m.seq`,
		threadID, agent, agent))
}

func (s *SQLite) ClaimInbox(ctx context.Context, agent, threadID string, limit int) ([]bus.Message, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	q := `SELECT ` + msgCols + ` FROM messages m WHERE m.recipient = ? AND m.status = 'queued'`
	args := []any{agent}
	if threadID != "" {
		q += ` AND m.thread_id = ?`
		args = append(args, threadID)
	}
	q += ` ORDER BY m.seq LIMIT ?`
	args = append(args, limit)
	msgs, err := collectMessages(tx.QueryContext(ctx, q, args...))
	if err != nil || len(msgs) == 0 {
		return msgs, err
	}
	now := time.Now().UnixMilli()
	for i := range msgs {
		if _, err := tx.ExecContext(ctx,
			`UPDATE messages SET status = 'delivered', delivered_at = ? WHERE id = ?`, now, msgs[i].ID); err != nil {
			return nil, err
		}
		msgs[i].Status = bus.StatusDelivered
	}
	return msgs, tx.Commit()
}

func (s *SQLite) DueOutbound(ctx context.Context, now time.Time, limit int) ([]bus.Outbound, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+msgCols+`, `+agentCols+` FROM messages m JOIN agents a ON a.name = m.recipient
		 WHERE m.status = 'queued' AND a.delivery != 'pull' AND m.next_attempt_at <= ?
		 ORDER BY m.seq LIMIT ?`, now.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []bus.Outbound
	for rows.Next() {
		var o bus.Outbound
		var meta, cfg string
		var mCreated, aCreated int64
		m, a := &o.Message, &o.Agent
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.InReplyTo, &m.From, &m.To, &m.Content, &meta, &m.Status, &m.Attempts, &mCreated,
			&a.Name, &a.TokenHash, &a.Delivery, &cfg, &aCreated); err != nil {
			return nil, err
		}
		m.CreatedAt, a.CreatedAt = time.UnixMilli(mCreated).UTC(), time.UnixMilli(aCreated).UTC()
		if err := json.Unmarshal([]byte(meta), &m.Meta); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(cfg), &a.Config); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *SQLite) MarkDelivered(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE messages SET status = 'delivered', delivered_at = ?, last_error = '' WHERE id = ?`, at.UnixMilli(), id)
	return err
}

func (s *SQLite) RecordFailure(ctx context.Context, id, errMsg string, next time.Time, failed bool) error {
	status := bus.StatusQueued
	if failed {
		status = bus.StatusFailed
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE messages SET attempts = attempts + 1, last_error = ?, next_attempt_at = ?, status = ? WHERE id = ?`,
		errMsg, next.UnixMilli(), status, id)
	return err
}
