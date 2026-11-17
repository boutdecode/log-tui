package tui

import (
	"strings"
	"testing"
)

func TestResetsBackground(t *testing.T) {
	yes := []string{"", "0", "49", "1;49", "38;5;99;49", "0;1"}
	no := []string{"1", "38;5;49", "48;5;49", "38;2;49;49;49", "31", "7"}
	for _, p := range yes {
		if !resetsBackground(p) {
			t.Errorf("%q devrait réinitialiser le fond", p)
		}
	}
	for _, p := range no {
		if resetsBackground(p) {
			t.Errorf("%q ne devrait pas réinitialiser le fond", p)
		}
	}
}

func TestPaintBackground(t *testing.T) {
	bg := "\x1b[48;5;234m"
	got := paintBackground("a\x1b[31mb\x1b[0mc\x1b[39m\x1b[49md", bg)
	want := bg + "a\x1b[31mb\x1b[0m" + bg + "c\x1b[39m\x1b[49m" + bg + "d\x1b[0m"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if strings.Count(paintBackground("plain", bg), bg) != 1 {
		t.Fatal("fond appliqué une seule fois sur du texte brut")
	}
}
