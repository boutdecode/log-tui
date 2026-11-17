package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kevin/log-tui/internal/config"
	"github.com/kevin/log-tui/internal/scheduler"
	"github.com/kevin/log-tui/internal/store"
)

func TestOnlyFollowedProvidersStart(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Parse([]byte("providers:\n" +
		"  - {name: off, type: local, path: " + filepath.Join(dir, "a.log") + "}\n" +
		"  - {name: on, type: local, path: " + filepath.Join(dir, "b.log") + ", follow: true}\n"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	run, err := scheduler.New(cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	defer run.Close()

	m := New(context.Background(), run, st, "", cfg, Options{})
	if !m.status["off"].paused || m.status["off"].fetching {
		t.Fatal("« off » ne doit pas être suivi au lancement")
	}
	if m.status["on"].paused || !m.status["on"].fetching {
		t.Fatal("« on » doit être suivi")
	}
	if m.noneFollowed() {
		t.Fatal("un provider est suivi")
	}
	// « p » sur « off » le démarre.
	m.sel = 1
	if cmd := m.togglePause(); cmd == nil || m.status["off"].paused || !m.status["off"].fetching {
		t.Fatal("p doit démarrer le suivi")
	}
}
