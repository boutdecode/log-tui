package notify

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func stub(t *testing.T, env map[string]string, havePath bool, runErr error) *[][]string {
	t.Helper()
	var calls [][]string
	oldL, oldG, oldR := lookPath, getenv, run
	t.Cleanup(func() { lookPath, getenv, run = oldL, oldG, oldR })
	getenv = func(k string) string { return env[k] }
	lookPath = func(string) (string, error) {
		if havePath {
			return "/usr/bin/notify-send", nil
		}
		return "", errors.New("absent")
	}
	run = func(_ context.Context, name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return runErr
	}
	return &calls
}

func TestSanitize(t *testing.T) {
	got := Sanitize("a\x1b[31mb\x07c\n\td   e\x1b]0;x\x07", 100)
	if strings.ContainsAny(got, "\x1b\x07\n\t") || got != "ab c d e" {
		t.Fatalf("%q", got)
	}
	if got := Sanitize(strings.Repeat("é", 300), 10); len([]rune(got)) != 10 || !strings.HasSuffix(got, "…") {
		t.Fatalf("%q", got)
	}
}

func TestDesktopFirstWithSafeArgs(t *testing.T) {
	calls := stub(t, nil, true, nil)
	if seq := Deliver("-t evil", "boom\x07"); seq != "" {
		t.Fatalf("pas de repli attendu: %q", seq)
	}
	want := []string{"/usr/bin/notify-send", "-a", "log-tui", "-u", "critical", "-i", "dialog-error", "--", "-t evil", "boom"}
	if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], want) {
		t.Fatalf("%v", *calls)
	}
}

func TestFallbacks(t *testing.T) {
	// notify-send en échec
	calls := stub(t, nil, true, errors.New("no dbus"))
	if seq := Deliver("t", "b"); !strings.HasPrefix(seq, "\x1b]9;t — b\x07") || !strings.HasSuffix(seq, "\a") || len(*calls) != 1 {
		t.Fatalf("%q", seq)
	}
	// notify-send absent
	stub(t, nil, false, nil)
	if seq := Deliver("t", "b"); seq == "" {
		t.Fatal("repli attendu")
	}
	// session SSH : jamais de notify-send
	calls = stub(t, map[string]string{"SSH_CONNECTION": "1 2 3 4"}, true, nil)
	if seq := Deliver("t", "b"); seq == "" || len(*calls) != 0 {
		t.Fatalf("SSH: %q %v", seq, *calls)
	}
	// kitty : OSC 99
	stub(t, map[string]string{"SSH_TTY": "/dev/pts/1", "KITTY_WINDOW_ID": "3"}, false, nil)
	if seq := Deliver("t", "b"); !strings.Contains(seq, "\x1b]99;") || strings.Contains(seq, "\x1b]9;") {
		t.Fatalf("kitty: %q", seq)
	}
}
