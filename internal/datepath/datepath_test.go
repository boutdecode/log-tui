package datepath

import (
	"testing"
	"time"
)

func TestExpand(t *testing.T) {
	ts := time.Date(2026, 11, 4, 7, 5, 0, 0, time.UTC)
	cases := map[string]string{
		"/var/log/prod-%Y-%m-%d.log": "/var/log/prod-2026-11-04.log",
		"app-%y%m%d-%H.log":          "app-261104-07.log",
		"x-%j.log":                   "x-308.log",
		"100%%.log":                  "100%.log",
		"plain.log":                  "plain.log",
	}
	for in, want := range cases {
		if got := Expand(in, ts); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestCheckAndHas(t *testing.T) {
	for _, bad := range []string{"a-%Q.log", "a-%"} {
		if Check(bad) == nil {
			t.Errorf("%q devrait être invalide", bad)
		}
	}
	if Check("a-%Y.log") != nil || Check("100%%") != nil {
		t.Error("motifs valides rejetés")
	}
	if !Has("a-%d") || Has("a-%%d") || Has("plain") {
		t.Error("Has")
	}
}
