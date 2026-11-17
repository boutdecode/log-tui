// Package store persiste les entrées de log dans SQLite (FTS5 trigram).
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kevin/log-tui/internal/parse"
	"github.com/kevin/log-tui/internal/query"

	_ "modernc.org/sqlite"
)

// Store encapsule la base.
type Store struct{ db *sql.DB }

// Row est une entrée relue de la base.
type Row struct {
	ID       int64
	Provider string
	TS       time.Time
	Level    string
	Msg      string
	Raw      string
	Fields   string
}

// MsgCount est un message et son nombre d'occurrences.
type MsgCount struct {
	Msg   string
	Count int
}

const schema = `
CREATE TABLE IF NOT EXISTS entries (
	id       INTEGER PRIMARY KEY,
	provider TEXT NOT NULL,
	ts       INTEGER NOT NULL,
	level    TEXT NOT NULL DEFAULT '',
	msg      TEXT NOT NULL,
	raw      TEXT NOT NULL,
	fields   TEXT
);
CREATE INDEX IF NOT EXISTS entries_ts ON entries(ts);
CREATE INDEX IF NOT EXISTS entries_provider_ts ON entries(provider, ts);
CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
	raw, content='entries', content_rowid='id', tokenize='trigram'
);
CREATE TRIGGER IF NOT EXISTS entries_ai AFTER INSERT ON entries BEGIN
	INSERT INTO entries_fts(rowid, raw) VALUES (new.id, new.raw);
END;
CREATE TRIGGER IF NOT EXISTS entries_ad AFTER DELETE ON entries BEGIN
	INSERT INTO entries_fts(entries_fts, rowid, raw) VALUES ('delete', old.id, old.raw);
END;
CREATE TABLE IF NOT EXISTS provider_state (
	name TEXT PRIMARY KEY,
	data TEXT NOT NULL
);`

// Open ouvre (et crée si besoin) la base. path ":memory:" est accepté pour les tests.
func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		dsn = "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Insert ajoute des entrées pour un provider.
func (s *Store) Insert(provider string, es []parse.Entry) error {
	if len(es) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(`INSERT INTO entries(provider, ts, level, msg, raw, fields) VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, e := range es {
		var fields any
		if e.Fields != "" {
			fields = e.Fields
		}
		if _, err := st.Exec(provider, e.TS.UnixMilli(), e.Level, e.Msg, e.Raw, fields); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Search retourne les limit dernières entrées correspondantes, de la plus ancienne à la plus récente.
func (s *Store) Search(q query.Query, limit int) ([]Row, error) {
	where, args := q.Where(time.Now())
	args = append(args, limit)
	rows, err := s.db.Query(`SELECT id, provider, ts, level, msg, raw, COALESCE(fields,'')
		FROM entries WHERE `+where+` ORDER BY ts DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		var ms int64
		if err := rows.Scan(&r.ID, &r.Provider, &ms, &r.Level, &r.Msg, &r.Raw, &r.Fields); err != nil {
			return nil, err
		}
		r.TS = time.UnixMilli(ms)
		out = append(out, r)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

// Count retourne le nombre d'entrées correspondantes.
func (s *Store) Count(q query.Query) (int, error) {
	where, args := q.Where(time.Now())
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE `+where, args...).Scan(&n)
	return n, err
}

// LevelCounts retourne le nombre d'entrées par niveau ("" = inconnu).
func (s *Store) LevelCounts(q query.Query) (map[string]int, error) {
	where, args := q.Where(time.Now())
	rows, err := s.db.Query(`SELECT level, COUNT(*) FROM entries WHERE `+where+` GROUP BY level`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var l string
		var n int
		if err := rows.Scan(&l, &n); err != nil {
			return nil, err
		}
		out[l] = n
	}
	return out, rows.Err()
}

// Histogram répartit les entrées correspondantes en buckets réguliers entre
// leur première et dernière date.
func (s *Store) Histogram(q query.Query, buckets int) (counts []int, from, to time.Time, err error) {
	where, args := q.Where(time.Now())
	var minTS, maxTS sql.NullInt64
	if err = s.db.QueryRow(`SELECT MIN(ts), MAX(ts) FROM entries WHERE `+where, args...).Scan(&minTS, &maxTS); err != nil {
		return
	}
	counts = make([]int, buckets)
	if !minTS.Valid {
		return
	}
	from, to = time.UnixMilli(minTS.Int64), time.UnixMilli(maxTS.Int64)
	span := maxTS.Int64 - minTS.Int64 + 1
	a := append([]any{minTS.Int64, int64(buckets), span}, args...)
	rows, err := s.db.Query(`SELECT (ts - ?) * ? / ? AS b, COUNT(*) FROM entries WHERE `+where+` GROUP BY b`, a...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var b, n int
		if err = rows.Scan(&b, &n); err != nil {
			return
		}
		if b >= 0 && b < buckets {
			counts[b] = n
		}
	}
	err = rows.Err()
	return
}

// TopMessages retourne les n messages les plus fréquents.
func (s *Store) TopMessages(q query.Query, n int) ([]MsgCount, error) {
	where, args := q.Where(time.Now())
	args = append(args, n)
	rows, err := s.db.Query(`SELECT msg, COUNT(*) c FROM entries WHERE `+where+
		` GROUP BY msg ORDER BY c DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MsgCount
	for rows.Next() {
		var m MsgCount
		if err := rows.Scan(&m.Msg, &m.Count); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ProviderCounts retourne le nombre d'entrées stockées par provider.
func (s *Store) ProviderCounts() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT provider, COUNT(*) FROM entries GROUP BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var p string
		var n int
		if err := rows.Scan(&p, &n); err != nil {
			return nil, err
		}
		out[p] = n
	}
	return out, rows.Err()
}

// Purge supprime les entrées plus anciennes que before.
func (s *Store) Purge(before time.Time) error {
	_, err := s.db.Exec(`DELETE FROM entries WHERE ts < ?`, before.UnixMilli())
	return err
}

// LoadState lit l'état sérialisé d'un provider ("" si absent).
func (s *Store) LoadState(name string) (string, error) {
	var d string
	err := s.db.QueryRow(`SELECT data FROM provider_state WHERE name = ?`, name).Scan(&d)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return d, err
}

// SaveState enregistre l'état sérialisé d'un provider.
func (s *Store) SaveState(name, data string) error {
	_, err := s.db.Exec(`INSERT INTO provider_state(name, data) VALUES (?,?)
		ON CONFLICT(name) DO UPDATE SET data = excluded.data`, name, data)
	return err
}
