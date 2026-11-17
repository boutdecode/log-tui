package parse

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// KV est une paire clé/valeur affichable.
type KV struct{ Key, Value string }

// Section est un bloc du détail : soit un JSON brut à indenter, soit des paires.
type Section struct {
	Title string
	JSON  string
	Pairs []KV
}

// Detail est la vue structurée d'une ligne de log, destinée au panneau de détail.
type Detail struct {
	Kind     string // monolog | access | syslog | logfmt | json | text
	Level    string // niveau normalisé si le format le fournit
	Header   []KV
	Message  string
	Sections []Section
}

const maxFormatLen = 64 << 10

var (
	monologRe = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2}[^\]]*)\]\s+([A-Za-z0-9_.\-]+)\.([A-Za-z]+):\s?(.*)$`)
	accessRe  = regexp.MustCompile(`^(\S+) (\S+) (\S+) \[([^\]]+)\] "([A-Z]+) (\S+)(?: (HTTP/[\d.]+))?" (\d{3}) (\d+|-)(?: "([^"]*)" "([^"]*)")?(.*)$`)
	syslogRe  = regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d{1,2} \d{2}:\d{2}:\d{2}) (\S+) ([^\s:\[]+)(?:\[(\d+)\])?: ?(.*)$`)
	journalRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?) (\S+) ([^\s:\[]+)(?:\[(\d+)\])?: ?(.*)$`)
	logfmtKey = regexp.MustCompile(`^([A-Za-z_][\w.\-]*)=`)
)

// Format analyse une ligne brute et retourne sa vue structurée.
func Format(raw string) Detail {
	raw = strings.TrimRight(raw, "\r\n")
	trimmed := strings.TrimSpace(raw)
	if len(raw) > maxFormatLen {
		return Detail{Kind: "text", Message: raw}
	}
	if d, ok := formatJSONLine(trimmed); ok {
		return d
	}
	if d, ok := formatMonolog(trimmed); ok {
		return d
	}
	if d, ok := formatAccess(trimmed); ok {
		return d
	}
	if d, ok := formatSyslog(trimmed); ok {
		return d
	}
	if d, ok := formatLogfmt(trimmed); ok {
		return d
	}
	return formatFallback(trimmed)
}

// --- JSON ------------------------------------------------------------------

type span struct{ start, end int }

// scanJSON retourne les valeurs JSON ({...} ou [...]) valides contenues dans s,
// de gauche à droite, sans chevauchement.
func scanJSON(s string) []span {
	var out []span
	fails := 0 // borne le coût sur des lignes pathologiques ({{{{…)
	for i := 0; i < len(s) && fails < 256; {
		if s[i] != '{' && s[i] != '[' {
			i++
			continue
		}
		if end := matchBracket(s, i); end > 0 && json.Valid([]byte(s[i:end])) {
			out = append(out, span{i, end})
			i = end
			continue
		}
		fails++
		i++
	}
	return out
}

// matchBracket retourne l'index qui suit le crochet/accolade fermant
// correspondant à celui en s[i], ou -1.
func matchBracket(s string, i int) int {
	depth, inStr := 0, false
	for j := i; j < len(s); j++ {
		c := s[j]
		if inStr {
			switch c {
			case '\\':
				j++
			case '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return j + 1
			}
		}
	}
	return -1
}

func isEmptyJSON(s string) bool {
	switch strings.TrimSpace(s) {
	case "{}", "[]":
		return true
	}
	return false
}

// worthShowing écarte le bruit dans un texte libre : « [12] » ou « [INFO] »
// ne sont pas des données ; on garde les objets et les tableaux composites.
func worthShowing(doc string) bool {
	if isEmptyJSON(doc) {
		return false
	}
	if doc[0] == '{' {
		return true
	}
	var arr []json.RawMessage
	if json.Unmarshal([]byte(doc), &arr) != nil {
		return false
	}
	for _, e := range arr {
		if len(e) > 0 && (e[0] == '{' || e[0] == '[') {
			return true
		}
	}
	return false
}

// splitInlineJSON sépare le texte des valeurs JSON qu'il contient : retourne le
// texte sans ces valeurs et une section par valeur.
func splitInlineJSON(s string) (string, []Section) {
	var secs []Section
	var b strings.Builder
	last := 0
	for _, sp := range scanJSON(s) {
		doc := s[sp.start:sp.end]
		if !worthShowing(doc) {
			continue
		}
		b.WriteString(s[last:sp.start])
		last = sp.end
		secs = append(secs, Section{Title: "JSON", JSON: doc})
	}
	b.WriteString(s[last:])
	return strings.Join(strings.Fields(b.String()), " "), secs
}

func formatJSONLine(line string) (Detail, bool) {
	if line == "" || line[0] != '{' || !json.Valid([]byte(line)) {
		return Detail{}, false
	}
	d := Detail{Kind: "json", Sections: []Section{{Title: "JSON", JSON: line}}}
	var obj map[string]any
	if json.Unmarshal([]byte(line), &obj) == nil {
		if v, ok := pick(obj, tsKeys); ok {
			if t := toTime(v); !t.IsZero() {
				d.Header = append(d.Header, KV{"date", t.Format("2006-01-02 15:04:05.000")})
			}
		}
		if v, ok := pick(obj, levelKeys); ok {
			if l := toLevel(v); l != "" {
				d.Level = l
				d.Header = append(d.Header, KV{"level", l})
			}
		}
		if v, ok := pick(obj, msgKeys); ok {
			if s, ok := v.(string); ok {
				d.Message = s
			}
		}
	}
	return d, true
}

// --- Monolog / Laravel / Symfony ------------------------------------------

func formatMonolog(line string) (Detail, bool) {
	m := monologRe.FindStringSubmatch(line)
	if m == nil {
		return Detail{}, false
	}
	level := NormLevel(m[3])
	if level == "" {
		return Detail{}, false
	}
	d := Detail{Kind: "monolog", Level: level, Header: []KV{
		{"date", m[1]}, {"channel", m[2]}, {"level", strings.ToUpper(m[3])},
	}}
	rest := m[4]

	// Les deux derniers JSON de fin de ligne sont context puis extra.
	spans := scanJSON(rest)
	pos := len(strings.TrimRight(rest, " \t"))
	var run []span
	for k := len(spans) - 1; k >= 0 && len(run) < 2; k-- {
		if strings.TrimSpace(rest[spans[k].end:pos]) != "" {
			break
		}
		run = append([]span{spans[k]}, run...)
		pos = spans[k].start
	}
	msgText := rest
	if len(run) > 0 {
		msgText = rest[:run[0].start]
		titles := []string{"context", "extra"}
		if len(run) == 1 {
			titles = []string{"context"}
		}
		for i, sp := range run {
			doc := rest[sp.start:sp.end]
			if !isEmptyJSON(doc) {
				d.Sections = append(d.Sections, Section{Title: titles[i], JSON: doc})
			}
		}
	}
	text, inline := splitInlineJSON(msgText)
	for i := range inline {
		inline[i].Title = "inline"
	}
	d.Message = text
	d.Sections = append(d.Sections, inline...)
	return d, true
}

// --- access log nginx / Apache ----------------------------------------------

func formatAccess(line string) (Detail, bool) {
	m := accessRe.FindStringSubmatch(line)
	if m == nil {
		return Detail{}, false
	}
	d := Detail{Kind: "access", Message: m[5] + " " + m[6]}
	switch m[8][0] { // niveau déduit de la classe de statut
	case '5':
		d.Level = "error"
	case '4':
		d.Level = "warn"
	default:
		d.Level = "info"
	}
	d.Header = append(d.Header, KV{"ip", m[1]})
	if m[3] != "-" {
		d.Header = append(d.Header, KV{"user", m[3]})
	}
	d.Header = append(d.Header, KV{"date", m[4]}, KV{"method", m[5]})
	if m[7] != "" {
		d.Header = append(d.Header, KV{"protocol", m[7]})
	}
	d.Header = append(d.Header, KV{"status", m[8]}, KV{"size", m[9]})
	if m[10] != "" && m[10] != "-" {
		d.Header = append(d.Header, KV{"referer", m[10]})
	}
	if m[11] != "" {
		d.Header = append(d.Header, KV{"user-agent", m[11]})
	}
	if extra := strings.TrimSpace(m[12]); extra != "" {
		d.Header = append(d.Header, KV{"rest", extra})
	}
	return d, true
}

// --- syslog / journald ------------------------------------------------------

func formatSyslog(line string) (Detail, bool) {
	m := syslogRe.FindStringSubmatch(line)
	if m == nil {
		m = journalRe.FindStringSubmatch(line)
	}
	if m == nil {
		return Detail{}, false
	}
	d := Detail{Kind: "syslog", Header: []KV{{"date", m[1]}, {"host", m[2]}, {"program", m[3]}}}
	if m[4] != "" {
		d.Header = append(d.Header, KV{"pid", m[4]})
	}
	text, secs := splitInlineJSON(m[5])
	d.Message, d.Sections = text, secs
	return d, true
}

// --- logfmt -------------------------------------------------------------------

// logfmtPairs découpe une ligne « clé=valeur ... ». Une valeur peut être entre
// guillemets ou être un JSON ({...} / [...]) contenant des espaces. ok est faux
// si du texte libre entrecoupe les paires (ce n'est alors pas du logfmt).
func logfmtPairs(line string) (pairs [][2]string, covered int) {
	i := 0
	for i < len(line) {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= len(line) {
			break
		}
		m := logfmtKey.FindStringSubmatch(line[i:])
		if m == nil { // mot sans « = » : on le saute
			for i < len(line) && line[i] != ' ' && line[i] != '\t' {
				i++
			}
			continue
		}
		start := i
		i += len(m[0])
		vstart := i
		switch {
		case i < len(line) && line[i] == '"':
			j := i + 1
			for j < len(line) && line[j] != '"' {
				if line[j] == '\\' {
					j++
				}
				j++
			}
			i = min(j+1, len(line))
		case i < len(line) && (line[i] == '{' || line[i] == '['):
			if end := matchBracket(line, i); end > 0 {
				i = end
				break
			}
			fallthrough
		default:
			for i < len(line) && line[i] != ' ' && line[i] != '\t' {
				i++
			}
		}
		pairs = append(pairs, [2]string{m[1], line[vstart:i]})
		covered += i - start
	}
	return pairs, covered
}

func formatLogfmt(line string) (Detail, bool) {
	ms, covered := logfmtPairs(line)
	if len(ms) < 3 || covered*2 < len(line) || !logfmtKey.MatchString(line) {
		return Detail{}, false
	}
	d := Detail{Kind: "logfmt"}
	var pairs []KV
	for _, m := range ms {
		key, val := m[0], m[1]
		if strings.HasPrefix(val, `"`) {
			if u, err := strconv.Unquote(val); err == nil {
				val = u
			}
		}
		switch {
		case inList(key, tsKeys) && d.Header == nil:
			d.Header = append(d.Header, KV{"date", val})
		case inList(key, levelKeys):
			d.Header = append(d.Header, KV{"level", val})
			d.Level = NormLevel(val)
		case inList(key, msgKeys) && d.Message == "":
			d.Message = val
		case len(val) > 1 && (val[0] == '{' || val[0] == '[') && json.Valid([]byte(val)):
			if !isEmptyJSON(val) {
				d.Sections = append(d.Sections, Section{Title: key, JSON: val})
			}
		default:
			pairs = append(pairs, KV{key, val})
		}
	}
	if len(pairs) > 0 {
		d.Sections = append([]Section{{Title: "fields", Pairs: pairs}}, d.Sections...)
	}
	return d, true
}

func inList(k string, set []string) bool {
	for _, s := range set {
		if s == k {
			return true
		}
	}
	return false
}

// --- repli -------------------------------------------------------------------

func formatFallback(line string) Detail {
	d := Detail{Kind: "text"}
	text, secs := splitInlineJSON(line)
	d.Sections = secs
	tsLoc := tsRe.FindStringSubmatchIndex(text)
	lvlLoc := levelRe.FindStringIndex(text)
	if tsLoc != nil {
		d.Header = append(d.Header, KV{"date", text[tsLoc[2]:tsLoc[3]]})
	}
	if lvlLoc != nil {
		d.Level = NormLevel(text[lvlLoc[0]:lvlLoc[1]])
		if d.Level != "" {
			d.Header = append(d.Header, KV{"level", d.Level})
		}
	}
	if tsLoc != nil || lvlLoc != nil {
		text = stripHeader(text, tsLoc, lvlLoc)
	}
	d.Message = text
	return d
}
