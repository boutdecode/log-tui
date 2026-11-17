package store

import (
	"testing"
	"time"

	"github.com/kevin/log-tui/internal/parse"
	"github.com/kevin/log-tui/internal/query"
)

func TestSearch(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	var es []parse.Entry
	for _, l := range []string{
		`{"level":"error","msg":"db timeout","user":{"id":42}}`,
		`{"level":"info","msg":"ok","user":{"id":7}}`,
		`2026-01-01 10:00:00 WARN healthcheck slow`,
		`ab short`,
	} {
		es = append(es, parse.Line(l, now))
	}
	if err := s.Insert("api", es); err != nil {
		t.Fatal(err)
	}
	cases := map[string]int{
		"timeout":          1,
		"TIMEOUT":          1, // trigram insensible à la casse
		"level:error":      1,
		"user.id:42":       1,
		"-healthcheck":     3,
		"ab":               1, // terme court -> LIKE
		"provider:nope":    0,
		"level:warn,error": 2,
		"":                 4,
	}
	for qs, want := range cases {
		rows, err := s.Search(query.Parse(qs), 100)
		if err != nil {
			t.Fatalf("%q: %v", qs, err)
		}
		if len(rows) != want {
			t.Errorf("%q: got %d want %d", qs, len(rows), want)
		}
	}
	if h, _, _, err := s.Histogram(query.Query{}, 10); err != nil || len(h) != 10 {
		t.Fatal(err, h)
	}
	if lc, _ := s.LevelCounts(query.Query{}); lc["error"] != 1 {
		t.Fatal(lc)
	}
	if err := s.SaveState("api", "{}"); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.LoadState("api"); d != "{}" {
		t.Fatal(d)
	}
}
