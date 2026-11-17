package query

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	q := Parse(`timeout "conn reset" -health level:error,WARN provider:api since:2h user.id:42 12:30`)
	if len(q.Terms) != 3 || q.Terms[1] != "conn reset" || q.Terms[2] != "12:30" {
		t.Fatalf("terms %v", q.Terms)
	}
	if len(q.Not) != 1 || q.Not[0] != "health" {
		t.Fatalf("not %v", q.Not)
	}
	if len(q.Levels) != 2 || q.Levels[1] != "warn" || q.Provider != "api" || q.Since != 2*time.Hour {
		t.Fatalf("%+v", q)
	}
	if len(q.Fields) != 1 || q.Fields[0].Path != "user.id" {
		t.Fatalf("fields %v", q.Fields)
	}
}

func TestWhere(t *testing.T) {
	w, a := Parse(`ab level:error`).Where(time.Now())
	if len(a) != 2 || w == "" {
		t.Fatalf("%q %v", w, a)
	}
}
