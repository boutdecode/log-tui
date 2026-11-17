package provider

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kevin/log-tui/internal/config"
)

func appendFile(t *testing.T, p, s string) {
	t.Helper()
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(s)
	f.Close()
}

func TestLocalIncrementalAndRotation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.log")
	l := &Local{cfg: config.Provider{Path: p, InitialBytes: 1 << 20}}
	st := &State{}
	ctx := context.Background()

	appendFile(t, p, "one\ntwo\npart")
	got, err := l.Fetch(ctx, st)
	if err != nil || !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatal(got, err)
	}
	appendFile(t, p, "ial\nthree\n")
	got, _ = l.Fetch(ctx, st)
	if !reflect.DeepEqual(got, []string{"partial", "three"}) {
		t.Fatal(got)
	}
	if got, _ = l.Fetch(ctx, st); len(got) != 0 {
		t.Fatal("rien de neuf attendu", got)
	}
	// Rotation : nouveau fichier plus petit.
	os.WriteFile(p, []byte("new\n"), 0o644)
	got, _ = l.Fetch(ctx, st)
	if !reflect.DeepEqual(got, []string{"new"}) {
		t.Fatal("rotation", got)
	}
}

func TestLocalInitialTail(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.log")
	appendFile(t, p, "aaaaaaaa\nbbbb\ncccc\n")
	l := &Local{cfg: config.Provider{Path: p, InitialBytes: 12}}
	got, _ := l.Fetch(context.Background(), &State{})
	if !reflect.DeepEqual(got, []string{"bbbb", "cccc"}) {
		t.Fatal(got)
	}
}

func TestDedupeTail(t *testing.T) {
	st := &State{}
	if got := dedupeTail([]string{"a", "b", "c"}, st); len(got) != 3 {
		t.Fatal(got)
	}
	if got := dedupeTail([]string{"b", "c", "d", "e"}, st); !reflect.DeepEqual(got, []string{"d", "e"}) {
		t.Fatal(got)
	}
	if got := dedupeTail([]string{"b", "c", "d", "e"}, st); len(got) != 0 {
		t.Fatal(got)
	}
	if got := dedupeTail([]string{"x", "y"}, st); len(got) != 2 {
		t.Fatal(got)
	}
}
