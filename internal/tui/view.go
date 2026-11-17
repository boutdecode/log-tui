package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kevin/log-tui/internal/store"
)

const (
	sideWidth    = 30
	sideMinWidth = 90 // largeur de terminal minimale pour afficher la sidebar
)

func (m Model) View() tea.View {
	var b strings.Builder
	b.WriteString(m.header() + "\n")

	bodyH := m.bodyH()
	var body []string
	switch m.mode {
	case modeDetail:
		dl := strings.Split(m.detail.View(), "\n")
		for i := range dl {
			dl[i] = " " + dl[i]
		}
		body = panel("Détail", dl, m.w, bodyH, accent)
	case modeStats:
		body = panel("Statistiques", m.statsLines(m.w-2), m.w, bodyH, accent)
	case modeHelp:
		body = panel("Aide", helpLines(), m.w, bodyH, accent)
	case modeConfig:
		body = panel("Configuration · "+m.cfgPath, strings.Split(m.editor.View(), "\n"), m.w, bodyH, accent)
	default:
		body = m.logAndSide(bodyH)
	}
	for i := 0; i < bodyH; i++ {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		b.WriteString(fit(line, m.w) + "\n")
	}
	b.WriteString(fit(modeBadge(m.mode)+" "+m.footer(), m.w) + "\n")
	b.WriteString(fit(keyHints(m.hints()), m.w))

	out := b.String()
	if m.bg != "" {
		lines := strings.Split(out, "\n")
		for i, l := range lines {
			lines[i] = paintBackground(l, m.bg)
		}
		out = strings.Join(lines, "\n")
	}
	v := tea.NewView(out)
	if m.mode == modeConfig {
		v.Cursor = m.editor.Cursor()
		if v.Cursor != nil {
			v.Cursor.X++    // bordure gauche
			v.Cursor.Y += 2 // en-tête + bordure haute
		}
	}
	v.AltScreen = true
	v.ReportFocus = true // FocusMsg / BlurMsg, pour les notifications
	v.WindowTitle = "log-tui"
	return v
}

func (m Model) header() string {
	chips := []string{brandSt.Render("◆ log-tui")}
	n := fmt.Sprintf("%d entrées", m.total)
	if len(m.rows) < m.total {
		n += fmt.Sprintf(" · %d affichées", len(m.rows))
	}
	chips = append(chips, chipSt.Render(n))
	if m.sel > 0 {
		chips = append(chips, chipProv.Render("provider: "+m.names[m.sel-1]))
	}
	if m.levelIdx > 0 {
		chips = append(chips, chipLevel.Render("niveau "+levelPresets[m.levelIdx].label))
	}
	if m.queryStr != "" {
		chips = append(chips, chipWarn.Render("filtre actif"))
	}
	switch {
	case m.mode == modeConfig && m.editor.Value() != m.edSaved:
		chips = append(chips, chipWarn.Render("● modifié"))
	case m.mode == modeDetail:
		chips = append(chips, chipSt.Render(fmt.Sprintf("%d/%d", m.cursor+1, len(m.rows))))
	}
	if m.follow {
		chips = append(chips, chipLive.Render("▼ suivi"))
	} else {
		chips = append(chips, chipPause.Render("⏸ figé · f"))
	}
	line := strings.Join(chips, barStyle.Render(" "))
	if pad := m.w - lipgloss.Width(line); pad > 0 {
		line += barStyle.Render(strings.Repeat(" ", pad))
	}
	return fit(line, m.w)
}

func (m Model) footer() string {
	switch {
	case m.mode == modeConfig:
		if m.edErr {
			return errStyle.Render("✗ " + clean(m.edMsg))
		}
		if m.edMsg != "" {
			return okStyle.Render("✓ " + clean(m.edMsg))
		}
		return dim.Render(m.cfgPath)
	case m.mode == modeSearch:
		return m.input.View()
	case m.loadErr != nil:
		return errStyle.Render("recherche: " + m.loadErr.Error())
	}
	// Première erreur de provider pertinente.
	for _, n := range m.names {
		if m.sel > 0 && m.names[m.sel-1] != n {
			continue
		}
		if s := m.status[n]; s.err != nil {
			return errStyle.Render("✗ " + n + ": " + clean(s.err.Error()))
		}
	}
	if m.queryStr != "" {
		return warnSt.Render("filtre: ") + m.queryStr + dim.Render("   (esc: effacer)")
	}
	if m.mode != modeLog {
		return ""
	}
	return dim.Render("/ pour rechercher")
}

func (m Model) hints() string {
	switch m.mode {
	case modeSearch:
		return "enter: appliquer · esc: annuler"
	case modeDetail:
		return "↑↓/pgup/pgdn: défiler · ←→: entrée précédente/suivante · esc: retour"
	case modeConfig:
		return "ctrl+s: enregistrer et appliquer · esc: fermer"
	case modeStats, modeHelp:
		return "esc: retour"
	}
	return "/ chercher · tab providers · enter détail · l niveau · f suivre · r rafraîchir · p suivre/pause · s stats · c config · ? aide · q quitter"
}

// logAndSide compose la sidebar (si la place le permet) et la liste de logs, chacune dans son cadre.
func (m Model) logAndSide(h int) []string {
	logW := m.w
	var side []string
	if m.w >= sideMinWidth {
		logW = m.w - sideWidth
		sc := borderIdle
		if m.side {
			sc = accent
		}
		side = panel("Providers", m.sidebar(sideWidth-2, h-2), sideWidth, h, sc)
	}
	lc := accent
	if m.side && side != nil {
		lc = borderIdle
	}
	ttl := "Logs"
	if m.queryStr != "" {
		ttl += " · " + m.queryStr
	}
	logPanel := panel(ttl, m.logLines(logW-2, h-2), logW, h, lc)
	if side == nil {
		return logPanel
	}
	out := make([]string, h)
	for i := range out {
		out[i] = side[i] + logPanel[i]
	}
	return out
}

func (m Model) logLines(w, h int) []string {
	out := make([]string, h)
	if len(m.rows) == 0 {
		msg := "aucune entrée"
		if m.total == 0 && m.queryStr == "" && m.sel == 0 && m.levelIdx == 0 {
			msg = "en attente des premiers logs…"
			if m.noneFollowed() {
				msg = "aucun provider suivi : choisissez-en un (tab, ↑/↓) puis p pour démarrer son suivi"
			}
		}
		out[0] = dim.Render(" " + msg)
		return out
	}
	for i := 0; i < h; i++ {
		idx := m.top + i
		if idx >= len(m.rows) {
			out[i] = ""
			continue
		}
		out[i] = rowLine(m.rows[idx], w, idx == m.cursor, m.sel == 0 && len(m.names) > 1)
	}
	return out
}

func rowLine(r store.Row, w int, sel, showProv bool) string {
	ts := r.TS.Format("01-02 15:04:05")
	lvl := fmt.Sprintf("%-5s", strings.ToUpper(r.Level))
	if len(lvl) > 5 {
		lvl = lvl[:5]
	}
	msg := clean(r.Msg)
	extra := ""
	if r.Fields != "" {
		extra = clean(compactFields(r.Fields))
	}
	prov := ""
	if showProv {
		prov = fit(clean(r.Provider), 10)
	}

	gut := " "
	if r.Level != "" {
		gut = levelStyle(r.Level).Render("▌")
	}
	if sel {
		gut = levelStyle(r.Level).Background(lipgloss.Color("238")).Render("▌")
		if r.Level == "" {
			gut = selected.Render(" ")
		}
		line := " " + ts + " "
		if showProv {
			line += prov + " "
		}
		line += lvl + " " + msg
		if extra != "" {
			line += "  " + extra
		}
		return gut + selected.Render(fit(line, w-1))
	}
	line := gut + " " + dim.Render(ts) + " "
	if showProv {
		line += provStyle(r.Provider).Render(prov) + " "
	}
	line += levelStyle(r.Level).Render(lvl) + " " + msg
	if extra != "" {
		line += "  " + dim.Render(extra)
	}
	return fit(line, w)
}

func (m Model) sidebar(w, h int) []string {
	lines := []string{}
	total := 0
	for _, c := range m.counts {
		total += c
	}
	add := func(idx int, dot, name string, count int) {
		label := fmt.Sprintf(" %s %s", dot, name)
		cnt := fmt.Sprintf("%d ", count)
		pad := w - lipgloss.Width(label) - lipgloss.Width(cnt)
		if pad < 1 {
			label = fit(label, w-lipgloss.Width(cnt)-1)
			pad = 1
		}
		line := label + strings.Repeat(" ", pad) + cnt
		if idx == m.sel {
			if m.side {
				line = selFocus.Render(fit(ansiPlain(line), w))
			} else {
				line = selected.Render(fit(ansiPlain(line), w))
			}
		}
		lines = append(lines, line)
	}
	add(0, " ", "tous", total)
	for i, n := range m.names {
		add(i+1, m.dot(n), n, m.counts[n])
	}

	if m.sel > 0 {
		n := m.names[m.sel-1]
		s := m.status[n]
		lines = append(lines, "", bold.Render(" "+n), dim.Render(" "+m.run.Kind(n)))
		lines = append(lines, dim.Render(fmt.Sprintf(" toutes les %s", m.run.Interval(n))))
		switch {
		case s.paused && s.last.IsZero():
			lines = append(lines, dim.Render(" non suivi · p: suivre"))
		case s.paused:
			lines = append(lines, warnSt.Render(" en pause · p: reprendre"))
		case s.fetching:
			lines = append(lines, warnSt.Render(" récupération…"))
		case !s.last.IsZero():
			lines = append(lines, dim.Render(fmt.Sprintf(" dernier: il y a %s (%s)", ago(s.last), s.dur.Round(time.Millisecond))))
		}
		lines = append(lines, dim.Render(fmt.Sprintf(" +%d depuis le lancement", s.added)))
		if s.err != nil {
			for _, l := range wrap(clean(s.err.Error()), w-2) {
				lines = append(lines, errStyle.Render(" "+l))
			}
		}
	}
	out := make([]string, h)
	for i := range out {
		if i < len(lines) {
			out[i] = fit(lines[i], w)
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

// noneFollowed indique qu'aucun provider n'a jamais été récupéré ni n'est en cours.
func (m Model) noneFollowed() bool {
	for _, s := range m.status {
		if !s.paused || !s.last.IsZero() {
			return false
		}
	}
	return true
}

func (m Model) dot(name string) string {
	s := m.status[name]
	switch {
	case s.paused && s.last.IsZero():
		return dim.Render("○")
	case s.paused:
		return warnSt.Render("○") // en pause après avoir été suivi : même pastille, en jaune
	case s.fetching:
		return warnSt.Render("◌")
	case s.err != nil:
		return errStyle.Render("●")
	case s.last.IsZero():
		return dim.Render("○")
	}
	return okStyle.Render("●")
}

func ago(t time.Time) string {
	d := time.Since(t).Round(time.Second)
	switch {
	case d < time.Second:
		return "<1s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func wrap(s string, w int) []string {
	if w < 8 {
		return []string{s}
	}
	var out []string
	r := []rune(s)
	for len(r) > w {
		out = append(out, string(r[:w]))
		r = r[w:]
	}
	return append(out, string(r))
}

// ansiPlain retire les styles pour pouvoir appliquer un fond homogène.
func ansiPlain(s string) string { return clean(s) }

func helpLines() []string {
	return []string{
		bold.Render(" Raccourcis"),
		"",
		"  ↑/↓ j/k        naviguer           pgup/pgdn  demi-page",
		"  g / G          début / fin        f          suivre les nouveaux logs",
		"  enter          détail (JSON décodé, ←/→ entrée suivante)",
		"  tab            focus liste des providers (↑/↓ pour filtrer)",
		"  /              rechercher         esc        effacer le filtre",
		"  l              filtre de niveau (tous → ≥error → ≥warn → ≥info)",
		"  r              rafraîchir tout    p          suivre / mettre en pause (provider sélectionné)",
		"  s              statistiques       c          éditer la configuration",
		"  q              quitter",
		"",
		bold.Render(" Notifications"),
		"",
		"  Les nouvelles erreurs (error/fatal) déclenchent une notification quand le terminal",
		"  n'a pas le focus (notify-send, sinon séquence du terminal). Options : notify,",
		"  notify_cooldown. Sous tmux : set -g focus-events on",
		"",
		bold.Render(" Langage de recherche"),
		"",
		"  timeout               sous-chaîne (insensible à la casse)",
		`  "connection reset"    phrase`,
		"  -healthcheck          exclure",
		"  level:error,warn      niveau(x)",
		"  provider:api          source",
		"  since:15m             fenêtre de temps (s, m, h, d)",
		"  user.id:42            sous-chaîne dans un champ JSON (chemin pointé)",
	}
}
