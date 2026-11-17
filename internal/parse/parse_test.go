package parse

import (
	"testing"
	"time"
)

var now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestJSONLine(t *testing.T) {
	e := Line(`{"time":"2026-03-04T10:11:12Z","level":"WARNING","msg":"disk almost full","pct":91}`, now)
	if e.Level != "warn" || e.Msg != "disk almost full" || e.Fields == "" {
		t.Fatalf("%+v", e)
	}
	if !e.TS.Equal(time.Date(2026, 3, 4, 10, 11, 12, 0, time.UTC)) {
		t.Fatalf("ts %v", e.TS)
	}
}

func TestPrefixedJSON(t *testing.T) {
	e := Line(`2026-03-04 10:11:12,345 ERROR {"msg":"boom","code":500}`, now)
	if e.Level != "error" || e.Msg != "boom" || e.Fields != `{"msg":"boom","code":500}` {
		t.Fatalf("%+v", e)
	}
	if e.TS.Year() != 2026 || e.TS.Nanosecond() != 345e6 {
		t.Fatalf("ts %v", e.TS)
	}
}

func TestStripHeader(t *testing.T) {
	cases := map[string]string{
		"2026-03-04 10:11:12 WARN upstream slow id=3":            "upstream slow id=3",
		"[2026-03-04T10:11:12Z] [ERROR] boom":                    "boom",
		"2026-03-04 10:11:12 service started with ERROR in name": "service started with ERROR in name",
		"plain line": "plain line",
	}
	for in, want := range cases {
		if got := Line(in, now).Msg; got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestPino(t *testing.T) {
	e := Line(`{"level":50,"time":1772619072000,"msg":"x"}`, now)
	if e.Level != "error" || e.TS.Year() != 2026 {
		t.Fatalf("%+v", e)
	}
}

func TestPlain(t *testing.T) {
	e := Line("hello {not json", now)
	if e.Fields != "" || e.Level != "" || !e.TS.Equal(now) || e.Msg != "hello {not json" {
		t.Fatalf("%+v", e)
	}
	e = Line("[2026-03-04T10:11:12+01:00] INFO started", now)
	if e.Level != "info" || e.TS.UTC().Hour() != 9 {
		t.Fatalf("%+v", e)
	}
}
