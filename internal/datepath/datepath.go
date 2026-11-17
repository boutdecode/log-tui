// Package datepath résout les motifs de date (style strftime) dans un chemin
// de fichier, par ex. /var/log/prod-%Y-%m-%d.log.
package datepath

import (
	"errors"
	"fmt"
	"github.com/kevin/log-tui/internal/i18n"
	"strings"
	"time"
)

// Directives supportées : %Y année, %y année sur 2 chiffres, %m mois, %d jour,
// %H heure, %j jour de l'année, %% pour un pourcentage littéral.

// Has indique si s contient au moins une directive de date.
func Has(s string) bool {
	return strings.Contains(strings.ReplaceAll(s, "%%", ""), "%")
}

// HasGlob indique si s contient un caractère joker (*, ?, [).
func HasGlob(s string) bool { return strings.ContainsAny(s, "*?[") }

// Check valide le motif.
func Check(pattern string) error {
	_, err := expand(pattern, time.Time{})
	return err
}

// Expand remplace les directives par les valeurs de t. Un motif invalide est
// renvoyé tel quel (utiliser Check à la validation de la config).
func Expand(pattern string, t time.Time) string {
	out, err := expand(pattern, t)
	if err != nil {
		return pattern
	}
	return out
}

func expand(p string, t time.Time) (string, error) {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		if p[i] != '%' {
			b.WriteByte(p[i])
			continue
		}
		i++
		if i >= len(p) {
			return "", errors.New(i18n.T("err.datetrail", p))
		}
		switch p[i] {
		case 'Y':
			fmt.Fprintf(&b, "%04d", t.Year())
		case 'y':
			fmt.Fprintf(&b, "%02d", t.Year()%100)
		case 'm':
			fmt.Fprintf(&b, "%02d", int(t.Month()))
		case 'd':
			fmt.Fprintf(&b, "%02d", t.Day())
		case 'H':
			fmt.Fprintf(&b, "%02d", t.Hour())
		case 'j':
			fmt.Fprintf(&b, "%03d", t.YearDay())
		case '%':
			b.WriteByte('%')
		default:
			return "", errors.New(i18n.T("err.datedirective", string(p[i]), p))
		}
	}
	return b.String(), nil
}
