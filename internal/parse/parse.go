// Package parse transforme une ligne de log brute en Entry structurée.
// Si la ligne contient un objet JSON (en début de ligne ou après un préfixe
// texte), il est décodé et ses champs usuels (heure, niveau, message) extraits.
package parse

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Entry est une ligne de log analysée.
type Entry struct {
	TS     time.Time
	Level  string // trace|debug|info|warn|error|fatal ou ""
	Msg    string
	Raw    string
	Fields string // texte JSON de l'objet trouvé, "" sinon
}

var (
	tsKeys    = []string{"time", "ts", "timestamp", "@timestamp", "datetime", "date", "t"}
	levelKeys = []string{"level", "lvl", "severity", "loglevel", "log.level"}
	msgKeys   = []string{"msg", "message", "event", "log", "text"}

	tsRe    = regexp.MustCompile(`^\s*\[?(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?)`)
	levelRe = regexp.MustCompile(`\b(TRACE|DEBUG|INFO|NOTICE|WARN|WARNING|ERR|ERROR|CRIT|CRITICAL|FATAL|PANIC)\b`)

	layouts = []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.999999999-0700",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999-0700",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02 15:04:05.999999999",
	}
)

// Line analyse une ligne brute. now sert de repli quand aucune date n'est trouvée.
func Line(raw string, now time.Time) Entry {
	raw = strings.TrimRight(raw, "\r\n")
	e := Entry{Raw: raw, Msg: strings.TrimSpace(raw)}

	prefix := raw
	if i := strings.IndexByte(raw, '{'); i >= 0 {
		cand := strings.TrimSpace(raw[i:])
		var obj map[string]any
		if json.Unmarshal([]byte(cand), &obj) == nil {
			e.Fields = cand
			prefix = raw[:i]
			if v, ok := pick(obj, tsKeys); ok {
				e.TS = toTime(v)
			}
			if v, ok := pick(obj, levelKeys); ok {
				e.Level = toLevel(v)
			}
			if v, ok := pick(obj, msgKeys); ok {
				if s, ok := v.(string); ok && s != "" {
					e.Msg = s
				}
			}
		}
	}
	tsLoc := tsRe.FindStringSubmatchIndex(prefix)
	if e.TS.IsZero() && tsLoc != nil {
		e.TS = parseTimeString(prefix[tsLoc[2]:tsLoc[3]])
	}
	lvlLoc := levelRe.FindStringIndex(prefix)
	if e.Level == "" && lvlLoc != nil {
		e.Level = NormLevel(prefix[lvlLoc[0]:lvlLoc[1]])
	}
	if e.Fields == "" {
		e.Msg = stripHeader(raw, tsLoc, lvlLoc)
	}
	if e.TS.IsZero() {
		e.TS = now
	}
	return e
}

// stripHeader retire de la ligne l'horodatage de tête et le niveau (s'il est
// situé juste après) pour ne garder que le message.
func stripHeader(raw string, tsLoc, lvlLoc []int) string {
	rest := raw
	if tsLoc != nil {
		rest = raw[tsLoc[1]:]
		rest = strings.TrimLeft(rest, " ]-:")
	}
	if lvlLoc != nil {
		head := levelRe.FindStringIndex(rest)
		// Le niveau est ôté seulement s'il ouvre le message (éventuellement entre [] ou suivi de :).
		if head != nil && strings.Trim(rest[:head[0]], " [(") == "" {
			rest = strings.TrimLeft(rest[head[1]:], " ]):-")
		}
	}
	if rest = strings.TrimSpace(rest); rest != "" {
		return rest
	}
	return strings.TrimSpace(raw)
}

func pick(obj map[string]any, keys []string) (any, bool) {
	for _, k := range keys {
		if v, ok := obj[k]; ok && v != nil {
			return v, true
		}
	}
	return nil, false
}

// NormLevel normalise un nom de niveau.
func NormLevel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace", "verbose":
		return "trace"
	case "debug", "dbg":
		return "debug"
	case "info", "notice", "information":
		return "info"
	case "warn", "warning":
		return "warn"
	case "err", "error":
		return "error"
	case "crit", "critical", "fatal", "panic", "emerg", "emergency", "alert":
		return "fatal"
	}
	return ""
}

func toLevel(v any) string {
	switch x := v.(type) {
	case string:
		return NormLevel(x)
	case float64: // pino / bunyan
		switch {
		case x <= 10:
			return "trace"
		case x <= 20:
			return "debug"
		case x <= 30:
			return "info"
		case x <= 40:
			return "warn"
		case x <= 50:
			return "error"
		default:
			return "fatal"
		}
	}
	return ""
}

func toTime(v any) time.Time {
	switch x := v.(type) {
	case string:
		return parseTimeString(x)
	case float64:
		switch {
		case x > 1e17:
			return time.Unix(0, int64(x))
		case x > 1e14:
			return time.UnixMicro(int64(x))
		case x > 1e11:
			return time.UnixMilli(int64(x))
		case x > 1e8:
			sec := int64(x)
			return time.Unix(sec, int64((x-float64(sec))*1e9))
		}
	}
	return time.Time{}
}

func parseTimeString(s string) time.Time {
	s = strings.Replace(strings.TrimSpace(s), ",", ".", 1)
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t
		}
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return toTime(f)
	}
	return time.Time{}
}

// IsMetaKey indique si une clé JSON est déjà représentée par l'heure, le
// niveau ou le message de l'Entry.
func IsMetaKey(k string) bool {
	for _, set := range [][]string{tsKeys, levelKeys, msgKeys} {
		for _, x := range set {
			if x == k {
				return true
			}
		}
	}
	return false
}
