package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/kevin/log-tui/internal/parse"
)

var (
	kindSt    = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("60")).Padding(0, 1)
	sectionSt = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
)

// statusStyle colore un code de statut HTTP selon sa classe.
func statusStyle(code string) lipgloss.Style {
	switch {
	case strings.HasPrefix(code, "2"):
		return okStyle
	case strings.HasPrefix(code, "3"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	case strings.HasPrefix(code, "4"):
		return warnSt
	case strings.HasPrefix(code, "5"):
		return errStyle
	}
	return lipgloss.NewStyle()
}

// refreshDetail reconstruit le contenu du panneau détail pour la ligne sélectionnée.
// La ligne brute est ré-analysée à l'ouverture : le détail ne dépend pas de ce
// qui a été extrait à l'ingestion.
func (m *Model) refreshDetail() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		m.detail.SetContent(dim.Render("(aucune entrée)"))
		return
	}
	r := m.rows[m.cursor]
	d := parse.Format(r.Raw)

	level := r.Level
	if level == "" {
		level = d.Level
	}
	lvl := strings.ToUpper(level)
	if lvl == "" {
		lvl = "-"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  %s  %s\n",
		dim.Render(r.TS.Format("2006-01-02 15:04:05.000")),
		provStyle(r.Provider).Render(r.Provider),
		levelStyle(level).Render(lvl),
		kindSt.Render(d.Kind))

	if len(d.Header) > 0 {
		b.WriteString("\n")
		writePairs(&b, d.Header)
	}
	if d.Message != "" {
		b.WriteString("\n" + bold.Render(clean(d.Message)) + "\n")
	}
	for _, s := range d.Sections {
		b.WriteString("\n" + sectionSt.Render("── "+s.Title+" ──") + "\n")
		switch {
		case s.JSON != "":
			if pretty, ok := prettyJSON(s.JSON); ok {
				b.WriteString(pretty + "\n")
			} else {
				b.WriteString(clean(s.JSON) + "\n")
			}
		default:
			writePairs(&b, s.Pairs)
		}
	}
	if d.Kind == "text" && d.Message == "" && len(d.Sections) == 0 {
		b.WriteString("\n" + clean(r.Raw) + "\n")
	}

	m.detail.SetContent(strings.TrimRight(b.String(), "\n"))
	m.detail.GotoTop()
}

// writePairs écrit un tableau clé/valeur aligné.
func writePairs(b *strings.Builder, kvs []parse.KV) {
	w := 0
	for _, kv := range kvs {
		w = max(w, lipgloss.Width(kv.Key))
	}
	for _, kv := range kvs {
		val := clean(kv.Value)
		switch kv.Key {
		case "statut":
			val = statusStyle(kv.Value).Render(val)
		case "niveau":
			val = levelStyle(parse.NormLevel(kv.Value)).Render(val)
		}
		b.WriteString(jKey.Render(fit(kv.Key, w)) + "  " + val + "\n")
	}
}
