package parse

import (
	"strings"
	"testing"
)

func hdr(d Detail, key string) string {
	for _, kv := range d.Header {
		if kv.Key == key {
			return kv.Value
		}
	}
	return ""
}

func sec(d Detail, title string) (Section, bool) {
	for _, s := range d.Sections {
		if s.Title == title {
			return s, true
		}
	}
	return Section{}, false
}

func TestMonologContextAndExtra(t *testing.T) {
	d := Format(`[2026-11-04T10:11:12.123456+01:00] app.ERROR: Échec paiement {"order":42,"exception":{"class":"X"}} {"url":"/pay"}`)
	if d.Kind != "monolog" || d.Level != "error" || hdr(d, "canal") != "app" || d.Message != "Échec paiement" {
		t.Fatalf("%+v", d)
	}
	c, ok := sec(d, "context")
	if !ok || !strings.Contains(c.JSON, `"order":42`) {
		t.Fatalf("context: %+v", d.Sections)
	}
	if e, ok := sec(d, "extra"); !ok || e.JSON != `{"url":"/pay"}` {
		t.Fatalf("extra: %+v", d.Sections)
	}
}

func TestMonologEmptyArrays(t *testing.T) {
	d := Format(`[2026-11-04 10:11:12] request.INFO: Hello [] []`)
	if d.Kind != "monolog" || d.Message != "Hello" || len(d.Sections) != 0 {
		t.Fatalf("%+v", d)
	}
}

func TestMonologContextOnlyAndBraces(t *testing.T) {
	d := Format(`[2026-11-04 10:11:12] app.WARNING: bad {placeholder} value {"id":7}`)
	if d.Message != "bad {placeholder} value" {
		t.Fatalf("message %q", d.Message)
	}
	if c, ok := sec(d, "context"); !ok || c.JSON != `{"id":7}` {
		t.Fatalf("%+v", d.Sections)
	}
}

func TestMonologInlineJSONInMessage(t *testing.T) {
	d := Format(`[2026-11-04 10:11:12] app.INFO: got {"a":1} from api [] []`)
	if d.Message != "got from api" {
		t.Fatalf("message %q", d.Message)
	}
	if _, ok := sec(d, "JSON dans le message"); !ok {
		t.Fatalf("%+v", d.Sections)
	}
}

func TestAccessCombined(t *testing.T) {
	d := Format(`203.0.113.9 - bob [04/Nov/2026:10:11:12 +0100] "GET /api/x?y=1 HTTP/1.1" 502 153 "https://ex.com/" "curl/8.0"`)
	if d.Kind != "access" || hdr(d, "statut") != "502" || hdr(d, "ip") != "203.0.113.9" ||
		d.Level != "error" || hdr(d, "utilisateur") != "bob" || hdr(d, "user-agent") != "curl/8.0" || d.Message != "GET /api/x?y=1" {
		t.Fatalf("%+v", d)
	}
	d = Format(`10.0.0.1 - - [04/Nov/2026:10:11:12 +0100] "POST /login HTTP/1.1" 200 -`)
	if d.Kind != "access" || hdr(d, "taille") != "-" || hdr(d, "utilisateur") != "" {
		t.Fatalf("common: %+v", d)
	}
}

func TestSyslog(t *testing.T) {
	d := Format(`Nov  4 10:11:12 web1 myapp[1234]: queue full {"len":99}`)
	if d.Kind != "syslog" || hdr(d, "hôte") != "web1" || hdr(d, "programme") != "myapp" || hdr(d, "pid") != "1234" || d.Message != "queue full" {
		t.Fatalf("%+v", d)
	}
	if _, ok := sec(d, "JSON"); !ok {
		t.Fatal("JSON du message non extrait")
	}
	d = Format(`2026-11-04T10:11:12+0100 web1 sshd[99]: Accepted publickey for kevin`)
	if d.Kind != "syslog" || hdr(d, "programme") != "sshd" || d.Message != "Accepted publickey for kevin" {
		t.Fatalf("journald: %+v", d)
	}
}

func TestLogfmt(t *testing.T) {
	d := Format(`time=2026-11-04T10:11:12Z level=warn msg="slow query" duration=1.5s meta={"a":[1,2]} rows=12`)
	if d.Kind != "logfmt" || d.Level != "warn" || d.Message != "slow query" || hdr(d, "date") == "" {
		t.Fatalf("%+v", d)
	}
	f, ok := sec(d, "champs")
	if !ok || len(f.Pairs) != 2 || f.Pairs[0].Key != "duration" {
		t.Fatalf("%+v", d.Sections)
	}
	if m, ok := sec(d, "meta"); !ok || m.JSON != `{"a":[1,2]}` {
		t.Fatalf("%+v", d.Sections)
	}
}

func TestFormatJSONLine(t *testing.T) {
	d := Format(`{"time":"2026-11-04T10:11:12Z","level":"error","msg":"boom","x":1}`)
	if d.Kind != "json" || d.Level != "error" || d.Message != "boom" || len(d.Sections) != 1 {
		t.Fatalf("%+v", d)
	}
}

func TestFallback(t *testing.T) {
	d := Format(`2026-11-04 10:11:12 ERROR something broke payload={"a":{"b":1}}`)
	if d.Kind != "text" || d.Level != "error" || hdr(d, "date") == "" {
		t.Fatalf("%+v", d)
	}
	if _, ok := sec(d, "JSON"); !ok {
		t.Fatalf("%+v", d.Sections)
	}
	d = Format(`plain [12] [INFO] text`)
	if len(d.Sections) != 0 || d.Message != "plain [12] [INFO] text" {
		t.Fatalf("bruit non filtré: %+v", d)
	}
}

func TestMalformedNeverPanics(t *testing.T) {
	for _, s := range []string{
		"", "{", "[", `{"a":`, `[2026-11-04 x] a.ERROR: {"a"`, `"unterminated`, "}{", `a=1`, strings.Repeat("{", 5000),
		`[2026-11-04 10:00:00] a.ERROR: ` + strings.Repeat(`{"a":`, 200),
	} {
		_ = Format(s)
	}
}
