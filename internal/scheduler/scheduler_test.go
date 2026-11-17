package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevin/log-tui/internal/config"
	"github.com/kevin/log-tui/internal/store"
)

func TestFetchedMsgCountsErrors(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "a.log")
	os.WriteFile(log, []byte(
		`{"level":"info","msg":"ok"}`+"\n"+
			`{"level":"error","msg":"first boom"}`+"\n"+
			"2026-11-04 10:00:00 FATAL second boom\n"+
			`{"level":"warn","msg":"meh"}`+"\n"), 0o644)

	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r, err := New(&config.Config{Providers: []config.Provider{{
		Name: "a", Type: "local", Path: log, Interval: time.Second, InitialBytes: 1 << 20,
	}}}, st)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	msg := r.Fetch(context.Background(), "a", 0)().(FetchedMsg)
	if msg.Err != nil || msg.New != 4 || msg.Errors != 2 || msg.Sample != "first boom" {
		t.Fatalf("%+v", msg)
	}
	if msg = r.Fetch(context.Background(), "a", 0)().(FetchedMsg); msg.New != 0 || msg.Errors != 0 {
		t.Fatalf("rien de neuf attendu: %+v", msg)
	}
}
