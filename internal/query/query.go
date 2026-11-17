// Package query implémente le mini-langage de recherche et sa traduction SQL.
//
// Syntaxe (séparée par des espaces) :
//
//	timeout            sous-chaîne dans la ligne brute
//	"connection reset" sous-chaîne avec espaces
//	-healthcheck       exclut les lignes contenant le terme
//	level:error,warn   filtre sur le niveau
//	provider:api       filtre sur le provider
//	since:15m          dernières 15 minutes (s, m, h, d)
//	user.id:42         sous-chaîne dans le champ JSON user.id
package query

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Field est un filtre sur un champ JSON.
type Field struct {
	Path  string
	Value string
}

// Query est une recherche analysée.
type Query struct {
	Terms    []string
	Not      []string
	Levels   []string
	Provider string
	Since    time.Duration
	Fields   []Field
}

var fieldRe = regexp.MustCompile(`^[A-Za-z_@][\w.\-@]*$`)

// Parse analyse une chaîne de recherche. Elle ne renvoie jamais d'erreur :
// ce qui n'est pas reconnu devient un terme texte.
func Parse(s string) Query {
	var q Query
	for _, tok := range tokenize(s) {
		neg := false
		if strings.HasPrefix(tok, "-") && len(tok) > 1 {
			neg = true
			tok = tok[1:]
		}
		if i := strings.IndexByte(tok, ':'); i > 0 && i < len(tok)-1 && !neg {
			key, val := tok[:i], tok[i+1:]
			switch strings.ToLower(key) {
			case "level", "lvl":
				for _, l := range strings.Split(val, ",") {
					if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
						q.Levels = append(q.Levels, l)
					}
				}
				continue
			case "provider", "p":
				q.Provider = val
				continue
			case "since":
				if d, err := ParseDuration(val); err == nil {
					q.Since = d
					continue
				}
			default:
				if fieldRe.MatchString(key) {
					q.Fields = append(q.Fields, Field{key, val})
					continue
				}
			}
		}
		if neg {
			q.Not = append(q.Not, tok)
		} else {
			q.Terms = append(q.Terms, tok)
		}
	}
	return q
}

// ParseDuration accepte en plus de time.ParseDuration le suffixe "d".
func ParseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		var n float64
		if _, err := fmt.Sscanf(strings.TrimSuffix(s, "d"), "%g", &n); err != nil {
			return 0, err
		}
		return time.Duration(n * 24 * float64(time.Hour)), nil
	}
	return time.ParseDuration(s)
}

func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote, has := false, false
	flush := func() {
		if has {
			out = append(out, cur.String())
		}
		cur.Reset()
		has = false
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			has = true
		case (r == ' ' || r == '\t') && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
			has = true
		}
	}
	flush()
	return out
}

// Where construit la clause WHERE (sans le mot-clé) et ses arguments.
// La table doit s'appeler entries et la table FTS entries_fts (trigram).
func (q Query) Where(now time.Time) (string, []any) {
	conds := []string{"1=1"}
	var args []any

	if q.Provider != "" {
		conds = append(conds, "provider = ?")
		args = append(args, q.Provider)
	}
	if len(q.Levels) > 0 {
		conds = append(conds, "level IN ("+strings.TrimSuffix(strings.Repeat("?,", len(q.Levels)), ",")+")")
		for _, l := range q.Levels {
			args = append(args, l)
		}
	}
	if q.Since > 0 {
		conds = append(conds, "ts >= ?")
		args = append(args, now.Add(-q.Since).UnixMilli())
	}
	for _, f := range q.Fields {
		conds = append(conds, `CAST(json_extract(fields, ?) AS TEXT) LIKE ? ESCAPE '\'`)
		args = append(args, jsonPath(f.Path), "%"+likeEscape(f.Value)+"%")
	}
	for _, t := range q.Terms {
		c, a := termCond(t)
		conds = append(conds, c)
		args = append(args, a)
	}
	for _, t := range q.Not {
		c, a := termCond(t)
		conds = append(conds, "NOT "+c)
		args = append(args, a)
	}
	return strings.Join(conds, " AND "), args
}

// Empty indique qu'aucun filtre n'est défini.
func (q Query) Empty() bool {
	return len(q.Terms) == 0 && len(q.Not) == 0 && len(q.Levels) == 0 &&
		q.Provider == "" && q.Since == 0 && len(q.Fields) == 0
}

func termCond(t string) (string, any) {
	// trigram FTS exige au moins 3 caractères.
	if len([]rune(t)) >= 3 {
		return "id IN (SELECT rowid FROM entries_fts WHERE entries_fts MATCH ?)",
			`"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	return `raw LIKE ? ESCAPE '\'`, "%" + likeEscape(t) + "%"
}

func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func jsonPath(p string) string {
	parts := strings.Split(p, ".")
	var b strings.Builder
	b.WriteString("$")
	for _, s := range parts {
		b.WriteString(`."` + strings.ReplaceAll(s, `"`, ``) + `"`)
	}
	return b.String()
}
